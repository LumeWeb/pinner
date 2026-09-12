package appswire

import (
	"context"
	"errors"
	"fmt"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
	"go.lumeweb.com/mcpplane/toolargs"
	"go.lumeweb.com/mcpplane/transfer"

	pinnertransfer "go.lumeweb.com/pinner/transfer"
)

// Package-scoped identity for the Upload to IPFS MCP App. The launcher,
// helper tools, and view all share the one table row (LauncherUploadManager);
// this file is the dependency-bound implementation of that row, so a shared
// upload-manager launcher and view can be registered by any composition root
// that wires a *transfer.Upload coordinator (the CLI locally, a hosted
// deployment over its own transfer wiring). The model-facing upload_file
// primitive stays headless; only the open_ upload_manager launcher advertises
// the ui:// view.

// uploadManagerInput is the typed argument shape for the model-facing Upload
// to IPFS launcher. handle is optional: when provided, the launcher opens the
// app pre-bound to that already-prepared upload operation so the user can
// pick a file to fulfill it; when empty (or stale/expired), the launcher
// prepares a fresh operation itself.
type uploadManagerInput struct {
	Handle string `json:"handle,omitempty" jsonschema:"description=Optional upload handle from a prior upload_file mint result."`
	TTL    string `json:"ttl,omitempty" jsonschema:"description=Optional presigned endpoint lifetime, e.g. 5m (default 5m). Only used when a fresh operation is prepared."`
}

// UploadMintPoll is the ONE canonical completion-wait clause for the
// upload_file mint (presigned HTTP PUT) flow: which tool to poll, with which
// handle, until which terminal status. The upload_file descriptor, the agent
// guide's mint step, and the Upload to IPFS launcher description all compose
// this fragment instead of hand-paraphrasing it, so the poll contract cannot
// drift between surfaces.
const UploadMintPoll = "poll upload_status with the returned upload_handle until it reports completed"

// UploadManagerLauncherDescription builds the launcher description for the
// shared Upload to IPFS row, composing the canonical launcher skeleton with
// the canonical UploadMintPoll fragment (which tool, with which handle, until
// which terminal status) plus the pinned-CID durability note.
func UploadManagerLauncherDescription() string {
	return OpenLauncherDescriptionBody("Upload to IPFS file picker", "pick a file",
		"Pass an optional 'handle' from a prior upload_file mint call to continue that exact operation; if the handle is stale/expired a fresh one is prepared. "+
			"Returns an upload_handle; "+UploadMintPoll+" (the completed CID is already pinned).",
		"upload_file for autonomous uploads without a rendered file picker")
}

// UploadManagerDescriptor builds the model-facing open_upload_manager launcher
// tool. It is the ONLY tool that carries _meta.ui.resourceUri for the Upload
// to IPFS view — upload_file itself is headless. Its handler mints (or
// continues) a one-time presigned PUT endpoint bound to a canonical upload
// handle; the returned handle is the shared UploadTaskManager handle, so pass
// it to upload_status for the final CID.
func UploadManagerDescriptor(hp *transfer.Upload) (model.ToolDescriptor, error) {
	v, ok := SpecForLauncher(LauncherUploadManager)
	if !ok {
		return model.ToolDescriptor{}, fmt.Errorf("appswire: %s not in view table", LauncherUploadManager)
	}
	appMeta, err := sdk.MarshalToolMeta(model.AppToolMeta{
		ResourceURI: v.URI,
		Visibility:  []model.ToolVisibility{model.ToolVisibilityModel, model.ToolVisibilityApp},
	})
	if err != nil {
		return model.ToolDescriptor{}, fmt.Errorf("appswire: upload manager launcher: %w", err)
	}
	desc := UploadManagerLauncherDescription()
	return model.ToolDescriptor{
		Name:        LauncherUploadManager,
		Title:       v.Title,
		Description: desc,
		MCPTargets:  []model.ToolTarget{fallbackTarget(desc)},
		InputSchema: toolargs.ToolSchemaFor[uploadManagerInput](),
		Meta:        appMeta,
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			in, err := toolargs.DecodeToolArgs[uploadManagerInput](request)
			if err != nil {
				return model.ToolResult{}, err
			}
			// continued reports we fulfilled the caller's EXISTING operation.
			// It is true only when the supplied handle resolved to a live
			// endpoint; a stale/expired/used handle falls back to a fresh mint
			// and is therefore a brand-new operation (continued=false).
			//
			// The presign TTL is parsed ONLY in the branches that call
			// Prepare: continuing a live handle never touches TTL (it is
			// documented as only applying to a fresh operation), so a caller
			// passing a valid handle with a malformed TTL still continues
			// instead of getting a spurious `invalid ttl` error.
			var handle, url string
			continued := false
			if in.Handle != "" {
				if resolved, ok := hp.FindUpload(in.Handle); ok {
					url, handle, continued = resolved, in.Handle, true
				} else {
					ttl, terr := pinnertransfer.ParsePresignTTL(in.TTL)
					if terr != nil {
						return model.ToolResult{}, terr
					}
					url, handle = hp.Prepare(ctx, transfer.DefaultUploadName, ttl)
				}
			} else {
				ttl, terr := pinnertransfer.ParsePresignTTL(in.TTL)
				if terr != nil {
					return model.ToolResult{}, terr
				}
				url, handle = hp.Prepare(ctx, transfer.DefaultUploadName, ttl)
			}
			if url == "" || handle == "" {
				return model.ToolResult{}, errors.New("failed to prepare one-time upload endpoint")
			}
			sc := map[string]any{
				"upload_handle":      handle,
				"upload_handle_poll": "upload_status",
				"presigned_url":      url,
				"continued":          continued,
			}
			return model.ToolResult{
				StructuredContent: sc,
				Text:              toolargs.ResultJSONText(sc) + " The Upload to IPFS UI is open; pick a file to upload. Poll upload_status with the handle for the CID.",
			}, nil
		},
	}, nil
}

// uploadSubmitInput is the typed argument shape for the app-only
// ipfs_upload_submit helper. It continues or prepares a single canonical
// upload operation and returns the one-time presigned PUT endpoint bound to
// it; the retrieved URL is returned to the app so Uppy can XHR the bytes.
type uploadSubmitInput struct {
	Handle string `json:"handle,omitempty" jsonschema:"description=Optional canonical upload handle to continue instead of minting a new one."`
	Name   string `json:"name,omitempty" jsonschema:"description=Optional upload name (defaults to the file name or 'upload')."`
	TTL    string `json:"ttl,omitempty" jsonschema:"description=Presigned endpoint lifetime (e.g. 5m; default 5 minutes)."`
}

// uploadHandleArg is the typed argument shape for the app-only
// ipfs_upload_status poll helper.
type uploadHandleArg struct {
	Handle string `json:"handle" jsonschema:"description=Opaque upload handle returned in the presigned upload's 202 response body."`
}

// UploadManagerHelpers returns the app-only helper tools (ipfs_upload_submit /
// ipfs_upload_status) the Upload to IPFS view calls: submit/continue prepares
// the one-time presigned PUT endpoint bound to a canonical upload handle, and
// status polls the shared UploadTaskManager for the resulting CID. Both are
// visible to the app only (never the model); no file bytes cross this tool or
// the LLM channel.
func UploadManagerHelpers(hp *transfer.Upload) []model.ToolDescriptor {
	v, _ := SpecForLauncher(LauncherUploadManager)
	appMeta, _ := sdk.MarshalToolMeta(model.AppToolMeta{
		ResourceURI: v.URI,
		Visibility:  []model.ToolVisibility{model.ToolVisibilityApp},
	})
	return []model.ToolDescriptor{
		{
			Name:        "ipfs_upload_submit",
			Title:       "Prepare a one-time upload endpoint",
			Description: "Prepare (or continue) a one-time presigned HTTP PUT endpoint bound to a canonical upload handle; the app's Uppy XHR uploader writes file bytes to it out of band. Passing a handle prepared by upload_file fulfills that same operation. App-only helper for the Upload to IPFS view.",
			InputSchema: toolargs.ToolSchemaFor[uploadSubmitInput](),
			Meta:        appMeta,
			Handler: func(ctx context.Context, req model.ToolRequest) (model.ToolResult, error) {
				in, err := toolargs.DecodeToolArgs[uploadSubmitInput](req)
				if err != nil {
					return model.ToolResult{}, err
				}
				// ONE canonical presign-TTL parser shared with upload_file and
				// the launcher.
				ttl, terr := pinnertransfer.ParsePresignTTL(in.TTL)
				if terr != nil {
					return model.ToolResult{}, terr
				}
				// Continue an operation the model-facing upload_file already
				// prepared: return the SAME endpoint + handle so the app
				// fulfills the canonical operation rather than minting a
				// sibling.
				if in.Handle != "" {
					if url, ok := hp.FindUpload(in.Handle); ok {
						sc := map[string]any{
							"url":           url,
							"upload_handle": in.Handle,
							"ttl":           ttl.String(),
							"max_bytes":     hp.MaxBytes(),
							"poll_tool":     "ipfs_upload_status",
							"continued":     true,
							"response_body": "the 202 body carries the SAME upload_handle; pass it to poll_tool",
						}
						return model.ToolResult{
							StructuredContent: sc,
							Text:              toolargs.ResultJSONText(sc) + " PUT the file bytes and poll for the CID.",
						}, nil
					}
					// Still tracked but its endpoint is gone: either never
					// fulfilled (window lapsed) or already claimed/completed.
					if task, terr := hp.Tasks().Get(in.Handle); terr == nil {
						if task.State == transfer.UploadStatePrepared {
							// Prepared but never fulfilled: not already-claimed
							// — nobody supplied bytes. The app should prepare a
							// fresh operation rather than poll a byte-less
							// handle or duplicate an in-flight upload.
							return model.ToolResult{}, fmt.Errorf(
								"upload %q was prepared but never fulfilled (endpoint expired); start a fresh upload",
								in.Handle)
						}
						// Claimed or finished: report the already-claimed state
						// so the app just polls and never re-uploads.
						sc := map[string]any{
							"upload_handle":   in.Handle,
							"already_claimed": true,
							"state":           task.State,
							"poll_tool":       "ipfs_upload_status",
						}
						return model.ToolResult{
							StructuredContent: sc,
							Text:              toolargs.ResultJSONText(sc) + " This operation is already fulfilled/claimed; poll ipfs_upload_status for the CID.",
						}, nil
					}
					return model.ToolResult{}, fmt.Errorf("unknown upload handle %q; start a fresh upload", in.Handle)
				}

				// No handle: prepare a fresh canonical operation and return its
				// handle up front so the same handle can be polled.
				name := in.Name
				if name == "" {
					name = transfer.DefaultUploadName
				}
				url, handle := hp.Prepare(ctx, name, ttl)
				if url == "" || handle == "" {
					return model.ToolResult{}, errors.New("failed to prepare one-time upload endpoint")
				}
				sc := map[string]any{
					"url":           url,
					"upload_handle": handle,
					"ttl":           ttl.String(),
					"max_bytes":     hp.MaxBytes(),
					"poll_tool":     "ipfs_upload_status",
					"response_body": "the 202 body carries the SAME upload_handle; pass it to poll_tool",
				}
				return model.ToolResult{
					StructuredContent: sc,
					// Text carries the same JSON so a text-only client sees the
					// actual presigned URL plus poll instructions.
					Text: toolargs.ResultJSONText(sc) + " PUT the file bytes and poll for the CID.",
				}, nil
			},
		},
		{
			Name:        "ipfs_upload_status",
			Title:       "Get upload status",
			Description: "Return the status of an async upload by handle: prepared, queued, running, completed (with CID), failed, cancelled, or expired. App-only helper for the Upload to IPFS view.",
			InputSchema: toolargs.ToolSchemaFor[uploadHandleArg](),
			Meta:        appMeta,
			Handler: func(ctx context.Context, req model.ToolRequest) (model.ToolResult, error) {
				in, err := toolargs.DecodeToolArgs[uploadHandleArg](req)
				if err != nil {
					return model.ToolResult{}, err
				}
				if in.Handle == "" {
					return model.ToolResult{}, errors.New("handle is required")
				}
				task, err := hp.Tasks().Get(in.Handle)
				if err != nil {
					return model.ToolResult{}, err
				}
				return model.ToolResult{StructuredContent: task, Text: toolargs.ResultJSONText(task)}, nil
			},
		},
	}
}

// UploadManagerInstaller returns an Installer that registers the Upload to
// IPFS view bound to hp. It supplies the view's CSP connectDomains from the
// coordinator's origins (so the sandbox iframe may PUT bytes to the presigned
// endpoint) and threads the submit/status helpers (from InstallContext.Helpers
// when the composition supplied them, else the shared helpers). This is the
// dependency-bound installer for the LauncherUploadManager row.
func UploadManagerInstaller(hp *transfer.Upload) Installer {
	return func(ic InstallContext) error {
		if hp == nil {
			return errors.New("appswire: upload manager installer requires a non-nil upload coordinator")
		}
		v, ok := SpecForLauncher(LauncherUploadManager)
		if !ok {
			return fmt.Errorf("appswire: %s not in view table", LauncherUploadManager)
		}
		helpers := ic.Helpers[LauncherUploadManager]
		if len(helpers) == 0 {
			helpers = UploadManagerHelpers(hp)
		}
		av := v.AppViewFor(ic.Render, helpers)
		av.ConnectDomainsFunc = hp.ConnectOrigins
		return ic.Registry.RegisterAppView(ic.Server, ic.Catalog, av)
	}
}
