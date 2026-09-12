package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/invopop/jsonschema"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/ieo"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/toolargs"
	"go.lumeweb.com/mcpplane/transfer"
	pinnertransfer "go.lumeweb.com/pinner/transfer"
)

// Transfer-tool descriptors: the descriptor halves of the unified transfer
// tools (upload_file / upload_data / download_file). The coordinators and
// executors live in mcpplane/transfer (source/sink contracts, HTTP
// upload/download coordinators, task manager) and pinnertransfer (the Pinner
// content-side executors). This file injects them via the plain function types
// (transfer.LocalPathUploadHandler, transfer.UploadHandler,
// pinnertransfer.IPFSDownloadHandler) and the wireable coordinator pointers
// (*transfer.Upload, *transfer.Download).

// UploadFileInput is the typed argument shape for the unified upload_file tool.
// Exactly one byte source must be provided per invocation: either the
// OpenAI/host-provided `file` reference (a generated artifact the host hands
// over as a temporary download_url) or the transport-scoped `source`. When
// `source` is used, the tool routes to the real file-input mechanism based on
// the server's transport — the caller never picks a mechanism.
type UploadFileInput struct {
	// Source is the file to upload. Mode must be valid for the running
	// transport: path=co-located stdio; mint=HTTP/tunnel (returns a presigned
	// curl PUT URL); url/data=OpenAI tunnel (relayed through MCP). Omit when a
	// host-provided `file` reference is used; see capabilities.source_modes for
	// the modes valid on this transport.
	Source *transfer.UploadSource `json:"source,omitempty" jsonschema:"description=The file to upload, as a transport-scoped source object. Omit when a host-provided file reference is used instead; the valid mode values for this transport are listed in capabilities.source_modes (path=co-located stdio, mint=HTTP/tunnel presigned endpoint, url/data=relay)."`
	// File is a host-provided file reference (temporary download_url +
	// file_id). It enables a ChatGPT user to hand a file — including
	// assistant-generated files in the assistant's sandbox — directly to Pinner
	// without a human file-picker or manual transport. The OpenAI runtime
	// resolves the file reference into the download_url + file_id structure;
	// the agent must NOT construct this object itself, base64-encode the
	// file, or create a data URI when `file` can be used. Mutually exclusive
	// with Source.
	File *transfer.ChatGPTFileInput `json:"file,omitempty" jsonschema:"description=Host-provided file reference ({download_url, file_id}). Only valid when this host can actually supply one — check capabilities.host_file_input (most non-OpenAI hosts, including Grok, have it false). When host_file_input is true, pass the reference for a file the host runtime already holds instead of a source. When it is false, this property does not apply: use a transport-scoped source (e.g. source.mode=mint for HTTP) instead of constructing download_url/file_id manually, base64-encoding the file, or minting a presigned URL."`
	// Name is the upload label (defaults to the source name or 'upload').
	Name string `json:"name,omitempty" jsonschema:"description=Optional upload name (defaults to the file name)."`
	// Wait waits for this upload's own pin operation to complete.
	Wait bool `json:"wait,omitempty" jsonschema:"description=Wait until this upload's own pin operation completes before returning (the upload already pins; this only controls whether the call blocks for it)."`
	// ArchiveMode controls how an archive (host `file`, url/data relay, or
	// path) is handled. 'convert' (default) extracts an archive and uploads its
	// contents as a directory DAG while preserving relative paths. Use
	// 'convert' for complete static website ZIPs containing index.html, CSS,
	// JS, images, and nested directories; the resulting CID is a directory CID
	// ready for websites_create/update. 'preserve' keeps the archive intact as
	// a single file. Honored on every source: host file, path, and url/data
	// relays default to convert and route through a buffering executor
	// directly; the mint (presigned PUT) source records the mode on the handle
	// at mint time and applies it when the PUT bytes arrive, but DEFAULTS to
	// preserve — only an explicit convert extracts a streamed archive.
	ArchiveMode string `json:"archive_mode,omitempty" jsonschema:"enum=convert,enum=preserve,description=How to treat an archive. convert extracts an archive and uploads its contents as a directory DAG while preserving relative paths; use for complete static website ZIPs (index.html, CSS, JS, images, nested directories) — the resulting CID is a directory CID ready for websites_create/update. The archive's directory structure is preserved exactly, so index.html sits at the archive root (not inside a wrapper directory). preserve keeps the archive intact as a single file. IMPORTANT: the default depends on the source. Host-file, path, and url/data sources default to convert; the mint (presigned PUT) source defaults to preserve and ONLY converts when archive_mode=convert is passed explicitly. A website ZIP streamed via source.mode=mint therefore needs archive_mode=convert, or it uploads as a raw single-file CID and websites_create will reject it."`
	// TTL is the presigned endpoint lifetime (e.g. 5m). Only used on transports
	// that use presigned PUT endpoints (HTTP/tunnel).
	TTL string `json:"ttl,omitempty" jsonschema:"description=Presigned endpoint lifetime (e.g. 5m; default 5 minutes). Only used on transports that use presigned PUT endpoints."`
	// Wrap forces a directory root when uploading a single file, required for
	// content that will be a website (a website must resolve to a directory,
	// not a bare file). Only affects single-file uploads (file / url / data /
	// path to a file, or a mint PUT whose bytes are not an archive); directory
	// and archive-converted uploads are already a directory root.
	Wrap bool `json:"wrap,omitempty" jsonschema:"description=Wrap a single file in a directory root so the CID is a directory (required when the upload is a website). When wrap=true and no name is given, HTML content is auto-named index.html so the site resolves at its root. An explicit name such as 'starter-site' is honored as-is and the page is then only reachable at /starter-site, not /. Only affects single-file uploads; directories and archive-converted uploads are already a directory root."`
}

// DataURIUploadInput is the typed argument shape for upload_data.
// The File field has no omitempty tag, so the jsonschema reflector marks it
// required (matching the wizard step-input convention).
type DataURIUploadInput struct {
	File string `json:"file" jsonschema:"format=uri,description=RFC 2397 data: URI with a base64-encoded payload, e.g. data:;base64,<base64 payload> or data:text/plain;base64,<base64 payload>; optional ;name=<name>;size=<n> parameters are accepted but not required. The payload is base64. The bytes do not enter the model context; the host supplies this value from a user-attached file."`
	Name string `json:"name,omitempty" jsonschema:"description=Optional upload name (defaults to the data URI name, else 'upload')."`
	Wait bool   `json:"wait,omitempty" jsonschema:"description=Wait until this upload's own pin operation completes before returning (the upload already pins; this only controls whether the call blocks for it)."`
	Wrap bool   `json:"wrap,omitempty" jsonschema:"description=Wrap the single file in a directory root so the resulting CID is a directory. Required when the upload is a website (a website resolves to a directory, not a bare file). When wrap=true and no name is given, HTML content is auto-named index.html so the site resolves at its root. An explicit name such as 'starter-site' is honored as-is and the page is then only reachable at /starter-site, not /. True only affects single-file uploads; directory uploads are already a directory root."`
}

// DownloadFileInput is the typed argument shape for the unified download_file
// tool. The caller supplies an IPFS path (CID or CID/path) plus a sink telling
// where the retrieved bytes should land. The tool routes to the real delivery
// mechanism based on the server's configured sinks — the caller never picks a
// mechanism.
type DownloadFileInput struct {
	// IPFSPath is the content to download, a CID or CID/subpath.
	IPFSPath string `json:"ipfs_path" jsonschema:"description=IPFS content to download, a CID or CID/path (e.g. bafy.../subdir/file.txt). Required."`
	// Sink tells where the bytes land: "local" writes to a host-side path
	// (available on every transport); "drop" mints a one-time HTTP GET
	// filedrop (only when a reachable HTTP mux exists).
	Sink transfer.DownloadSink `json:"sink" jsonschema:"enum=local,enum=drop,description=Where the downloaded bytes land: local writes to a host-side output_path on the MCP server's disk (available on every transport); drop mints a one-time HTTP GET filedrop link to pull from out of band."`
	// Name is an optional filename override for the downloaded file (used for
	// the local output name and the filedrop attachment name). Defaults to the
	// last path segment of ipfs_path.
	Name string `json:"name,omitempty" jsonschema:"description=Optional filename for the downloaded file (defaults to the last segment of ipfs_path)."`
	// OutputPath is the destination for sink=local, resolved RELATIVE to the
	// configured download root (default <config-dir>/downloads).
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Destination path for sink=local, relative to the configured download root (subdirectories are created). If omitted, the source name is used at the root. Paths that escape the root are rejected."`
	// TTL is the filedrop GET lifetime for sink=drop (e.g. 5m; default 5m).
	TTL string `json:"ttl,omitempty" jsonschema:"description=Filedrop GET endpoint lifetime for sink=drop (e.g. 5m; default 5 minutes)."`
}

// ---
// Vendored forge description builders.
//
// mcpforge is consumer-agnostic (feature vocabulary is owned by the
// consumer), so the per-tool description targets for upload_file,
// download_file, and upload_data live here, expressed over HostProfile, with
// the transport gates expressed as HostProfile predicates.
// ---

// uploadFileDesc composes the upload_file tool description from a static
// preamble plus feature-gated segments. At resolution time only
// segments whose required features are satisfied by the profile are
// concatenated.
var uploadFileDesc = mcpforge.Static[HostProfile](
	"Upload a file and pin it. The returned CID is already pinned, so importing it again with pins_add is unnecessary; the wait flag waits for this upload's own pin operation.",
).
	When(FeatFileHostInput,
		"Use `file` when the host already has the file (user-uploaded attachments AND assistant-generated files in the assistant's sandbox); the OpenAI runtime converts it to a temporary download_url + file_id this tool receives — the file is passed as-is, without base64 encoding, a data URI, or manually constructing the download_url object.",
	).
	When(FeatSourceMint,
		"Use source.mode=mint to get a one-time presigned HTTP PUT endpoint. Mint does NOT store bytes: PUT your agent-local file to the returned url (curl -sS -T <file> \"<url>\"), then poll upload_status with the returned upload_handle until it reports completed — the completed CID is already pinned, so pins_add is unnecessary. For a website ZIP, mint holds the bytes as a raw archive unless you pass archive_mode=convert, so always pass archive_mode=convert for a site ZIP (or wrap=true for a single HTML page).",
	).
	When(FeatSourcePath,
		"Use source.mode=path with a host-side file/directory/archive path.",
	).
	WhenPred(TransportIs(canimcp.TransportOpenAI),
		"Use source.mode=url (server-fetchable HTTPS URL) or source.mode=data (RFC 2397 data: URI) — the server fetches/decodes and uploads them.",
	).
	When(FeatFileHostInput,
		"Website ZIPs: if you already have a site ZIP on the host (index.html + CSS/JS/images), call upload_file with file=<host file> and archive_mode=convert — the entire directory tree becomes one directory DAG whose CID you can publish directly to websites_create/update. Individual images/assets are not uploaded separately, and no presigned curl URL is minted for a file the host already holds. Before uploading a site ZIP, verify that index.html is at the archive root (not inside a wrapper directory) — websites_create/update will reject a CID whose root lacks index.html.",
	).
	When(FeatSourcePath,
		"Website ZIPs: if you already have a site ZIP on the host (index.html + CSS/JS/images), call upload_file with source.mode=path and archive_mode=convert — the entire directory tree becomes one directory DAG whose CID you can publish directly to websites_create/update. Individual images/assets are not uploaded separately. Before uploading a site ZIP, verify that index.html is at the archive root (not inside a wrapper directory) — websites_create/update will reject a CID whose root lacks index.html.",
	).
	When(FeatSourceMint,
		"Website ZIPs: call upload_file with source.mode=mint and archive_mode=convert — the entire directory tree becomes one directory DAG whose CID you can publish directly to websites_create/update.",
	).
	WhenPred(TransportIs(canimcp.TransportOpenAI),
		"If the upload fails with 'context canceled', retry with the same parameters — this is a transient host-side cancellation, not a file rejection. Poll upload_status with the returned handle.",
	)

// uploadFileTargets are the per-profile description targets for upload_file.
// A single fallback target with a resolver resolves the description
// dynamically against the platform profile, eliminating pre-built
// complete-string variants.
var uploadFileTargets = []mcpforge.Target[HostProfile]{{
	Visible:  true,
	DescFunc: uploadFileDesc.Resolve,
}}

// downloadFileDesc composes the download_file description: sink=local is
// available on every transport, while the drop filedrop sink is only
// advertised when the resolved profile has a reachable HTTP mux
// (FeatSinkDrop). The two clauses mirror the source's if/else so the startup
// and per-request surfaces cannot diverge.
var downloadFileDesc = mcpforge.Static[HostProfile](
	"Download IPFS content (CID or CID/path) as a file. Set sink=local to write the bytes to a host-side output_path on the MCP server's own disk (available on every transport)",
).
	WhenSep(mcpforge.SepSpace, FeatSinkDrop,
		"or sink=drop to get a one-time HTTP GET filedrop link to pull from out of band (curl -o <url> or a browser link).",
	).
	UnlessSep(mcpforge.SepSentence, FeatSinkDrop,
		"The filedrop GET sink is unavailable on this transport.",
	)

// downloadFileTargets are the per-profile description targets for
// download_file.
var downloadFileTargets = []mcpforge.Target[HostProfile]{{
	Visible:  true,
	DescFunc: downloadFileDesc.Resolve,
}}

// dataURIUploadDesc composes the upload_data tool description per profile.
// upload_data is only usable on a host that exposes the data: URI relay
// (FeatSourceData — the OpenAI tunnel). On a host without it (Grok, generic
// HTTP) the copy unconditionally forbids the tool so a model never
// base64-encodes a sandbox file when mint + PUT is the byte path. The old
// ChatGPT-oriented stop-rule ("do not call when host_file_input == true") is
// gone: it was a negation that flipped meaning after the honest
// host_file_input report, so the gate is now the presence of the data relay
// itself.
var dataURIUploadDesc = mcpforge.Static[HostProfile](
	"Upload bytes from an RFC 2397 data: URI and pin the resulting CID. The returned CID is already pinned, so pins_add is not needed afterward; the wait flag waits for this upload's own pin operation.",
).
	When(FeatSourceData,
		"Last resort — not for a host-provided or assistant-generated file.",
	).
	WhenAll([]mcpforge.Feature{FeatSourceMint, FeatSourceURL, FeatSourceData},
		"On this host prefer upload_file (mint + PUT) for an agent-local file and upload_url for a public HTTPS URL.",
	).
	Unless(FeatSourceData,
		"This transport has no data: URI relay. Upload bytes with upload_file(source.mode=mint) by PUTting the agent-local file to the returned url, then poll upload_status; a file is not base64-encoded as a data URI.",
	)

// resolveDescription resolves a vendored target list against ctx. A nil
// result marks a broken composition (a target list must always end with a
// fallback target); callers treat that invariant breach as the programming
// error it is rather than serving an empty description.
func resolveDescription(targets []mcpforge.Target[HostProfile], ctx HostProfile) (string, bool) {
	return mcpforge.ResolveDescription(targets, ctx)
}

// uploadFileResolutionProfile composes the HostProfile the upload_file
// description resolves against: the transport's generic mechanism profile with
// the registration-time effective feature set overlaid.
// profileForTransport — which for the embedded OpenAI tunnel carries
// FeatFileHostInput — is the base, so the host-file handoff guidance
// the input schema and ChatGPT metadata advertise also appears in the prose.
// Overriding with the effective features keeps the advertised description,
// schema, and Meta derived from ONE source of truth: whatever feature set
// shaped the schema also shapes the description.
func uploadFileResolutionProfile(features mcpforge.FeatureSet, t canimcp.TransportKind) HostProfile {
	profile := profileForTransport(t).CloneFeatures()
	for f, on := range features {
		profile.Features[f] = on
	}
	return profile
}

// uploadFileDescription resolves the tool description from the vendored
// feature-keyed targets against the transport's mechanism profile overlaid
// with the registration-time effective feature set (see
// uploadFileResolutionProfile). The forge picks the most specific matching
// target.
func uploadFileDescription(features mcpforge.FeatureSet, t canimcp.TransportKind) string {
	profile := uploadFileResolutionProfile(features, t)
	desc, ok := resolveDescription(uploadFileTargets, profile)
	if !ok {
		panic(fmt.Sprintf("mcp: upload_file has no matching description target for transport %q", t))
	}
	return desc
}

// downloadProfile maps the transport wiring to the feature set the description
// DSL resolves against. sink=local is always available; sink=drop needs a
// reachable HTTP mux, so it is advertised only when a filedrop coordinator is
// wired AND the transport is not the embedded OpenAI tunnel.
func downloadProfile(dropWired, tunnelOpenAI bool) HostProfile {
	p := profileForTransport(canimcp.TransportHTTP)
	p.Features[FeatSinkLocal] = true
	p.Features[FeatSinkDrop] = dropWired && !tunnelOpenAI
	return p
}

// downloadFileDescription resolves the download_file description from the
// vendored feature-keyed targets against the transport-built profile. It bakes
// the startup tools/list value.
func downloadFileDescription(dropWired, tunnelOpenAI bool) string {
	profile := downloadProfile(dropWired, tunnelOpenAI)
	desc, ok := resolveDescription(downloadFileTargets, profile)
	if !ok {
		panic(fmt.Sprintf("mcp: download_file has no matching description target (dropWired=%v tunnelOpenAI=%v)", dropWired, tunnelOpenAI))
	}
	return desc
}

// ---
// Schema transforms (feature-gated narrowing of the published schema).
// ---

// archiveModeSchemaDesc and wrapSchemaDesc are the shared property copy for the
// upload_file archive_mode and wrap inputs. They are static (the same wording
// is correct on every profile); only their presence and the source-mode enum
// vary by feature.
const (
	archiveModeSchemaDesc = "How to treat an archive. convert extracts an archive and uploads its contents as a directory DAG while preserving relative paths; use for complete static website ZIPs (index.html, CSS, JS, images, nested directories) — the resulting CID is a directory CID ready for websites_create/update. The archive's directory structure is preserved exactly, so index.html sits at the archive root (not inside a wrapper directory). preserve keeps the archive intact as a single file. IMPORTANT: the default depends on the source. Host-file, path, and url/data sources default to convert; the mint (presigned PUT) source defaults to preserve and ONLY converts when archive_mode=convert is passed explicitly. A website ZIP streamed via source.mode=mint therefore needs archive_mode=convert, or it uploads as a raw single-file CID and websites_create will reject it."
	wrapSchemaDesc        = "Wrap a single file in a directory root so the CID is a directory (required when the upload is a website). When wrap=true and no name is given, HTML content is auto-named index.html so the site resolves at its root. An explicit name such as 'starter-site' is honored as-is and the page is then only reachable at /starter-site, not /. Only affects single-file uploads; directories and archive-converted uploads are already a directory root."
)

// sourceFallbackDesc is the mode.copy for hosts that accept a host-provided
// `file` object (OpenAI/ChatGPT) where the source is only a fallback.
const sourceFallbackDesc = "Fallback transport. Only use when the host does not already hold the file as a host file accepted by the file parameter. The enum advertises which modes are valid on this transport."

// uploadSourceModeDesc is upload_file's mint-only mode copy: tool-scoped (only
// what THIS tool's source.mode accepts), never a claim the host has no other
// upload tool. The separate upload_url / upload_data relay tools exist on hosts
// that register them (FeatSourceURL/FeatSourceData) and are named as siblings.
var uploadSourceModeDesc = mcpforge.Static[HostProfile]("Only source.mode this tool accepts on this transport.").
	When(FeatSourceURL, "For a public HTTPS URL use the separate upload_url tool.").
	When(FeatSourceData, "For inline data: bytes with no file and no URL use the separate upload_data tool.")

// UploadSourceSchemaTransform narrows a reflected UploadSource schema's `mode`
// enum to the profile's supported source modes and rewrites its prose so a host
// that cannot pass a `file` object (no FeatFileHostInput) is led to the
// transport source as the only byte path rather than as a fallback.
func UploadSourceSchemaTransform(s *jsonschema.Schema, fs mcpforge.FeatureSet) {
	sourceSchemaTransform(s, fs)
}

// sourceSchemaTransform implements the UploadSource mode enum narrow + prose
// for upload_file. The mode prose is composed with the mcpforge DescBuilder
// (the same DSL used for tool descriptions), not by string concatenation, so
// the feature gating stays declarative.
func sourceSchemaTransform(s *jsonschema.Schema, fs mcpforge.FeatureSet) {
	mode, ok := s.Properties.Get("mode")
	if !ok {
		return
	}
	// The source.mode enum is pinned to the transport, never to capability
	// features. A host may declare FeatSourceData/FeatSourceURL to register
	// the separate upload_data/upload_url tools, but upload_file's own handler
	// is transport-bound (mint on HTTP), so the enum must not advertise a mode
	// the handler would reject. Deriving the transport via
	// TransportKindFromFeatures and taking the enum from THAT transport's
	// mechanism profile (not the raw feature set) keeps the enum honest even
	// when a relay capability feature co-occurs with the transport's mechanism
	// feature — the exact Grok pitfall the source pinned.
	mfs := modelFeatureSet(fs)
	transport := transfer.TransportKindFromFeatures(mfs)
	mode.Enum = transfer.SourceModeEnumFromFeatures(modelFeatureSet(transportFeaturesFor(canimcp.TransportKind(transport))))
	// Resolve the mode copy against a profile carrying the relevant features.
	prof := HostProfile{Features: fs}
	if fs.Has(FeatFileHostInput) {
		mode.Description = sourceFallbackDesc
	} else {
		mode.Description = uploadSourceModeDesc.Resolve(prof)
	}
	// Drop sibling payload fields whose source mode upload_file's handler cannot
	// accept on the resolved transport. The reflected UploadSource object
	// publishes path/url/data on every profile; on a mint-only transport
	// (HTTP) those dead OpenAI/ChatGPT fields are bindable training data even
	// though the mode enum has narrowed. This is transport-derived, not
	// feature-derived: a host like Grok declares FeatSourceURL/FeatSourceData
	// to register the separate upload_data/upload_url tools, but those do NOT
	// give upload_file a url/data branch — its HTTP handler rejects them. So
	// the sibling fields follow the enum (transport), keeping the schema a
	// model cannot hand a mode it would have to bind with no valid handler.
	t := transfer.TransportKindFromFeatures(mfs)
	for _, p := range [...]struct {
		field string
		ts    canimcp.TransportKind // the transport whose handler accepts this field
	}{
		{"path", canimcp.TransportStdio},
		{"url", canimcp.TransportOpenAI},
		{"data", canimcp.TransportOpenAI},
	} {
		if t != transfer.TransportKind(p.ts) {
			s.Properties.Delete(p.field)
		}
	}
}

// uploadFileSchema compiles the upload_file input schema from the tool's
// feature set. `source` is always present with its mode enum narrowed to the
// profile's transport modes and its prose adapted to the file-handoff feature;
// `file` (the OpenAI host-file reference) is present only when
// FeatFileHostInput. Because presence, enums, and prose are feature-driven, the
// schema never advertises a handoff the connected host cannot produce and never
// omits a source mode the transport supports.
func uploadFileSchema(features mcpforge.FeatureSet) json.RawMessage {
	return mcpforge.Schema().
		Property("file", toolargs.SchemaFor[transfer.ChatGPTFileInput](), mcpforge.When(FeatFileHostInput)).
		Property("source", toolargs.SchemaFor[transfer.UploadSource](), mcpforge.Description("The file to upload as a transport-scoped source object. Choose the mode this transport accepts (see capabilities.source_modes): path=co-located stdio, mint=HTTP/tunnel presigned endpoint, url/data=relay. Omit when a host-provided file reference is used instead."), mcpforge.Transform(UploadSourceSchemaTransform)).
		StringProperty("name", "Optional upload name (defaults to the file name).").
		BoolProperty("wait", "Wait until this upload's own pin operation completes before returning (the upload already pins; this only controls whether the call blocks for it).").
		StringProperty("archive_mode", archiveModeSchemaDesc, mcpforge.Enum("convert", "preserve"), mcpforge.Transform(archiveModeSchemaTransform)).
		StringProperty("ttl", "Presigned endpoint lifetime (e.g. 5m; default 5 minutes). Only used on transports that use presigned PUT endpoints.").
		BoolProperty("wrap", wrapSchemaDesc).
		RawJSON(features)
}

// mintOnlyArchiveModeDesc is the archive_mode copy for a host whose ONLY byte
// source is mint (e.g. Grok). It is composed from discrete self-punctuated
// sentences so the preserve-for-mint default and the convert-for-website rule
// stay critical and independently maintainable.
var mintOnlyArchiveModeDesc = mcpforge.Static[HostProfile](
	"convert extracts an archive and uploads its contents as a directory DAG (index.html sits at the archive root).",
).
	StaticSentence("preserve (the default for the mint source) keeps the archive intact as a single file.").
	StaticSentence("For source.mode=mint, pass archive_mode=convert for a website ZIP or it uploads as a raw single-file CID that websites_create will reject.")

// archiveModeSchemaTransform rewrites archive_mode.description to name only the
// byte sources THIS tool actually accepts on the resolved profile. The static
// base copy enumerates every transport source, so leaving it in place would make
// property-level schema text advertise path/url/data beside a source.mode enum
// that rejects them (e.g. a mint-only HTTP host with file input). The resolved
// copy is composed from gated sentence fragments (archiveModeDesc), so each
// accepted route contributes only its own default.
func archiveModeSchemaTransform(s *jsonschema.Schema, fs mcpforge.FeatureSet) {
	// A host whose only byte source for upload_file is mint (e.g. Grok) keeps
	// the focused mint preserve/convert copy; any host with another accepted
	// source gets the route-gated general copy.
	if mintOnlySource(fs) {
		s.Description = mintOnlyArchiveModeDesc.Resolve(HostProfile{Features: fs})
		return
	}
	// archiveModeDesc gates its source sentences on the mechanism transport
	// (transport predicates), so the profile must carry the resolved
	// transport — a Features-only profile would leave Transport zero and drop
	// every clause.
	profile := HostProfile{Features: fs, Transport: canimcp.TransportKind(transfer.TransportKindFromFeatures(modelFeatureSet(fs)))}
	s.Description = archiveModeDesc.Resolve(profile)
}

// mintOnlySource reports whether a profile routes upload_file through the mint
// source alone (HTTP transport, no host-file input). It keys on the transport's
// mechanism, not on capability features: a host may declare FeatSourceURL or
// FeatSourceData to register separate upload_url/upload_data relay tools, but
// those do not change upload_file's own mint-only source.
func mintOnlySource(fs mcpforge.FeatureSet) bool {
	return transfer.TransportKindFromFeatures(modelFeatureSet(fs)) == transfer.TransportHTTP && !fs.Has(FeatFileHostInput)
}

// archiveModeDesc composes the route-specific archive_mode copy. Each byte
// source this profile accepts contributes one sentence naming its own default,
// so an HTTP host with file input advertises only host-file + mint behavior, a
// stdio host only path, and a tunnel host only url/data — never a route the
// source.mode enum rejects. Sources are keyed on the transport's mechanism
// (see mintOnlySource's comment for why relay capability features are ignored).
// See mintOnlyArchiveModeDesc for the pure mint-only variant.
var archiveModeDesc = mcpforge.Static[HostProfile](
	"How to treat an archive. convert extracts an archive and uploads its contents as a directory DAG while preserving relative paths; use for complete static website ZIPs (index.html, CSS, JS, images, nested directories) — the resulting CID is a directory CID ready for websites_create/update. The archive's directory structure is preserved exactly, so index.html sits at the archive root (not inside a wrapper directory). preserve keeps the archive intact as a single file.",
).
	StaticSentence("The default depends on which sources this host accepts.").
	WhenSentence(FeatFileHostInput,
		"A host-file source defaults to convert.",
	).
	WhenPredSep(mcpforge.SepSentence, TransportIs(canimcp.TransportStdio),
		"A path source defaults to convert.",
	).
	WhenPredSep(mcpforge.SepSentence, TransportIs(canimcp.TransportHTTP),
		"The mint (presigned PUT) source defaults to preserve, converting only when archive_mode=convert is passed explicitly.",
	).
	WhenPredSep(mcpforge.SepSentence, TransportIs(canimcp.TransportOpenAI),
		"A url/data source defaults to convert.",
	).
	WhenPredSep(mcpforge.SepSentence, TransportIs(canimcp.TransportHTTP),
		"A website ZIP streamed via source.mode=mint therefore needs archive_mode=convert, or it uploads as a raw single-file CID that websites_create will reject.",
	)

// UploadFileTransport picks the TransportKind from the wiring flags. It classifies
// by reachability, not by whether a particular coordinator is wired: co-located
// stdio, the shared HTTP mux (plain HTTP or any non-OpenAI tunnel, with or without
// a presigned curl coordinator), or the embedded OpenAI tunnel, which exposes no
// reachable HTTP mux.
func UploadFileTransport(coLocated, tunnelOpenAI bool) canimcp.TransportKind {
	if coLocated {
		return canimcp.TransportStdio
	}
	if tunnelOpenAI {
		return canimcp.TransportOpenAI
	}
	return canimcp.TransportHTTP
}

// FileBaseName returns the base name of a path for a default upload label.
func FileBaseName(p string) string {
	if p == "" {
		return transfer.DefaultUploadName
	}
	// Trim trailing separators first so a trailing-slash path (e.g. "/tmp/"
	// or "C:\\d\\") resolves to its last segment ({"tmp", "d"}) instead of a
	// malformed name that still carries the separator.
	end := len(p)
	for end > 0 && (p[end-1] == '/' || p[end-1] == '\\') {
		end--
	}
	if end == 0 {
		// The path is only separators (e.g. "/"). Nothing to name.
		return transfer.DefaultUploadName
	}
	for i := end - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1 : end]
		}
	}
	// No separator: the path is already a bare relative file name.
	return p[:end]
}

// wrapUploadError enriches context-cancellation errors with a retry hint so
// the model treats them as transient host-side interruptions rather than
// structural file rejections that warrant switching to a fallback transport.
func wrapUploadError(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("upload interrupted (context canceled) — retry upload_file with the same file parameter; this is a transient host-side cancellation, not a file rejection: %w", err)
	}
	return err
}

// NewUploadFileDescriptor builds the unified, transport-aware upload_file tool.
// It accepts exactly one byte source per invocation: an OpenAI/host-provided
// `file` reference (fetched/streamed through relayFn on any transport, no
// human file-picker needed) OR a transport-scoped `source`. When `source` is
// used, the handler routes its mode to the real mechanism:
//
//   - stdio (coLocated): source mode path → pathFn reads the host path.
//   - HTTP/tunnel: source mode mint → hp mints a presigned PUT.
//   - OpenAI tunnel (tunnelOpenAI): source mode url/data → relayed through MCP.
//
// Executor and coordinator parameters are injected (LocalPathUploadHandler,
// *transfer.Upload, transfer.UploadHandler): this package owns only the
// presentation. features is the registration-time effective feature set — the
// same set the input schema is compiled against — so the published schema and
// the advertised description can never disagree.
func NewUploadFileDescriptor(features mcpforge.FeatureSet, coLocated, tunnelOpenAI bool, pathFn transfer.LocalPathUploadHandler, hp *transfer.Upload, relayFn transfer.UploadHandler, relayHosts []string, maxRelayBytes int64) model.ToolDescriptor {
	transport := UploadFileTransport(coLocated, tunnelOpenAI)
	hostFile := features.Has(FeatFileHostInput)
	var meta map[string]any
	if hostFile {
		// Advertise the OpenAI file-parameter handoff so a ChatGPT/OpenAI host
		// knows the top-level `file` argument carries a generated-file
		// reference (temporary download_url + file_id) it can populate from a
		// file it owns, without a human file-picker. This metadata is additive:
		// _meta.ui (MCP Apps), securitySchemes, and any other Pinner metadata
		// remain intact alongside it. Hosts without FeatFileHostInput (e.g.
		// Grok) must not advertise it.
		meta = transfer.ChatGPTFileMeta()
	}
	return model.ToolDescriptor{
		Name:  "upload_file",
		Title: "Upload a file to Pinner",
		// The description resolves against the transport's mechanism profile
		// overlaid with the SAME effective feature set the schema below is
		// compiled from — so when the profile carries FeatFileHostInput (the
		// OpenAI tunnel, or any host with the file handoff), the description
		// includes the host-file instructions the `file` schema property and
		// ChatGPT metadata advertise, never a schema/prose disagreement.
		Description:   uploadFileDescription(features, transport),
		Category:      model.CategoryCore,
		OpenWorldHint: true, // submits content to the Pinner/IPFS network
		// The input schema is compiled from the profile's feature set: the
		// `file` handoff is present only when FeatFileHostInput, source.mode is
		// narrowed to the transport's modes, and the mode prose is rewritten
		// accordingly. This keeps the published schema and the advertised
		// capabilities derived from one source of truth.
		InputSchema: uploadFileSchema(features),
		Meta:        meta,
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			in, err := toolargs.DecodeToolArgs[UploadFileInput](request)
			if err != nil {
				return model.ToolResult{}, err
			}

			// Deterministic source selection: exactly one byte source must be
			// provided. Do not let a silent precedence rule decide between
			// `file` and `source` for the caller.
			hasSource := in.Source != nil
			hasFile := in.File != nil
			switch {
			case !hasSource && !hasFile:
				return model.ToolResult{}, errors.New("an upload source is required")
			case hasSource && hasFile:
				return model.ToolResult{}, errors.New("provide exactly one upload source")
			}

			// OpenAI/host-provided generated-file handoff. Works on every
			// transport: the host passes a temporary download_url + file_id,
			// and Pinner fetches/streams the bytes through the same
			// authenticated UploadHandler executor the relay url/data sources
			// use — there is no separate pinning or transport path.
			if hasFile {
				if relayFn == nil {
					return model.ToolResult{}, errors.New("file upload executor is not configured")
				}
				ref, body, size, oerr := transfer.OpenChatGPTFileInput(ctx, *in.File, transfer.ChatGPTOpenTimeout, maxRelayBytes, relayHosts, nil)
				if oerr != nil {
					return model.ToolResult{}, oerr
				}
				defer body.Close()
				// Name precedence: explicit name > file.file_name > default.
				name := in.Name
				if name == "" {
					name = ref.FileName
				}
				if name == "" {
					name = transfer.DefaultUploadName
				}
				transferCtx, cancel := context.WithTimeout(ctx, transfer.SyncUploadBudget(size))
				defer cancel()
				// Thread archiveMode so a host-provided `file` (and url/data
				// relay) with archive_mode=convert extracts the archive into a
				// directory DAG — the same directory shape path-mode convert
				// produces — instead of uploading the raw archive as a single
				// file that breaks websites_create/update root resolution. The
				// executor only honors it when it can buffer to a seekable
				// temp file.
				result, err := relayFn(transferCtx, body, size, name, in.Wait, in.ArchiveMode, in.Wrap)
				return toolargs.WrapResult(result, wrapUploadError(err), "Uploaded.")
			}

			src := *in.Source
			if err := src.Validate(transfer.TransportKind(transport)); err != nil {
				return model.ToolResult{}, err
			}

			switch transport {
			case canimcp.TransportStdio:
				if src.Mode != transfer.SourcePath {
					return model.ToolResult{}, fmt.Errorf("source mode %q is not available on the %s transport", src.Mode, transport)
				}
				if pathFn == nil {
					return model.ToolResult{}, errors.New("local path upload is not configured")
				}
				name := in.Name
				if name == "" {
					name = FileBaseName(src.Path)
				}
				result, err := pathFn(ctx, src.Path, name, in.Wait, in.ArchiveMode, in.Wrap)
				return toolargs.WrapResult(result, wrapUploadError(err), "Uploaded.")
			case canimcp.TransportHTTP:
				if src.Mode != transfer.SourceMint {
					return model.ToolResult{}, fmt.Errorf("source mode %q is not available on the %s transport", src.Mode, transport)
				}
				if hp == nil {
					return model.ToolResult{}, errors.New("presigned upload endpoint is not configured for remote mode")
				}
				name := in.Name
				if name == "" {
					name = transfer.DefaultUploadName
				}
				// ONE canonical presign-TTL parser shared with the open_upload_manager
				// launcher and the upload App helpers — empty → default, non-positive
				// → default, unparseable → the stable `invalid ttl "..."` error.
				ttl, ttlErr := pinnertransfer.ParsePresignTTL(in.TTL)
				if ttlErr != nil {
					return model.ToolResult{}, ttlErr
				}
				// Prepare mints the presigned URL AND pre-creates a single
				// canonical upload handle in the shared UploadTaskManager. The
				// handle is returned up front so either the agent (curl the
				// URL) or the upload App file picker (which continues the same
				// handle) can fulfill the SAME operation — there is exactly
				// one upload task per operation, resolved by this one handle.
				//
				// The mint source records archive_mode/wrap on the handle at
				// mint time. The presigned PUT carries only raw bytes, so these
				// are captured here and applied by the executor when the bytes
				// arrive at Fulfill — the same directory-DAG conversion,
				// single-file wrap, or preserve that host-file/path/url/data
				// sources express directly. Unlike those in-band sources (whose
				// archive_mode default is convert), mint DEFAULTS to preserve:
				// it is an out-of-band raw-byte stream with no in-band archive
				// contract, so an undecorated mint PUT keeps its legacy
				// single-file CID. Only an EXPLICIT archive_mode=convert
				// extracts a streamed archive into a directory DAG, so a raw
				// .zip is never silently converted without being asked.
				m := in.ArchiveMode
				if m == "" {
					m = string(pinnertransfer.ArchivePreserve)
				}
				opts := []transfer.PrepareOption{transfer.WithArchiveMode(m)}
				if in.Wrap {
					opts = append(opts, transfer.WithWrap(true))
				}
				url, handle := hp.Prepare(ctx, name, ttl, opts...)
				if url == "" || handle == "" {
					return model.ToolResult{}, errors.New("failed to prepare one-time upload endpoint")
				}
				curlCmd := fmt.Sprintf("curl -sS -T <your-file> %q", url)
				sc := map[string]any{
					"url":                url,
					"curl_command":       curlCmd,
					"upload_handle":      handle,
					"upload_handle_poll": "upload_status",
					"ttl":                ttl.String(),
					"max_bytes":          hp.MaxBytes(),
				}
				// Text carries the same JSON as StructuredContent so a
				// text-only MCP client (which renders no widget) still sees
				// the actual presigned URL, curl command, and the pre-created
				// handle — not just prose. The handle is now produced up front
				// (rather than only in the PUT's 202 body) and can be handed
				// to the App's ipfs_upload_submit to fulfill the same
				// operation, or polled with upload_status.
				return model.ToolResult{
					StructuredContent: sc,
					Text:              toolargs.ResultJSONText(sc) + " Stream your file bytes to the URL with the curl command, or pass upload_handle to the upload App's file picker; then poll upload_status with the handle.",
				}, nil
			default: // TransportOpenAI
				if src.Mode != transfer.SourceURL && src.Mode != transfer.SourceData {
					return model.ToolResult{}, fmt.Errorf("source mode %q is not available on the OpenAI tunnel transport", src.Mode)
				}
				if relayFn == nil {
					return model.ToolResult{}, errors.New("file relay upload is not configured")
				}
				res := &transfer.SourceResolver{Transport: transfer.TransportOpenAI, RelayAllowedHosts: relayHosts, RelayMaxBytes: ieo.EffectiveRelayMaxBytes(maxRelayBytes)}
				body, size, srcName, oerr := res.OpenBytes(ctx, src)
				if oerr != nil {
					return model.ToolResult{}, oerr
				}
				defer body.Close()
				name := in.Name
				if name == "" {
					name = srcName
				}
				if name == "" {
					name = transfer.DefaultUploadName
				}
				transferCtx, cancel := context.WithTimeout(ctx, transfer.SyncUploadBudget(size))
				defer cancel()
				// Thread archiveMode so a host-provided `file` (and url/data
				// relay) with archive_mode=convert extracts the archive into a
				// directory DAG — the same directory shape path-mode convert
				// produces — instead of uploading the raw archive as a single
				// file that breaks websites_create/update root resolution. The
				// executor only honors it when it can buffer to a seekable
				// temp file.
				result, err := relayFn(transferCtx, body, size, name, in.Wait, in.ArchiveMode, in.Wrap)
				return toolargs.WrapResult(result, wrapUploadError(err), "Uploaded.")
			}
		},
	}
}

// DataURIUploadDescriptor uploads a file passed as a SEP-2356 data: URI. This
// is the additive, optional draft-MCP file-input mode declared via the
// x-mcp-file schema annotation; hosts that don't speak the draft simply omit
// it. Bytes are decoded by Pinner from the data URI, never re-emitted into the
// model context. The base64 payload is streamed to the upload handler, not
// materialized in memory.
//
// The startup/tools-list description resolves against the OpenAI-tunnel
// generic profile: upload_data only works on a host that exposes the data: URI
// relay (FeatSourceData, the OpenAI tunnel).
func DataURIUploadDescriptor(handler transfer.UploadHandler, maxBytes int64) model.ToolDescriptor {
	maxBytes = ieo.EffectiveRelayMaxBytes(maxBytes)
	dataURIUploadDescription := dataURIUploadDesc.Resolve(profileForTransport(canimcp.TransportOpenAI))
	return model.ToolDescriptor{
		Name:          "upload_data",
		Title:         "Upload a file from a data URI",
		Description:   dataURIUploadDescription,
		Category:      model.CategoryCore,
		OpenWorldHint: true, // submits content to the Pinner/IPFS network
		InputSchema:   toolargs.ToolSchemaFor[DataURIUploadInput](),
		// x-mcp-file marks the "file" property as a file-valued input per the
		// draft spec; the descriptor's Meta map carries it without a typed
		// field.
		Meta: map[string]any{"x-mcp-file": map[string]any{"file": map[string]any{"transferModes": []string{"inline"}}}},
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			in, err := toolargs.DecodeArgsFor[DataURIUploadInput]("data URI upload", handler != nil, request)
			if err != nil {
				return model.ToolResult{}, err
			}
			if in.File == "" {
				return model.ToolResult{}, fmt.Errorf("file (data URI) is required")
			}
			reader, opt, err := ieo.ParseFileDataURI(in.File, maxBytes)
			if err != nil {
				return model.ToolResult{}, err
			}
			name := in.Name
			if name == "" {
				name = opt.Name
			}
			if name == "" {
				name = transfer.DefaultUploadName
			}
			// Bound the upload phase; see transfer.SyncUploadBudget.
			transferCtx, cancel := context.WithTimeout(ctx, transfer.SyncUploadBudget(opt.Size))
			defer cancel()
			// data: URI input exposes no archive_mode field, so the upload must
			// always stay single-file. Pass an explicit "preserve" so
			// pinnertransfer.ParseArchiveMode cannot default "" to convert and
			// silently extract a base64 ZIP into a directory DAG the caller
			// cannot opt out of.
			result, err := handler(transferCtx, reader, opt.Size, name, in.Wait, "preserve", in.Wrap)
			return toolargs.WrapResult(result, err, "Data URI uploaded.")
		},
	}
}

// NewDownloadFileDescriptor builds the unified, sink-aware download_file tool.
// It downloads an IPFS node (CID or CID/path) and routes the retrieved bytes to
// the requested sink:
//
//   - sink=local (every transport): ipfsFn streams the CID bytes to a host-side
//     path confined under downloadRoot on the MCP server's own disk. Valid
//     regardless of transport, because the server's disk is always local to the
//     server process.
//   - sink=drop (HTTP / real tunnel): hd mints a one-time HTTP GET filedrop the
//     consumer pulls with curl -o / a browser <a download> link.
//
// On the OpenAI tunnel, only sink=local is honored (no reachable HTTP mux for
// a drop). The handler validates the sink against
// pinnertransfer.DownloadSinksAllowed before any byte is read or written.
func NewDownloadFileDescriptor(ipfsFn pinnertransfer.IPFSDownloadHandler, hd *transfer.Download, downloadRoot string, maxDownloadBytes int64, tunnelOpenAI bool) model.ToolDescriptor {
	return model.ToolDescriptor{
		Name:          "download_file",
		Title:         "Download IPFS content to a file",
		Description:   downloadFileDescription(hd != nil, tunnelOpenAI),
		Category:      model.CategoryCore,
		OpenWorldHint: false, // closed workflow: pulls content into local storage/filedrop; publishes nothing to the public internet
		// The input schema advertises only the sink values valid for the running
		// transport (drop only when a reachable HTTP mux exists on a non-OpenAI
		// tunnel), matching capabilities().download_sink_modes so the published
		// schema never contradicts the advertised sinks.
		InputSchema: transfer.RewriteSinkEnum(toolargs.ToolSchemaFor[DownloadFileInput](), hd != nil, tunnelOpenAI),
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			in, err := toolargs.DecodeToolArgs[DownloadFileInput](request)
			if err != nil {
				return model.ToolResult{}, err
			}
			if in.IPFSPath == "" {
				return model.ToolResult{}, fmt.Errorf("ipfs_path is required")
			}
			// Validate the sink against what this transport actually offers
			// before reading anything.
			if err := pinnertransfer.DownloadSinksAllowed(in.Sink, hd != nil, tunnelOpenAI); err != nil {
				return model.ToolResult{}, err
			}
			name := in.Name
			if name == "" {
				name = pinnertransfer.SinkDefaultName(in.IPFSPath)
			}
			if name == "" {
				name = "download"
			}

			switch in.Sink {
			case transfer.SinkLocal:
				if ipfsFn == nil {
					return model.ToolResult{}, errors.New("IPFS download handler is not configured")
				}
				res, err := pinnertransfer.ExecuteLocalSink(ctx, in.IPFSPath, name, in.OutputPath, downloadRoot, maxDownloadBytes, func(ctx context.Context, w io.Writer) error {
					return ipfsFn(ctx, in.IPFSPath, w)
				})
				return toolargs.WrapResult(res, err, "Downloaded from IPFS.")
			case transfer.SinkDrop:
				res, err := pinnertransfer.ExecuteDropSink(ctx, in.IPFSPath, name, hd, in.TTL, maxDownloadBytes, func(ctx context.Context, w io.Writer) error {
					return ipfsFn(ctx, in.IPFSPath, w)
				})
				return toolargs.WrapResult(res, err, "Filedrop minted; pull the bytes from fetch_url.")
			default:
				return model.ToolResult{}, fmt.Errorf("unknown sink %q", in.Sink)
			}
		},
	}
}
