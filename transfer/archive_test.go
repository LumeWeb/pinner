package transfer

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"

	contentArchive "go.lumeweb.com/ipfs-content/archive"
)

func TestParseArchiveMode(t *testing.T) {
	cases := []struct {
		in   string
		want ArchiveMode
	}{
		{"convert", ArchiveConvert},
		{"preserve", ArchivePreserve},
		{"", ArchiveConvert},
		{"bogus", ArchiveConvert},
	}
	for _, c := range cases {
		if got := ParseArchiveMode(c.in); got != c.want {
			t.Errorf("ParseArchiveMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// buildZip creates an in-memory zip with the given entries (name -> contents).
// A trailing slash in name marks a directory entry.
func buildZip(t *testing.T, entries map[string]string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, contents := range entries {
		if strings.HasSuffix(name, "/") {
			if _, err := zw.Create(name); err != nil {
				t.Fatalf("create dir entry %q: %v", name, err)
			}
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			t.Fatalf("write entry %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestDetectArchive(t *testing.T) {
	t.Run("zip", func(t *testing.T) {
		r := buildZip(t, map[string]string{"a.txt": "hello"})
		format, isArchive, err := SniffArchive(r)
		if err != nil {
			t.Fatalf("SniffArchive: %v", err)
		}
		if format != contentArchive.FormatZIP {
			t.Errorf("format = %v, want %v", format, contentArchive.FormatZIP)
		}
		if !isArchive {
			t.Errorf("IsArchiveFormat() = false, want true for zip")
		}
	})

	t.Run("plain-text-not-archive", func(t *testing.T) {
		r := bytes.NewReader([]byte("definitely not an archive, just plain text content"))
		format, isArchive, err := SniffArchive(r)
		if err != nil {
			t.Fatalf("SniffArchive: %v", err)
		}
		if isArchive {
			t.Errorf("IsArchiveFormat() = true for plain text, want false (format=%v)", format)
		}
	})
}

func TestOpenArchiveFS(t *testing.T) {
	ctx := context.Background()
	r := buildZip(t, map[string]string{
		"sub/":          "",
		"sub/hello.txt": "hello world",
		"top.txt":       "top content",
	})

	f, closer, err := OpenArchiveFS(ctx, r)
	if err != nil {
		t.Fatalf("OpenArchiveFS: %v", err)
	}
	defer func() {
		if err := closer(); err != nil {
			t.Errorf("closer: %v", err)
		}
	}()

	// Walk the returned fs.FS and assert entries + content.
	var names []string
	err = fs.WalkDir(f, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		names = append(names, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}

	want := map[string]string{
		"sub/hello.txt": "hello world",
		"top.txt":       "top content",
	}
	for name, contents := range want {
		data, err := fs.ReadFile(f, name)
		if err != nil {
			t.Errorf("ReadFile(%q): %v", name, err)
			continue
		}
		if string(data) != contents {
			t.Errorf("ReadFile(%q) = %q, want %q", name, data, contents)
		}
	}

	// Sanity check the walk actually saw our entries.
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "sub/hello.txt") {
		t.Errorf("walked entries %v did not include sub/hello.txt", names)
	}
	if !strings.Contains(joined, "top.txt") {
		t.Errorf("walked entries %v did not include top.txt", names)
	}
}

// TestOpenArchiveFSRejectsNonSeekReader pins the ReaderAt+Seeker requirement:
// a plain io.Reader (no Seek/ReaderAt) is rejected with an error, not a panic.
func TestOpenArchiveFSRejectsNonSeekReader(t *testing.T) {
	_, _, err := OpenArchiveFS(context.Background(), struct{ io.Reader }{strings.NewReader("x")})
	if err == nil {
		t.Fatal("expected error for reader without Seek/ReaderAt, got nil")
	}
	if !strings.Contains(err.Error(), "ReaderAtSeeker") {
		t.Errorf("error = %v, want a ReaderAtSeeker complaint", err)
	}
}

func TestCheckTreeSize(t *testing.T) {
	ctx := context.Background()

	// A real archive tree (10 bytes per file) for the in-cap cases.
	zr := buildZip(t, map[string]string{
		"a.txt": "0123456789",
		"b.txt": "0123456789",
	})

	t.Run("within-cap", func(t *testing.T) {
		r := buildZip(t, map[string]string{"a.txt": "0123456789"})
		f, closer, err := OpenArchiveFS(ctx, r)
		if err != nil {
			t.Fatalf("OpenArchiveFS: %v", err)
		}
		defer closer()
		if err := CheckTreeSize(f, 100, TreeSizeAggregate); err != nil {
			t.Errorf("aggregate cap not exceeded, got %v", err)
		}
	})

	t.Run("aggregate-over-cap", func(t *testing.T) {
		f, closer, err := OpenArchiveFS(ctx, zr)
		if err != nil {
			t.Fatalf("OpenArchiveFS: %v", err)
		}
		defer closer()
		// Two 10-byte files, aggregate cap 12: the sum breaches, either file alone does not.
		err = CheckTreeSize(f, 12, TreeSizeAggregate)
		if err == nil {
			t.Fatal("expected aggregate-cap rejection for a 20-byte tree with cap 12")
		}
		if !strings.Contains(err.Error(), "exceeds max_mcp_upload_size") {
			t.Errorf("error = %v, want a max_mcp_upload_size complaint", err)
		}
	})

	t.Run("disabled-cap", func(t *testing.T) {
		f, closer, err := OpenArchiveFS(ctx, zr)
		if err != nil {
			t.Fatalf("OpenArchiveFS: %v", err)
		}
		defer closer()
		if err := CheckTreeSize(f, 0, TreeSizeAggregate); err != nil {
			t.Errorf("maxBytes<=0 must disable the check, got %v", err)
		}
	})
}
