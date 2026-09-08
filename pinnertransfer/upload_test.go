package pinnertransfer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	contentfs "go.lumeweb.com/ipfs-content/fs"

	"go.lumeweb.com/pinner/core/uploads"
)

// capturingUploadService snapshots the fs.FS/name/wait/wrap the executor
// handed to uploads.Service.Upload and returns a canned result. The snapshot
// runs at Upload time because the executor's temp-file buffer is closed as
// soon as the handler returns.
type capturingUploadService struct {
	name   string
	wait   bool
	wrap   bool
	snap   map[string]string // name -> contents
	single bool              // fs was a *contentfs.SingleFileFS
	result *uploads.UploadResult
	err    error
}

func (c *capturingUploadService) Upload(ctx context.Context, filesystem fs.FS, name string, wait bool, wrap bool) (*uploads.UploadResult, error) {
	c.name, c.wait, c.wrap = name, wait, wrap
	_, c.single = filesystem.(*contentfs.SingleFileFS)
	snap, snapErr := readFS(filesystem)
	if snapErr != nil {
		c.err = snapErr
	}
	// SingleFileFS surfaces its sole file under the root name "." regardless of
	// the requested path; key the snapshot under the upload name instead so
	// assertions stay name-shaped.
	if c.single && len(snap) == 1 {
		if content, ok := snap["."]; ok {
			snap = map[string]string{name: content}
		}
	}
	c.snap = snap
	if c.err != nil {
		return nil, c.err
	}
	return c.result, nil
}

// readFS dumps an fs.FS as name->contents for assertions.
func readFS(fsys fs.FS) (map[string]string, error) {
	out := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out[p] = string(data)
		return nil
	})
	return out, err
}

func TestStreamUploadSingleFile(t *testing.T) {
	svc := &capturingUploadService{result: &uploads.UploadResult{CID: "bafyfile"}}
	handler := StreamUpload(svc, 0)

	res, err := handler(context.Background(), strings.NewReader("plain data"), 10, "data.bin", true, string(ArchivePreserve), false)
	if err != nil {
		t.Fatalf("StreamUpload: %v", err)
	}
	if res != any(svc.result) {
		t.Errorf("result = %v, want the service's UploadResult", res)
	}
	if svc.name != "data.bin" || !svc.wait || svc.wrap {
		t.Errorf("svc got name=%q wait=%v wrap=%v, want data.bin/true/false", svc.name, svc.wait, svc.wrap)
	}
	if !svc.single {
		t.Errorf("fs type = want *contentfs.SingleFileFS for a non-wrapped upload")
	}
	// A non-archive upload goes through the single-file FS: exactly one file
	// named after the upload name with the full bytes.
	if len(svc.snap) != 1 {
		t.Fatalf("single-file FS entries = %v, want exactly one", svc.snap)
	}
	data, ok := svc.snap["data.bin"]
	if !ok || data != "plain data" {
		t.Fatalf("entries %v, want data.bin with the full bytes", svc.snap)
	}
}

func TestStreamUploadArchiveConvertUploadsExtractedTree(t *testing.T) {
	svc := &capturingUploadService{result: &uploads.UploadResult{CID: "bafydir"}}
	handler := StreamUpload(svc, 0)

	zr := buildZip(t, map[string]string{
		"sub/hello.txt": "hello world",
		"top.txt":       "top content",
	})
	if _, err := handler(context.Background(), zr, int64(zr.Len()), "site.zip", false, string(ArchiveConvert), false); err != nil {
		t.Fatalf("StreamUpload(archive): %v", err)
	}

	want := map[string]string{
		"sub/hello.txt": "hello world",
		"top.txt":       "top content",
	}
	if len(svc.snap) != len(want) {
		t.Fatalf("extracted tree entries %v, want %v", svc.snap, want)
	}
	for name, contents := range want {
		if svc.snap[name] != contents {
			t.Errorf("extracted %q = %q, want %q", name, svc.snap[name], contents)
		}
	}
	if svc.wrap || svc.single {
		t.Errorf("archive-convert uploads must be a plain directory FS (wrap=%v single=%v)", svc.wrap, svc.single)
	}
	if svc.name != "site.zip" {
		t.Errorf("archive-convert keeps the caller name, got %q", svc.name)
	}
}

func TestStreamUploadArchivePreserveUploadsRawArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, zerr := zw.Create("a.txt")
	if zerr != nil {
		t.Fatalf("create entry: %v", zerr)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	raw := buf.Bytes()

	svc := &capturingUploadService{result: &uploads.UploadResult{CID: "bafyzip"}}
	handler := StreamUpload(svc, 0)

	if _, err := handler(context.Background(), bytes.NewReader(raw), int64(len(raw)), "bundle.zip", true, string(ArchivePreserve), false); err != nil {
		t.Fatalf("StreamUpload(preserve): %v", err)
	}
	if svc.wrap {
		t.Errorf("preserve + wrap=false must not request a wrap")
	}
	if len(svc.snap) != 1 || svc.snap["bundle.zip"] != string(raw) {
		t.Fatalf("snap %v, want the raw archive bytes under bundle.zip", svc.snap)
	}
}

func TestStreamUploadWrapSniffsHTMLToIndex(t *testing.T) {
	html := "<!DOCTYPE html><html><body>wrapped</body></html>"
	svc := &capturingUploadService{result: &uploads.UploadResult{CID: "bafysite"}}
	handler := StreamUpload(svc, 0)

	// wrap=true with the default name: HTML content is renamed index.html so
	// the wrapped site resolves at its root.
	if _, err := handler(context.Background(), strings.NewReader(html), int64(len(html)), "", true, string(ArchivePreserve), true); err != nil {
		t.Fatalf("StreamUpload(wrap): %v", err)
	}
	if svc.name != "index.html" {
		t.Errorf("wrapped HTML name = %q, want index.html", svc.name)
	}
	if !svc.wrap {
		t.Errorf("wrap=true must be forwarded to the upload service")
	}
	if len(svc.snap) != 1 || svc.snap["index.html"] != html {
		t.Fatalf("single-file snapshot %v, want index.html with the full HTML", svc.snap)
	}

	// An explicit name is always honored, even for HTML content.
	svc2 := &capturingUploadService{result: &uploads.UploadResult{CID: "bafysite"}}
	handler2 := StreamUpload(svc2, 0)
	if _, err := handler2(context.Background(), strings.NewReader(html), int64(len(html)), "landing.html", false, string(ArchiveConvert), true); err != nil {
		t.Fatalf("StreamUpload(wrap, named): %v", err)
	}
	if svc2.name != "landing.html" {
		t.Errorf("explicit name = %q, want landing.html (never overridden)", svc2.name)
	}
}

func TestStreamUploadEnforcesArchiveTreeCap(t *testing.T) {
	svc := &capturingUploadService{result: &uploads.UploadResult{CID: "never"}}
	handler := StreamUpload(svc, 12) // aggregate cap: 2 files x 10 bytes breach it

	zr := buildZip(t, map[string]string{
		"a.txt": "0123456789",
		"b.txt": "0123456789",
	})
	_, err := handler(context.Background(), zr, int64(zr.Len()), "big.zip", false, string(ArchiveConvert), false)
	if err == nil {
		t.Fatal("expected over-cap archive tree rejection")
	}
	if !strings.Contains(err.Error(), "exceeds max_mcp_upload_size") {
		t.Errorf("error = %v, want a max_mcp_upload_size complaint", err)
	}
	if svc.snap != nil {
		t.Errorf("upload service must NOT be invoked for an over-cap tree (snap=%v)", svc.snap)
	}
}

// TestStreamUploadConvertNonArchiveFallsThrough pins the archive-detection
// gate: with archive_mode=convert a non-archive stream skips the extract
// branch entirely and uploads how it would have without the flag.
func TestStreamUploadConvertNonArchiveFallsThrough(t *testing.T) {
	svc := &capturingUploadService{result: &uploads.UploadResult{CID: "fallback"}}
	handler := StreamUpload(svc, 0)

	payload := "definitely not an archive, just plain text content"
	res, err := handler(context.Background(), strings.NewReader(payload), int64(len(payload)), "mystery.dat", false, string(ArchiveConvert), false)
	if err != nil {
		t.Fatalf("StreamUpload(convert, non-archive): %v", err)
	}
	if res != any(svc.result) {
		t.Errorf("result = %v, want the service result", res)
	}
	if len(svc.snap) != 1 || svc.snap["mystery.dat"] != payload {
		t.Fatalf("fallback snapshot %v, want the raw file as mystery.dat", svc.snap)
	}
}

// TestStreamUploadUploadServiceErrorPropagates pins the contract that the
// executor is a thin proxy: the service's error and result flow through as-is.
func TestStreamUploadUploadServiceErrorPropagates(t *testing.T) {
	boom := errors.New("upload boomed")
	svc := &capturingUploadService{err: boom}
	handler := StreamUpload(svc, 0)
	_, err := handler(context.Background(), strings.NewReader("x"), 1, "x.bin", true, string(ArchivePreserve), false)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the service error", err)
	}
}
