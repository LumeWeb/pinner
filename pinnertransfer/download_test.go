package pinnertransfer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpplane/transfer"

	"go.lumeweb.com/pinner/core/download"
)

// fakeDownloadService is a core/download.Service stub carrying only Cat; the
// stream executor never touches the remaining methods. Embedding the interface
// keeps the stub honest if the contract grows while this executor stays
// Cat-only.
type fakeDownloadService struct {
	download.Service
	payload string
	err     error
}

func (f *fakeDownloadService) Cat(ctx context.Context, ipfsPath string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewBufferString(f.payload)), nil
}

// TestStreamDownloadStreamsViaCat pins the IPFS download executor: it Cats the
// given path and copies the bytes to the sink writer, closing the reader.
func TestStreamDownloadStreamsViaCat(t *testing.T) {
	svc := &fakeDownloadService{payload: "ipfs bytes"}
	handler := StreamDownload(svc)

	var buf bytes.Buffer
	require.NoError(t, handler(context.Background(), "bafyabc/doc.txt", &buf))
	require.Equal(t, "ipfs bytes", buf.String())
}

func TestSinkDefaultName(t *testing.T) {
	require.Equal(t, "file.txt", SinkDefaultName("bafyabc/file.txt"))
	require.Equal(t, "file.txt", SinkDefaultName("/ipfs/bafyabc/sub/file.txt"))
	require.Equal(t, "file.txt", SinkDefaultName("vault:/docs/file.txt"))
	require.Equal(t, "bafyabc", SinkDefaultName("bafyabc"))
}

func TestResolveLocalOutputPath(t *testing.T) {
	root := t.TempDir()
	// Omitted output_path → source-derived name at the root.
	got, err := ResolveLocalOutputPath(root, "", "f.pdf")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "f.pdf"), got)
	// Relative subdir path is confined to the root (subdirs created later).
	got, err = ResolveLocalOutputPath(root, "sub/f.pdf", "f.pdf")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "sub", "f.pdf"), got)
	// Exactly the root dir is OK.
	got, err = ResolveLocalOutputPath(root, ".", "f.pdf")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "f.pdf"), got)
}

func TestResolveLocalOutputPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	attempts := []string{
		"../escape.txt",        // parent traversal
		"sub/../../escape.txt", // deeper traversal
		"/etc/passwd",          // absolute path
		"/etc/passwd/",         // trailing-separator absolute path
		"\\etc\\evil",          // Windows-root-relative path — never a valid relative target
		"..",                   // exactly parent
	}
	for _, a := range attempts {
		_, err := ResolveLocalOutputPath(root, a, "f.pdf")
		require.Error(t, err, "expected escape rejection for %q", a)
	}
	// Windows drive-letter input is rejected on Windows via VolumeName; on a
	// POSIX host the whole drive path is treated as one opaque (non-separator)
	// filename and lexically stays inside the root, so only the VolumeName
	// branch is meaningful there.
	if filepath.VolumeName(`C:\evil.txt`) != "" {
		_, err := ResolveLocalOutputPath(root, `C:\evil.txt`, "f.pdf")
		require.Error(t, err, "drive-letter output path must be rejected on Windows")
	}
	// Empty root is not configured.
	_, err := ResolveLocalOutputPath("", "f.pdf", "f.pdf")
	require.Error(t, err)
}

func TestExecuteLocalSinkConfinesToRoot(t *testing.T) {
	root := t.TempDir()
	src := "vault:/docs/secret.pdf"
	name := "secret.pdf"
	// An absolute output_path that would escape the root must be rejected.
	_, err := ExecuteLocalSink(context.Background(), src, name, "/etc/evil.pdf", root, 0, func(ctx context.Context, w io.Writer) error {
		_, _ = w.Write([]byte("x"))
		return nil
	})
	require.Error(t, err)
	// A traversal is rejected too.
	_, err = ExecuteLocalSink(context.Background(), src, name, "../evil.pdf", root, 0, func(ctx context.Context, w io.Writer) error {
		_, _ = w.Write([]byte("x"))
		return nil
	})
	require.Error(t, err)
	// A legitimate relative path lands inside the root.
	res, err := ExecuteLocalSink(context.Background(), src, name, "docs/secret.pdf", root, 0, func(ctx context.Context, w io.Writer) error {
		_, _ = w.Write([]byte("plaintext"))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "docs", "secret.pdf"), res.Output)
	data, err := os.ReadFile(res.Output)
	require.NoError(t, err)
	require.Equal(t, "plaintext", string(data))
	// The canonical envelope carries the sink in string form.
	require.Equal(t, string(transfer.SinkLocal), res.Sink)
	require.Equal(t, "ok", res.Status)
}

func TestDownloadSinksAllowed(t *testing.T) {
	require.NoError(t, DownloadSinksAllowed(transfer.SinkLocal, false, false))
	require.NoError(t, DownloadSinksAllowed(transfer.SinkLocal, true, true)) // local always
	require.NoError(t, DownloadSinksAllowed(transfer.SinkDrop, true, false)) // drop needs reachable mux
	require.Error(t, DownloadSinksAllowed(transfer.SinkDrop, false, false))  // no drop coordinator
	require.Error(t, DownloadSinksAllowed(transfer.SinkDrop, true, true))    // openai tunnel: no mux
	require.Error(t, DownloadSinksAllowed("bogus", true, false))
}

func TestWriteLocalDownload(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sub", "out.bin")
	n, err := WriteLocalDownload(context.Background(), out, 0, func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte("hello world"))
		return err
	})
	require.NoError(t, err)
	require.Equal(t, int64(11), n)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(data))
}

func TestWriteLocalDownloadExceedsCap(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.bin")
	// Cap smaller than the stream; the write must fail loudly and must NOT
	// leave a final file (the temp is cleaned up), so no truncated download
	// is presented as complete.
	_, err := WriteLocalDownload(context.Background(), out, 4, func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte("hello world"))
		return err
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds max_mcp_upload_size")
	_, statErr := os.Stat(out)
	require.Error(t, statErr, "final destination must not exist after an over-limit write")
	// The over-cap error is recognized as the coined too-large error.
	require.True(t, transfer.IsDownloadTooLarge(err), "over-cap stream error must satisfy transfer.IsDownloadTooLarge")
}

// TestWriteLocalDownloadTempCleanedUpOnFailure pins the atomicity invariant:
// a failed resolve leaves NO artifacts in the destination directory (neither a
// truncated final file nor a leftover temp).
func TestWriteLocalDownloadTempCleanedUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.bin")
	boom := context.Canceled
	_, err := WriteLocalDownload(context.Background(), out, 0, func(ctx context.Context, w io.Writer) error {
		return boom
	})
	require.ErrorIs(t, err, boom)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries, "destination dir must contain no temp files after a failed write")
}

func TestExecuteDropSinkReportsRealSize(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 1556)

	hd := transfer.NewHTTPDownload()
	defer hd.Stop(context.Background())
	hd.SetBaseURL("http://127.0.0.1") // avoid the loopback listener; not needed here

	res, err := ExecuteDropSink(context.Background(), "bafy/example.pdf", "example.pdf", hd,
		"5m", 0, func(_ context.Context, w io.Writer) error {
			_, err := w.Write(payload)
			return err
		})
	require.NoError(t, err)
	require.Equal(t, int64(1556), res.Size, "drop result must report the real byte size")
	require.Equal(t, string(transfer.SinkDrop), res.Sink)
	require.NotEmpty(t, res.FetchURL, "drop result must carry a fetch URL")

	// The result's canonical JSON (the Text/StructuredContent payload) must
	// carry the real size too, not 0.
	raw, merr := json.Marshal(res)
	require.NoError(t, merr)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, float64(1556), decoded["size"])
}

// TestExecuteDropSinkEnforcesCapAtMint verifies an over-cap drop fails up
// front (at mint/resolve time) rather than minting an endpoint that would
// stream a truncated file.
func TestExecuteDropSinkEnforcesCapAtMint(t *testing.T) {
	hd := transfer.NewHTTPDownload()
	defer hd.Stop(context.Background())
	hd.SetBaseURL("http://127.0.0.1")

	_, err := ExecuteDropSink(context.Background(), "bafy/big", "big.bin", hd,
		"5m", 10, func(_ context.Context, w io.Writer) error {
			_, werr := w.Write(bytes.Repeat([]byte("a"), 100))
			return werr
		})
	require.Error(t, err, "over-cap download must fail at mint time")
	require.True(t, transfer.IsDownloadTooLarge(err))
}

// TestDropSinkGetStreamsBytesOnce verifies the minted GET serves the full
// pre-buffered bytes with a correct Content-Length and cleans up the temp file.
func TestDropSinkGetStreamsBytesOnce(t *testing.T) {
	payload := []byte("pdf-bytes-1-2-3")
	hd := transfer.NewHTTPDownload()
	defer hd.Stop(context.Background())

	// Wire into a real loopback via httptest so we can actually GET it.
	res, err := ExecuteDropSink(context.Background(), "bafy/a.pdf", "a.pdf", hd,
		"5m", 0, func(_ context.Context, w io.Writer) error {
			_, werr := w.Write(payload)
			return werr
		})
	require.NoError(t, err)
	require.Equal(t, int64(len(payload)), res.Size)

	mux := http.NewServeMux()
	hd.RegisterHandlers(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/download/" + tokenFromURL(t, res.FetchURL))
	require.NoError(t, err)
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, payload, got, "GET must stream the full bytes")
	require.Equal(t, int64(len(payload)), resp.ContentLength, "Content-Length must match the real size")
}

// TestExecuteDropSinkDefaultsReportedTTL pins the reporting fix: an omitted or
// invalid ttl must surface as the applied default duration, never "0s".
func TestExecuteDropSinkDefaultsReportedTTL(t *testing.T) {
	cases := []string{"", "bogus", "0s", "-1m"}
	for _, ttl := range cases {
		hd := transfer.NewHTTPDownload()
		hd.SetBaseURL("http://127.0.0.1")
		res, err := ExecuteDropSink(context.Background(), "bafy/a", "a.bin", hd,
			ttl, 0, func(_ context.Context, w io.Writer) error {
				_, werr := w.Write([]byte("x"))
				return werr
			})
		require.NoError(t, err)
		require.Equal(t, transfer.DefaultHTTPDownloadTTL.String(), res.TTL,
			"ttl %q: reported TTL must be the applied default", ttl)
		hd.Stop(context.Background())
	}
}

// TestResolveDownloadRootPrefersSupplierAndDefaults pins the fallback chain:
// an operator supplier wins; a nil/empty supplier falls back to the config
// default download root.
func TestResolveDownloadRootPrefersSupplierAndDefaults(t *testing.T) {
	require.Equal(t, "/data/downloads", ResolveDownloadRoot(func() string { return "/data/downloads" }))
	require.NotEmpty(t, ResolveDownloadRoot(nil), "nil supplier must fall back to the config default")
	require.NotEmpty(t, ResolveDownloadRoot(func() string { return "" }))
}

// tokenFromURL extracts the trailing download token from a minted fetch URL.
func tokenFromURL(t *testing.T, u string) string {
	t.Helper()
	i := 0
	for j, c := range u {
		if c == '/' {
			i = j
		}
	}
	require.NotZero(t, i, "fetch URL must include a token path")
	return u[i+1:]
}
