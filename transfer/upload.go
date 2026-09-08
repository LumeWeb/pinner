package transfer

import (
	"context"
	"io"
	"os"

	contentfs "go.lumeweb.com/ipfs-content/fs"

	"go.lumeweb.com/mcpplane/transfer"

	"go.lumeweb.com/pinner/core/uploads"
)

// StreamUpload returns the shared IPFS stream→upload executor: the single
// implementation of the authenticated upload contract for a stream source
// (mcpplane/transfer's core executor contract, transfer.UploadHandler). It is
// the content-side half of pinner-cli's streamUploadHandler: it buffers the
// stream to a temp file, applies the wrap/archive rules, and proxies into the
// uploads.Service. It is IPFS-only — no vault — and never formats output;
// callers own tool descriptors and presentation.
//
// Behavior (preserved verbatim from pinner-cli internal/cli/streamUploadHandler):
//
//   - An empty name falls back to transfer.DefaultUploadName.
//   - A wrapped (website) single-file upload with no explicit name sniffs the
//     content: HTML becomes index.html so the site resolves at its root
//     (transfer.ResolveWrappedFileName). The sniffed head bytes are written to
//     the temp file first so io.Copy appends the remainder without dropping the
//     content consumed during sniffing.
//   - archive_mode=convert (the default) on a stream source: the buffered temp
//     file is sniffed and, when it is an extractable archive, opened as an
//     fs.FS tree, size-checked against maxBytes (aggregate policy, mirroring
//     the whole-body caps of the relay/DataURI/curl surfaces) and uploaded as a
//     directory DAG with wrap=False (it is already a directory root). A
//     sniff/extract failure falls back to the raw single-file upload.
//   - Otherwise the stream uploads as a single file via
//     contentfs.NewSingleFileFS, honoring wait and wrap.
//
// maxBytes is the operator-set upload cap applied to an extracted archive's
// total size (core/config Config.GetMaxMCPUploadSize, converted to int64 by
// the caller); <= 0 disables the tree-size check. The raw single-file path is
// NOT capped here — that cap is enforced upstream at the transport/body level.
func StreamUpload(uploadSvc uploads.Service, maxBytes int64) transfer.UploadHandler {
	return func(ctx context.Context, reader io.Reader, size int64, name string, wait bool, archiveMode string, wrap bool) (any, error) {
		if name == "" {
			name = transfer.DefaultUploadName
		}
		file, err := os.CreateTemp("", "pinner-mcp-upload-*")
		if err != nil {
			return nil, err
		}
		path := file.Name()
		defer os.Remove(path)
		defer file.Close()
		// A wrapped (website) single-file upload with no explicit name sniffs the
		// content: HTML becomes index.html so the site resolves at its root. The
		// sniffed head bytes are written to the temp file first so io.Copy appends
		// the remainder without dropping the content consumed during sniffing.
		if wrap && (name == "" || name == transfer.DefaultUploadName) {
			var head [512]byte
			n, _ := io.ReadFull(reader, head[:])
			if resolved := transfer.ResolveWrappedFileName(name, true, head[:n]); resolved != "" {
				name = resolved
			}
			if n > 0 {
				if _, err := file.Write(head[:n]); err != nil {
					return nil, err
				}
			}
		}
		if _, err := io.Copy(file, reader); err != nil {
			return nil, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		// archive_mode=convert (default) on a stream source: sniff the buffered
		// temp file and, when it is an archive, extract it into a directory DAG
		// rather than uploading the raw archive as a single file.
		if ParseArchiveMode(archiveMode) == ArchiveConvert {
			if _, isArc, serr := SniffArchive(file); serr == nil && isArc {
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					return nil, err
				}
				vfs, closer, aerr := OpenArchiveFS(ctx, file)
				if aerr == nil {
					defer closer()
					if err := CheckTreeSize(vfs, maxBytes, TreeSizeAggregate); err != nil {
						return nil, err
					}
					return uploadSvc.Upload(ctx, vfs, name, wait, false)
				}
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
		}
		result, err := uploadSvc.Upload(ctx, contentfs.NewSingleFileFS(file, name), name, wait, wrap)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}
