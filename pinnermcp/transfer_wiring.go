package pinnermcp

import (
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/transfer"

	pinnertransfer "go.lumeweb.com/pinner/transfer"
)

// TransferDeps carries the transfer-tool wiring: the executor function types
// and coordinator pointers that live in mcpplane/transfer (the transport-
// neutral coordination layer) and pinnertransfer (the Pinner content-side
// executors), plus the registration-time flags the capabilities tool must
// stay honest about. All fields are optional: a zero field means the
// corresponding capability is simply not registered and never advertised.
//
// This is the injected-function-type boundary the port defined: the
// descriptor halves (pinnermcp) never construct coordinators or services —
// the composition root wires them (pinnertransfer.StreamUpload / StreamDownload
// over the CLI-built services, the HTTP upload/download coordinators it serves
// on its mux) and hands them in. The historic package-global transport flags
// (SetTransportFlags) are superseded by these fields.
type TransferDeps struct {
	// Wiring flags that classify the transport (see UploadFileTransport).
	CoLocated    bool
	TunnelOpenAI bool

	// Executors: the Pinner content-side edges, injected by the composition
	// root. PathUpload serves upload_file's stdio path branch; Relay serves
	// the OpenAI file/url/data relays and the host `file` handoff;
	// IPFSDownload serves download_file's local and drop sinks.
	PathUpload   transfer.LocalPathUploadHandler
	Relay        transfer.UploadHandler
	IPFSDownload pinnertransfer.IPFSDownloadHandler

	// Coordinators. PresignedUpload mints the presigned PUT endpoints for the
	// HTTP transport (nil disables the mint branch); FileDrop mints one-time
	// GET filedrops for sink=drop (nil disables the drop sink).
	PresignedUpload *transfer.Upload
	FileDrop        *transfer.Download

	// DownloadRoot confines sink=local outputs; MaxDownloadBytes caps a
	// buffered filedrop. MaxRelayBytes caps relayed (url/data/file) bytes
	// (0 = the ieo default relay cap).
	DownloadRoot      string
	MaxDownloadBytes  int64
	MaxRelayBytes     int64
	RelayAllowedHosts []string

	// Tool registration flags — mirrored into the capabilities descriptor so
	// its report stays identical to what tools/list actually exposed.
	// DropWired is the caller's explicit drop registration decision (Assemble
	// ORs it with FileDrop != nil). VaultPutFile/VaultGetFile are not ported
	// (the vault transfer tools stay CLI-side) but the flags exist so an
	// assembly can still report honestly about them.
	UploadFile   bool
	VaultPutFile bool
	DownloadFile bool
	VaultGetFile bool
	DropWired    bool
	// RelayURLWired / DataURIWired: whether the upload_url / upload_data relay
	// tools are registered. RelayFeatures is the registration-time effective
	// feature set that decided it.
	RelayURLWired bool
	DataURIWired  bool
	RelayFeatures mcpforge.FeatureSet
}

// RelayURLRegistered is the SINGLE honest registration/advertising gate for
// the upload_url relay tool: the caller's explicit wiring decision
// (RelayURLWired) AND the relay executor actually wired (Relay != nil) AND
// the registration-time effective feature set declaring the server-fetch
// relay feature (FeatSourceURL). Both buildDirectTools's registration branch
// AND the capabilities tool's upload_tools report MUST consume this one
// method, so a relay tool can never be advertised without being registered
// (the pinnermcp C1 regression) nor registered without being advertised.
func (t TransferDeps) RelayURLRegistered(features mcpforge.FeatureSet) bool {
	return t.RelayURLWired && t.Relay != nil && features.Has(FeatSourceURL)
}
