package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.lumeweb.com/mcpplane/transfer"

	"go.lumeweb.com/pinner/core/config"
	"go.lumeweb.com/pinner/core/download"
)

// IPFSDownloadHandler streams a single IPFS node (CID or CID/path) to dest.
// It is the authenticated IPFS download executor — the counterpart of
// transfer.UploadHandler: the tool never decides the mechanism, it hands the
// resolver a sink and this handler provides the source bytes for the local or
// filedrop branch.
type IPFSDownloadHandler func(ctx context.Context, ipfsPath string, w io.Writer) error

// StreamDownload returns the IPFS download executor over a core/download
// Service: it streams a single IPFS node (CID or CID/path) to w via the
// service's Cat. Callers build the concrete download.Service from their
// config manager; this executor only owns the Cat→io.Copy mechanics shared
// by every surface. The auth gate lives in Cat (context-aware), which never
// rejects an authenticated hosted caller.
func StreamDownload(downloadSvc download.Service) IPFSDownloadHandler {
	return func(ctx context.Context, ipfsPath string, w io.Writer) error {
		reader, err := downloadSvc.Cat(ctx, ipfsPath)
		if err != nil {
			return err
		}
		defer reader.Close()
		_, err = io.Copy(w, reader)
		return err
	}
}

// DownloadSinkInput is the shared argument shape for how a download's bytes are
// delivered. A single `sink` union plus the local output path / filedrop TTL
// mirrors how upload_file carries its transport-scoped source.
type DownloadSinkInput struct {
	// Sink is where the retrieved bytes land: "local" writes to a host-side
	// output path (available on every transport); "drop" mints a one-time
	// HTTP GET filedrop the consumer pulls (only when a reachable HTTP mux
	// exists). See DownloadSink / DownloadSinksAllowed.
	Sink transfer.DownloadSink `json:"sink"`
	// OutputPath is the host-side destination file path for sink=local. When
	// sink=local and output_path is omitted, the server picks a default in the
	// process working directory from the source name. Required before a local
	// write can proceed; unavailable sinks are rejected up front.
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Host-side destination file path for sink=local (e.g. /data/out/report.pdf). Required for local sink unless a default directory is configured."`
	// TTL is the filedrop GET lifetime for sink=drop (e.g. 5m; default 5
	// minutes). Only used with sink=drop.
	TTL string `json:"ttl,omitempty" jsonschema:"description=Filedrop GET endpoint lifetime for sink=drop (e.g. 5m; default 5 minutes)."`
}

// DownloadSinksAllowed reports whether the requested sink is one the running
// server honors, given whether a filedrop coordinator is wired and whether the
// transport is the OpenAI tunnel (no reachable mux). It is the per-invocation
// gate mirroring the registration-time capability report (transfer's
// downloadSinksFor / SinkEnumValues).
func DownloadSinksAllowed(sink transfer.DownloadSink, dropWired, tunnelOpenAI bool) error {
	if !sink.Valid() {
		return fmt.Errorf("unknown sink %q (valid: local, drop)", sink)
	}
	for _, s := range transfer.SinkEnumValues(dropWired, tunnelOpenAI) {
		if s == string(sink) {
			return nil
		}
	}
	if sink == transfer.SinkDrop {
		return fmt.Errorf("sink %q is not available on this transport: no reachable filedrop GET endpoint", sink)
	}
	return fmt.Errorf("sink %q is not available on this transport", sink)
}

// DefaultSourceName is a shared fallback for an empty derived name.
const DefaultSourceName = "download"

// SinkDefaultName derives a filesystem-safe base filename from an IPFS path or
// vault path for the local-sink default output. It returns the last path
// segment (striping any /ipfs/<cid>/ prefix and vault path) or "download".
func SinkDefaultName(source string) string {
	// Strip a vault path (vault:/... or vault:<profile>/...) authority stem.
	base := source
	if idx := strings.Index(base, ":/"); idx >= 0 {
		base = base[idx+2:]
	}
	// Strip a /ipfs/<cid>/ slash-path prefix: take everything after the last
	// slash that follows the CID, or just the last segment.
	if i := strings.LastIndex(base, "/"); i >= 0 && i+1 < len(base) {
		base = base[i+1:]
	}
	base = transfer.SanitizeFilename(base)
	if base == "download" {
		return base
	}
	return base
}

// ResolveLocalOutputPath confines a caller-supplied (or absent) output path for
// sink=local to a configured download root, resolving it to a concrete host
// path and rejecting any attempt to escape the root.
//
// Security invariant: local-sink writes are confined to downloadRoot. The
// caller's output_path is a RELATIVE path within the root (subdirectories are
// allowed and created); an absolute path, a drive root, or any path whose
// cleaned lexical form escapes the root (via ".." or a Windows drive/cross-drive
// input) is rejected, as is any path whose real filesystem resolution escapes
// the root through a pre-existing symbolic link. This prevents a compromised
// MCP agent from overwriting arbitrary server files or redirecting decrypted
// vault/IPFS content elsewhere on the host — the mirror of upload's gating.
//
// Implementation: `filepath.Join(root, rel)` replaces the root when rel is
// absolute (or a Windows volume path), and lexically resolves `..`; the returned
// path is confined only if `filepath.Rel(root, joined)` stays inside root (does
// not start with ".."). That lexical check rejects absolute inputs, drive
// inputs, and `..` traversal alike.
//
// The lexical check alone is not sufficient, because the downstream write
// (WriteLocalDownload's MkdirAll / CreateTemp / Rename) resolves intermediate
// directory components THROUGH the filesystem: a pre-existing symlink inside
// the root pointing outside it would silently carry the destination — and the
// downloaded bytes — out of the root. After the lexical check, containment is
// therefore re-verified against the destination's REAL path (existing
// components resolved via EvalSymlinks); any resolution outside the root's
// real path, or an unresolvable component, is rejected.
//
// It never writes — it only decides the destination and validates containment
// (the symlink resolution behind that validation is read-only); the returned
// error rejects the request before any byte is read or written.
func ResolveLocalOutputPath(downloadRoot, outputPath, sourceName string) (string, error) {
	name := SinkDefaultName(sourceName)
	if name == "" {
		name = DefaultSourceName
	}
	root := filepath.Clean(downloadRoot)
	if root == "" || root == "." {
		return "", fmt.Errorf("download root is not configured")
	}
	rel := outputPath
	if rel == "" || rel == "." || rel == string(filepath.Separator) ||
		strings.HasSuffix(rel, "/") || strings.HasSuffix(rel, string(filepath.Separator)) {
		// Absent path, or an explicit "this directory": either the root itself
		// or a path ending in a separator ("reports/") is a DIRECTORY — the
		// source-derived filename is appended inside it (mirroring vault cp's
		// directory-expansion). Without the trailing-separator case a "sub/"
		// input would collapse to a bare file destination and clobber it.
		rel = filepath.Join(rel, name)
	}
	// Reject any caller-supplied ABSOLUTE path outright. filepath.Join does not
	// reset on a leading separator (Join("a", "/b") = "a/b"), so an absolute
	// input would otherwise silently collapse INTO the root; a Windows
	// volume/drive path (C:\... or a bare "C:") is likewise never a valid
	// relative target. Because filepath.IsAbs on Windows only treats paths with
	// a drive letter as absolute (a POSIX-style "/etc/..." or a bare "\\x"
	// root-relative path is reported relative on the current drive), we also
	// reject any rel that begins with a path separator — such a path is never a
	// valid relative destination and must not be merged.
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" ||
		strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return "", fmt.Errorf("download output path %q must be relative to the configured download root %q", outputPath, root)
	}
	// Clean both the requested path and the root, then join.
	clean := filepath.Clean(filepath.Join(root, rel))
	// The candidate is confined if and only if it is lexically inside root.
	relFromRoot, err := filepath.Rel(root, clean)
	if err != nil || relFromRoot == ".." || strings.HasPrefix(relFromRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relFromRoot) {
		return "", fmt.Errorf("download output path %q escapes the configured download root %q", outputPath, root)
	}
	// The lexical check cannot see through symlinks, so the candidate is
	// re-verified against its real filesystem resolution before the caller is
	// handed a path the write machinery will follow component-by-component.
	if err := ensureRealPathInsideRoot(root, clean); err != nil {
		return "", err
	}
	return clean, nil
}

// resolveRealPath resolves path to its effective filesystem location: the
// deepest EXISTING ancestor is resolved through any symlinks via
// filepath.EvalSymlinks, and the remaining non-existent tail components are
// re-appended lexically (they cannot yet be symlinks, so there is nothing on
// disk to resolve). Walking up to the deepest existing ancestor — rather than
// failing when path itself does not exist — lets a destination inside a
// not-yet-created subdirectory of a symlinked root resolve consistently on
// both sides of the containment comparison.
func resolveRealPath(path string) (string, error) {
	probe := filepath.Clean(path)
	tail := ""
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		tail = filepath.Join(filepath.Base(probe), tail)
		probe = parent
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil {
		// An existing but unresolvable component (e.g. a broken symlink in the
		// ancestry) means the effective location is unknown: fail closed
		// rather than assume containment the write would traverse anyway.
		return "", err
	}
	return filepath.Join(resolved, tail), nil
}

// ensureRealPathInsideRoot is the filesystem half of the local-sink
// confinement invariant. The lexical containment check cannot see through
// symlinks, but WriteLocalDownload resolves intermediate directory components
// through the filesystem (MkdirAll / CreateTemp / Rename), so a pre-existing
// root-internal symlink pointing outside downloadRoot would carry the write —
// and the downloaded bytes — outside the root. Both the root and the
// destination are resolved to their real locations and containment is
// re-checked there; a component whose real location cannot be determined
// fails closed.
func ensureRealPathInsideRoot(root, dest string) error {
	realRoot, err := resolveRealPath(root)
	if err != nil {
		return fmt.Errorf("cannot resolve the configured download root %q: %w", root, err)
	}
	realDest, err := resolveRealPath(dest)
	if err != nil {
		return fmt.Errorf("download output path %q cannot be resolved inside the configured download root %q: %w", dest, root, err)
	}
	relFromRoot, err := filepath.Rel(realRoot, realDest)
	if err != nil || relFromRoot == ".." || strings.HasPrefix(relFromRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relFromRoot) {
		return fmt.Errorf("download output path %q resolves outside the configured download root %q via a symbolic link", dest, root)
	}
	return nil
}

// WriteLocalDownload streams the source bytes to a host-side output path
// atomically: it writes to a temp file in the destination directory, then
// renames onto the final path only after the stream succeeds, so a failed or
// interrupted download never leaves a truncated file as if it were complete.
// The destination directory is created if missing. An existing destination is
// overwritten by the rename (the caller is expected to gate on --force-style
// semantics at the tool boundary if desired).
func WriteLocalDownload(ctx context.Context, outputPath string, maxBytes int64, resolve func(ctx context.Context, w io.Writer) error) (int64, error) {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create destination directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".pinner-dl-*")
	if err != nil {
		return 0, fmt.Errorf("create temp download file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after successful rename
	// Enforce the download size cap at the stream: over-limit bytes fail
	// loudly (and the temp file is removed by the defer) rather than landing a
	// truncated file as if it were complete.
	if err := resolve(ctx, transfer.NewSizeLimitedWriter(tmp, maxBytes)); err != nil {
		tmp.Close()
		return 0, err
	}
	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return 0, fmt.Errorf("finalize download: %w", err)
	}
	return info.Size(), nil
}

// DownloadResult is the canonical envelope returned by download tools.
type DownloadResult struct {
	Status   string `json:"status"`
	Source   string `json:"source"`
	Sink     string `json:"sink"`
	Output   string `json:"output_path,omitempty"` // sink=local
	FetchURL string `json:"fetch_url,omitempty"`   // sink=drop
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	TTL      string `json:"ttl,omitempty"` // sink=drop
	Error    string `json:"error,omitempty"`
}

// ExecuteLocalSink resolves bytes to a root-confined host path and returns a
// local result. It rejects any output path that escapes downloadRoot.
func ExecuteLocalSink(ctx context.Context, source, sourceName, outputPath, downloadRoot string, maxBytes int64, resolve func(ctx context.Context, w io.Writer) error) (DownloadResult, error) {
	final, err := ResolveLocalOutputPath(downloadRoot, outputPath, sourceName)
	if err != nil {
		return DownloadResult{}, err
	}
	size, err := WriteLocalDownload(ctx, final, maxBytes, resolve)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{
		Status: "ok",
		Source: source,
		Sink:   string(transfer.SinkLocal),
		Output: final,
		Name:   filepath.Base(final),
		Size:   size,
	}, nil
}

// ExecuteDropSink mints a one-time filedrop GET and returns a drop result. The
// ttl string is parsed; when omitted/invalid/<=0 the default is applied so the
// reported TTL matches what mint actually enforces (mint does the same default
// internally, but reporting "0s" would mislead a consumer into treating a live
// endpoint as already expired).
//
// The source bytes are resolved once into a temp file at mint time so the
// result can report the size the consumer is about to pull AND the GET can
// stream with an accurate Content-Length (a size:0 drop result is a silent
// lie, and a byte-stale size is worse). The cap is enforced during that
// write, so an over-limit download fails here, up front, rather than serving
// a truncated file. The temp file lives for the endpoint TTL and is removed
// by the GET handler after it is streamed once (or expired).
func ExecuteDropSink(ctx context.Context, source, sourceName string, hd *transfer.Download, ttl string, maxBytes int64, resolve func(ctx context.Context, w io.Writer) error) (DownloadResult, error) {
	if hd == nil {
		return DownloadResult{}, errors.New("filedrop GET coordinator is not configured for sink=drop")
	}
	var d time.Duration
	if ttl != "" {
		if parsed, err := time.ParseDuration(ttl); err == nil && parsed > 0 {
			d = parsed
		}
	}
	if d <= 0 {
		d = transfer.DefaultHTTPDownloadTTL
	}

	tmp, err := os.CreateTemp("", ".pinner-drop-*")
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create filedrop temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// Cleanup on any error path (including an over-cap resolve, which aborts
	// before the endpoint is minted). Once minted, the GET handler owns the
	// file and removes it after it is served / on expiry.
	removeOnErr := true
	defer func() {
		if removeOnErr {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := resolve(ctx, transfer.NewSizeLimitedWriter(tmp, maxBytes)); err != nil {
		tmp.Close()
		return DownloadResult{}, err
	}
	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		return DownloadResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return DownloadResult{}, err
	}

	// Serve the already-resolved file. The temp file is removed by the cleanup
	// closure the GET handler / expiry path invokes exactly once the token is
	// consumed or expires.
	serve := func(ctx context.Context, w io.Writer) error {
		f, err := os.Open(tmpPath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}
	fetchURL, err := hd.Mint(ctx, sourceName, info.Size(), serve, d, func() {
		_ = os.Remove(tmpPath)
	})
	if err != nil {
		return DownloadResult{}, err
	}
	removeOnErr = false
	return DownloadResult{
		Status:   "ok",
		Source:   source,
		Sink:     string(transfer.SinkDrop),
		FetchURL: fetchURL,
		Name:     sourceName,
		Size:     info.Size(),
		TTL:      d.String(),
	}, nil
}

// ResolveDownloadRoot returns the host directory confining download_file
// local-sink writes. It prefers the operator-supplied supplier (from the
// surface's WithDownloadRoot option), falling back to the config default
// (<config-dir>/downloads). The value is returned verbatim — Clean/containment
// is applied by ResolveLocalOutputPath at invocation time.
func ResolveDownloadRoot(supplier func() string) string {
	if supplier != nil {
		if r := supplier(); r != "" {
			return r
		}
	}
	return config.DefaultDownloadRoot()
}
