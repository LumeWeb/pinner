package pinnermcp

import (
	"context"
	"fmt"
	"time"

	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/ieo"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/toolargs"
	"go.lumeweb.com/mcpplane/transfer"
)

// Relay-description first half of pinner-cli's internal/mcp/upload/
// upload_url.go (f4351d88): the upload_url relay tool, the last transfer
// descriptor promoted from the original CLI. The coordinator/executor halves
// stay where the other upload descriptors leave them: ieo.OpenFileURL (the
// vendored, SSRF-hardened URL relay primitive in mcpplane/ieo) fetches the
// caller-supplied public HTTPS URL, and the injected transfer.UploadHandler —
// the same Relay executor the url/data sources of upload_file and the host
// `file` handoff stream through — does the upload + pin.

// RelayURLUploadInput is the typed argument shape for upload_url.
type RelayURLUploadInput struct {
	URL  string `json:"url" jsonschema:"format=uri,description=Public HTTPS URL to fetch and upload."`
	Name string `json:"name,omitempty" jsonschema:"description=Optional upload name (defaults to the URL's file name)."`
	Wait bool   `json:"wait,omitempty" jsonschema:"description=Wait until this upload's own pin operation completes before returning (the upload already pins; this only controls whether the call blocks for it)."`
	Wrap bool   `json:"wrap,omitempty" jsonschema:"description=Wrap the single fetched file in a directory root so the CID is a directory (required when the upload is a website)."`
}

// relayURLUploadDesc composes the upload_url tool description per profile.
// Ported verbatim from pinner-cli's relayURLUploadDesc with hostenv.FeatSourceURL
// swapped for this package's FeatSourceURL. upload_url is a server-fetch URL
// relay — only a host with FeatSourceURL can have bytes fetched for it. On a
// host without it the copy unconditionally forbids the tool so a model never
// routes the byte path through a URL fetch when mint + PUT is what works.
var relayURLUploadDesc = mcpforge.Static[HostProfile](
	"Fetch a public HTTPS URL and upload it to Pinner, pinning the resulting CID. The returned CID is already pinned, so pins_add is not needed afterward; the wait flag waits for this upload's own pin operation. Pinner's credentials are not placed in the URL; Pinner fetches with its own stored auth.",
).
	When(FeatSourceURL,
		"Use when the bytes are already on the public web, not for a file in the agent sandbox — that is upload_file(source.mode=mint) plus the host PUT. The server fetches the URL directly (no download-then-re-upload).",
	).
	Unless(FeatSourceURL,
		"This transport has no URL-fetch relay. Upload bytes with upload_file(source.mode=mint) by PUTting the agent-local file to the returned url, then poll upload_status.",
	)

// RelayURLUploadDescriptor uploads a file by having the MCP process fetch a
// caller-supplied public HTTPS URL through ieo.OpenFileURL (SSRF hardening,
// host allowlist, hard byte bound), then streams the bytes through the
// injected executor — the SAME transfer.UploadHandler the url/data sources of
// upload_file and the host `file` handoff use (TransferDeps.Relay), so there
// is exactly one authenticated relay byte path. This is the generic relay
// fallback for remote HTTP-mode clients that are not co-located with Pinner
// and cannot pass a host path.
//
// allowedHosts and maxBytes are the relay constraints threaded from
// TransferDeps.RelayAllowedHosts / MaxRelayBytes (0 = the ieo default relay
// cap). features is the registration-time effective feature set the
// registration gate decided on: because upload_url registers ONLY when that
// set declares FeatSourceURL (see TransferDeps.RelayURLRegistered), the
// description resolves to the usable-fetch copy, never the no-relay forbid.
func RelayURLUploadDescriptor(handler transfer.UploadHandler, allowedHosts []string, maxBytes int64, features mcpforge.FeatureSet) model.ToolDescriptor {
	maxBytes = ieo.EffectiveRelayMaxBytes(maxBytes)
	description, ok := resolveDescription(relayURLUploadTargets, HostProfile{Features: features})
	if !ok {
		panic("pinnermcp: upload_url has no matching description target for its feature set")
	}
	return model.ToolDescriptor{
		Name:          "upload_url",
		Title:         "Upload a file from a URL",
		Description:   description,
		Category:      model.CategoryCore,
		OpenWorldHint: true, // fetches a caller-supplied URL and submits to Pinner/IPFS
		InputSchema:   toolargs.ToolSchemaFor[RelayURLUploadInput](),
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			in, err := toolargs.DecodeArgsFor[RelayURLUploadInput]("relay URL upload", handler != nil, request)
			if err != nil {
				return model.ToolResult{}, err
			}
			if in.URL == "" {
				return model.ToolResult{}, fmt.Errorf("url is required")
			}
			body, size, err := ieo.OpenFileURL(ctx, in.URL, ieo.FileRelayOptions{
				AllowedHosts:   allowedHosts,
				MaxBytes:       maxBytes,
				RequestTimeout: 2 * time.Minute,
			})
			if err != nil {
				return model.ToolResult{}, err
			}
			defer body.Close()
			name := in.Name
			if name == "" {
				name = transfer.DefaultUploadName
			}
			// Bound the upload itself: the MCP request ctx may carry no
			// deadline, so a hung network operation must not run
			// indefinitely. Budget scales with size; see SyncUploadBudget.
			transferCtx, cancel := context.WithTimeout(ctx, transfer.SyncUploadBudget(size))
			defer cancel()
			// Relay URL input exposes no archive_mode field, so the upload must
			// always stay single-file. Pass an explicit "preserve" so
			// pinnertransfer.ParseArchiveMode cannot default "" to convert and
			// silently extract a fetched ZIP into a directory DAG the caller
			// cannot opt out of.
			result, err := handler(transferCtx, body, size, name, in.Wait, "preserve", in.Wrap)
			return toolargs.WrapResult(result, wrapUploadError(err), "URL uploaded.")
		},
	}
}

// relayURLUploadTargets are the per-profile description targets for
// upload_url, mirroring the upload_file/download_file target shape so
// resolveDescription's invariant (a target list must end with a resolvable
// fallback) is exercised at descriptor construction.
var relayURLUploadTargets = []mcpforge.Target[HostProfile]{{
	Visible:  true,
	DescFunc: relayURLUploadDesc.Resolve,
}}
