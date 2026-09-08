// Package pinnertransfer provides the Pinner/IPFS upload and download
// EXECUTORS over the transport-neutral coordination layer of
// go.lumeweb.com/mcpplane/transfer, per the package-boundaries overhaul
// (00-package-boundaries.md §4).
//
// The split mirrors mcpplane's own: mcpplane/transfer owns the transport-facing
// coordination (HTTP upload/download coordinators, task manager, source/sink
// contracts); pinnertransfer owns the content-side behavior that talks to the
// Pinner services — the stream->upload executor (temp-file buffering, wrap
// sniffing, archive-convert extraction) and the download sink executor (root-
// confined local writes, pre-buffered filedrops). Both halves meet on the
// transfer.UploadHandler and IPFSDownloadHandler contracts.
//
// # Coding conventions
//
//   - This package is a hard boundary: it imports only the standard library,
//     go.lumeweb.com/mcpplane/transfer, go.lumeweb.com/ipfs-content
//     (fs + archive), and this module's core/{uploads,download,config}. It never
//     imports a CLI formatter, command framework, or MCP SDK package; tool
//     descriptors and vault handling stay in pinner-cli per the boundary doc.
//   - No CLI presentation layer: executors return plain values/errors; callers
//     (pinner-cli) own output formatting.
package pinnertransfer

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	contentArchive "go.lumeweb.com/ipfs-content/archive"
)

// ArchiveMode mirrors the portal plugin's archive decision.
type ArchiveMode string

const (
	// ArchiveConvert extracts the archive and uploads its contents.
	ArchiveConvert ArchiveMode = "convert"
	// ArchivePreserve keeps the archive file as-is.
	ArchivePreserve ArchiveMode = "preserve"
)

// ParseArchiveMode parses "convert"/"preserve"; defaults to ArchiveConvert.
func ParseArchiveMode(s string) ArchiveMode {
	switch s {
	case "preserve":
		return ArchivePreserve
	case "convert":
		return ArchiveConvert
	default:
		return ArchiveConvert
	}
}

// SniffArchive sniffs a reader's container format and reports whether it is an
// extractable archive. It reuses contentArchive.DetectFormat. The reader must
// implement io.ReadSeeker (bytes.Reader and *os.File both do); detection seeks
// back to the start.
func SniffArchive(r io.Reader) (contentArchive.Format, bool, error) {
	f, err := contentArchive.DetectFormat(r)
	if err != nil {
		return f, false, err
	}
	return f, f.IsArchiveFormat(), nil
}

// readerAtSeeker is the interface contentArchive.CreateExtractor needs
// (io.Reader + io.ReaderAt + io.Seeker). *os.File and *bytes.Reader satisfy it.
type readerAtSeeker interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

// OpenArchiveFS opens an archive into an fs.FS view of its entries via
// contentArchive.CreateExtractor(...).Filesystem(ctx). The returned closer must
// be called to release extractor resources. reader must satisfy the
// ReaderAtSeeker interface (io.Reader + io.ReaderAt + io.Seeker); *os.File does.
func OpenArchiveFS(ctx context.Context, reader io.Reader) (fs.FS, func() error, error) {
	rsc, ok := reader.(readerAtSeeker)
	if !ok {
		return nil, nil, fmt.Errorf("reader does not implement archives.ReaderAtSeeker")
	}
	ext, err := contentArchive.CreateExtractor(rsc)
	if err != nil {
		return nil, nil, err
	}
	f, err := ext.Filesystem(ctx)
	if err != nil {
		_ = ext.Close()
		return nil, nil, err
	}
	return f, ext.Close, nil
}
