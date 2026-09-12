package main

// harness.go builds the test-only MCP server from pinner's public seams:
//
//   - assembly.AssembleCatalogOps over the FakeServices bundle (in-process,
//     never touching the network)
//   - mcp.Assemble for the SDK-free presentation surface (compiled catalog
//     tools + direct tools + progressive-disclosure meta-tools)
//   - mcpplane/sdk for the official MCP server, tool registration and
//     transports
//   - mcpplane/transfer for the HTTP presigned-PUT upload/download
//     coordinators the app helper tools mint endpoints from
//   - mcp/appswire + canvasassets for the ui:// MCP Apps (real rendered
//     canvas documents)
//
// The composition requires sdk.SetToolRegistrar to be installed BEFORE app
// (launcher/helper) registration so RegisterAppTool funnels through the same
// HandlerDeps the plain tools use.

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	mcpapps "go.lumeweb.com/mcpplane/apps"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
	"go.lumeweb.com/mcpplane/toolargs"
	mptransfer "go.lumeweb.com/mcpplane/transfer"
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/pinner/assembly"
	"go.lumeweb.com/pinner/canvas"
	"go.lumeweb.com/pinner/canvasassets"
	"go.lumeweb.com/pinner/catalogops"
	"go.lumeweb.com/pinner/core/apikeys"
	"go.lumeweb.com/pinner/core/auth"
	"go.lumeweb.com/pinner/core/config"
	"go.lumeweb.com/pinner/core/dns"
	"go.lumeweb.com/pinner/core/ipns"
	"go.lumeweb.com/pinner/core/operations"
	"go.lumeweb.com/pinner/core/pinning"
	"go.lumeweb.com/pinner/core/websites"

	pinnermcp "go.lumeweb.com/pinner/mcp"
	appswire "go.lumeweb.com/pinner/mcp/appswire"
)

// maxUploadBytes is the presigned-PUT upload cap for the harness.
const maxUploadBytes = int64(1024) * 1024 * 1024

// HarnessOptions are the build-time knobs parsed from main.go flags.
type HarnessOptions struct {
	// AuthToken overrides the config-stored credential; also the HTTP-mode
	// static bearer (empty = "token-<email>" derived from SeedEmail).
	AuthToken string
	// SeedEmail seeds the faked account identity and the derived token.
	SeedEmail string
}

// Harness holds the fully-wired server artifacts.
type Harness struct {
	// Surface is the assembled SDK-free presentation (Catalog/Config/Tools…).
	Surface *pinnermcp.Server
	// Catalog is the assembled opmesh catalog (dispatch target).
	Catalog opmesh.Catalog
	// SDK is the official MCP server serving the harness tools.
	SDK *sdk.Server
	// Registry is the per-server MCP App registry.
	Registry *mcpapps.AppRegistry
	// InstalledApps are the launcher views that actually installed.
	InstalledApps []string
	// Upload / Download are the presigned transfer coordinators.
	Upload *mptransfer.Upload
	// Download is the presigned download coordinator.
	Download *mptransfer.Download
	// Fakes are the underlying in-memory core services.
	Fakes *FakeServices

	deps        sdk.HandlerDeps
	toolCatalog *harnessToolCatalog
	authHost    string // host:port base for HTTP mode SetBaseURL (set by http.go)
}

// Build assembles the harness server: fakes -> catalog -> app install ->
// mcp.Assemble -> SDK tool registration. The server is not yet standing on any
// transport; main.go chooses stdio or HTTP.
func Build(ctx context.Context, opts HarnessOptions) (*Harness, error) {
	fakes, err := NewFakeServices(opts.SeedEmail)
	if err != nil {
		return nil, err
	}
	if opts.AuthToken != "" {
		fakes.Token = opts.AuthToken
		if err := fakes.CfgMgr().SetAuthToken(opts.AuthToken); err != nil {
			return nil, fmt.Errorf("harness: set auth token on config: %w", err)
		}
	}

	cat, err := assembly.AssembleCatalogOps(buildCatalogDeps(fakes), assembly.FullDomainScope, false)
	if err != nil {
		return nil, fmt.Errorf("harness: assemble catalog: %w", err)
	}

	official := sdk.NewServer(&sdk.ServerOptions{
		Instructions: "pinner mcpharness: test-only MCP server over faked core services (no network). Discovery starts at search_tools; use describe_tool then the typed invoke_*_tool dispatchers.",
	})

	upload := newFakeUpload()
	download := mptransfer.NewHTTPDownload()

	registry := mcpapps.NewAppRegistry()
	tools := newHarnessToolCatalog()

	h := &Harness{
		Catalog:     cat,
		SDK:         official,
		Registry:    registry,
		Upload:      upload,
		Download:    download,
		Fakes:       fakes,
		toolCatalog: tools,
	}
	h.deps = sdk.HandlerDeps{
		RequestCaps:             buildRequestCaps,
		ReservedRequestStateKey: opmesh.ReservedRequestStateKey,
		LogStart:                func(string, map[string]any) {},
		LogEnd:                  func(string, time.Time, model.ToolResult, error) {},
		AnnotateApp:             h.annotateApp,
	}

	// The SDK tool registrar must be installed BEFORE any app-tool
	// registration: RegisterAppTool funnels through it so helper tools carry
	// the same HandlerDeps adornment (per-request caps, reserved-key echo,
	// app annotation) as plain tools.
	sdk.SetToolRegistrar(func(srv *sdk.Server, desc model.ToolDescriptor, handler model.ToolHandler) error {
		_ = handler // the descriptor carries its own handler; keep single source
		return sdk.RegisterTool(srv, h.deps, desc)
	})

	// Selectable app-view table rows for every capability the harness wires
	// (OOB sign-in panels + the Sia vault views). Every row gets an installer,
	// so the install inventory is exactly these launchers.
	caps := appswire.CapOOB | appswire.CapVault
	rows := appswire.Selectable(caps, nil)

	// Register the open_* launchers first: RegisterAppView asserts its
	// AttachTo tools exist in the catalog, and a launcher is the ONLY tool
	// advertising a view's resourceUri.
	for _, row := range rows {
		desc, err := row.NewLauncherDescriptorFor()
		if err != nil {
			return nil, fmt.Errorf("harness: launcher %s: %w", row.Launcher, err)
		}
		if err := h.registerTool(desc); err != nil {
			return nil, fmt.Errorf("harness: register launcher %s: %w", row.Launcher, err)
		}
	}

	installers := appswire.Installers{}
	for _, row := range rows {
		installers[row.Launcher] = row.ViewInstaller()
	}
	helpers := map[string][]model.ToolDescriptor{
		appswire.LauncherSSOSignin:     {authSSOStatusDescriptor(fakes)},
		appswire.LauncherUploadManager: {ipfsUploadSubmitDescriptor(h), ipfsUploadStatusDescriptor(h)},
		appswire.LauncherVaultManager:  {vaultUploadSubmitDescriptor(h)},
	}
	installed, err := appswire.Install(official, tools, rows, installers, appswire.InstallOptions{
		Registry: registry,
		Render:   renderFuncFor(rows),
		Helpers:  helpers,
	})
	if err != nil {
		return nil, fmt.Errorf("harness: install app views: %w", err)
	}
	h.InstalledApps = installed

	// Assemble the presentation surface (guide inventory now knows the
	// installed launchers).
	surface, err := pinnermcp.Assemble(pinnermcp.Config{
		DomainScope:         assembly.FullDomainScope,
		Catalog:             cat,
		DevTools:            true,
		InstalledApps:       installed,
		VerifyInstalledApps: verifyInstalledApps(tools, registry),
	})
	if err != nil {
		return nil, fmt.Errorf("harness: assemble mcp surface: %w", err)
	}
	h.Surface = surface

	// 1) Compiled catalog surface: register every model-visible tool (the
	// DirectVisible front-door set INCLUDED — surface.Direct carries only the
	// direct-only tools: agent_guide, capabilities, dev_* ) with a handler
	// that dispatches through the catalog's Invoke gate.
	for _, d := range surface.Tools {
		desc := d
		name := d.Name
		desc.Handler = model.ToolHandler(func(ctx context.Context, req model.ToolRequest) (model.ToolResult, error) {
			return h.dispatchCatalogOp(ctx, name, req.Arguments)
		})
		if err := h.registerTool(desc); err != nil {
			return nil, fmt.Errorf("harness: register %s: %w", name, err)
		}
	}
	// 2) Direct-only tools (agent_guide, capabilities, dev_*): they carry
	// baked-in handlers.
	for _, d := range surface.Direct {
		if err := h.registerTool(d); err != nil {
			return nil, fmt.Errorf("harness: register direct %s: %w", d.Name, err)
		}
	}
	// 3) Progressive-disclosure meta-tools: search/describe and the typed
	// invoke dispatchers that enforce the read/write/destructive classes and
	// refuse admin tools.
	metaDescs, err := surface.MetaToolDescriptors(h.catalogDispatch)
	if err != nil {
		return nil, fmt.Errorf("harness: build meta tools: %w", err)
	}
	for _, d := range metaDescs {
		if err := h.registerTool(d); err != nil {
			return nil, fmt.Errorf("harness: register meta %s: %w", d.Name, err)
		}
	}

	return h, nil
}

// registerTool registers one descriptor on the official server via
// HandlerDeps and records it in the harness tool catalog the app views attach
// through.
func (h *Harness) registerTool(desc model.ToolDescriptor) error {
	if err := sdk.RegisterTool(h.SDK, h.deps, desc); err != nil {
		return err
	}
	h.toolCatalog.record(desc)
	return nil
}

// catalogDispatch is the mcp.CatalogDispatch seam: for a compiled op name it
// returns the handler that routes through Catalog.Invoke.
func (h *Harness) catalogDispatch(name string) model.ToolHandler {
	return func(ctx context.Context, req model.ToolRequest) (model.ToolResult, error) {
		return h.dispatchCatalogOp(ctx, name, req.Arguments)
	}
}

// dispatchCatalogOp executes one catalog op with the model actor and maps the
// gate refusals to typed results:
//
//   - ErrConfirmRequired (destructive, model actor) -> needs_human
//     confirmation hand-off
//   - ErrHumanRequired (InteractionHumanOnly / InteractionNeedsHandoff,
//     non-human actor) -> needs_human interactive_only hand-off
//   - other errors -> IsError text result
//   - success -> {status:"ok", value} structured envelope
func (h *Harness) dispatchCatalogOp(ctx context.Context, name string, args map[string]any) (model.ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	result, err := h.Catalog.Invoke(ctx, name, args, opmesh.ActorModel)
	if err != nil {
		if errors.Is(err, opmesh.ErrConfirmRequired) {
			return model.NeedsHumanResult(model.NeedsHuman{
				Reason: model.ReasonConfirmation,
				Detail: name + " is destructive and requires explicit human confirmation",
			}), nil
		}
		if errors.Is(err, opmesh.ErrHumanRequired) {
			return model.NeedsHumanResult(model.NeedsHuman{
				Reason:     model.ReasonInteractiveOnly,
				ResumeTool: name,
				Detail:     name + " requires interactive human input; an agent cannot run it directly",
			}), nil
		}
		return model.ToolResult{IsError: true, Text: err.Error()}, nil
	}
	return resultToToolResult(result), nil
}

// resultToToolResult wraps the typed catalog result in the canonical
// {status:"ok", value} envelope on both channels (mirrors the CLI dispatch).
func resultToToolResult(result any) model.ToolResult {
	b, err := json.Marshal(result)
	if err != nil {
		return model.ToolResult{IsError: true, Text: "unable to serialize catalog result"}
	}
	if string(b) == "null" {
		sc := map[string]any{"status": model.StatusOk}
		jb, _ := json.Marshal(sc)
		return model.ToolResult{Text: string(jb), StructuredContent: sc}
	}
	var value any
	if json.Valid(b) {
		value = json.RawMessage(b)
	}
	sc := map[string]any{"status": model.StatusOk, "value": value}
	jb, _ := json.Marshal(sc)
	return model.ToolResult{Text: string(jb), StructuredContent: sc}
}

// renderFuncFor adapts canvasassets.RenderAppDoc (which takes a title) to the
// appswire RenderFunc: each row's view renders with that row's ResourceTitle.
func renderFuncFor(rows []appswire.ViewSpec) appswire.RenderFunc {
	titles := map[canvas.View]string{}
	for _, row := range rows {
		titles[row.View] = row.ResourceTitle
	}
	return func(view canvas.View) string {
		return canvasassets.RenderAppDoc(view, titles[view])
	}
}

// buildRequestCaps builds the SDK-neutral per-request capability view. The
// pinned mcpplane has no request-caps builder on the SDK, so the harness
// populates the typed fields it can read directly from the go-sdk request:
// the negotiated protocol version and the client identity.
func buildRequestCaps(req *sdk.CallToolRequest) *model.RequestCaps {
	if req == nil {
		return nil
	}
	caps := &model.RequestCaps{ProtocolVersion: req.ProtocolVersion()}
	if req.Session != nil {
		if init := req.Session.InitializeParams(); init != nil {
			if init.ProtocolVersion != "" {
				caps.ProtocolVersion = init.ProtocolVersion
			}
			if init.ClientInfo != nil {
				caps.ClientName = init.ClientInfo.Name
				caps.ClientVersion = init.ClientInfo.Version
			}
		}
	}
	return caps
}

// annotateApp appends companion-app context to a needs_human result whose tool
// has a registered ui:// app view.
func (h *Harness) annotateApp(toolName string, _ *model.RequestCaps, result *model.ToolResult) {
	if result == nil || result.StructuredContent == nil {
		return
	}
	info, ok := h.Registry.AppInfoForTool(toolName)
	if !ok {
		return
	}
	if sc, ok := result.StructuredContent.(map[string]any); ok {
		if st, _ := sc["status"].(string); st == model.StatusNeedsHuman {
			sc["companion_app"] = map[string]any{
				"uri":   info.URI,
				"name":  info.Name,
				"title": info.Title,
			}
		}
	}
}

// harnessToolCatalog is the mcpplane apps.AppCatalog adapter: the launchers +
// helper tools the harness registered, so RegisterAppView can validate and
// attach _meta.ui.
type harnessToolCatalog struct {
	entries map[string]*model.ToolEntry
}

func newHarnessToolCatalog() *harnessToolCatalog {
	return &harnessToolCatalog{entries: map[string]*model.ToolEntry{}}
}

func (c *harnessToolCatalog) record(desc model.ToolDescriptor) {
	c.entries[desc.Name] = model.ToolEntryFromDescriptor(desc)
}

// Get implements mcpapps.AppCatalog.
func (c *harnessToolCatalog) Get(name string) (*model.ToolEntry, bool) {
	e, ok := c.entries[name]
	return e, ok
}

// verifyInstalledApps returns the mcp.Config.VerifyInstalledApps assertion: a
// guide-inventory launcher without a registry association fails loudly.
func verifyInstalledApps(tools *harnessToolCatalog, registry *mcpapps.AppRegistry) func([]string) error {
	return func(installed []string) error {
		for _, name := range installed {
			if _, ok := registry.AppInfoForTool(name); !ok {
				return fmt.Errorf("installed app %q has no registry association", name)
			}
			if _, ok := tools.Get(name); !ok {
				return fmt.Errorf("installed app %q is not a registered tool", name)
			}
		}
		return nil
	}
}

// buildCatalogDeps wires the fake services into the assembly.CatalogDepsBundle
// (lazy getters so every invocation constructs fresh fakes over shared state).
func buildCatalogDeps(f *FakeServices) *assembly.CatalogDepsBundle {
	authService := func(cfgMgr config.Manager, _ string) auth.AuthService { return newFakeAuthService(f) }

	ipnsDeps := ipnsDepsFor(f)

	return &assembly.CatalogDepsBundle{
		CfgMgr: f.CfgMgr,
		CredentialResolver: assembly.ConfigCredentialResolver{
			AuthToken: func() (string, error) { return f.resolveToken(f.cfgMgr), nil },
		},
		Auth: catalogops.AuthDeps{
			CfgMgr:           f.CfgMgr,
			AuthService:      authService,
			ResolveAuthToken: f.resolveToken,
		},
		Account: catalogops.AccountDeps{
			CfgMgr:      f.CfgMgr,
			AuthService: authService,
			PortalURL: func(cfgMgr config.Manager) string {
				return cfgMgr.Config().BaseEndpoint + "/account/subscription"
			},
		},
		Pins: catalogops.PinsDeps{
			CfgMgr: f.CfgMgr,
			Secure: f.secure,
			ServiceFactory: func(config.Manager, bool) pinning.PinningService {
				return newFakePinningService(f)
			},
			NewAuthenticated: func(config.Manager, bool, string) pinning.PinningService {
				return newFakePinningService(f)
			},
			GetAuthToken: func() string { return f.Token },
		},
		Websites: catalogops.WebsitesDeps{
			CfgMgr: f.CfgMgr,
			Secure: f.secure,
			ServiceFactory: func(config.Manager, bool, ...websites.Option) websites.Service {
				return newFakeWebsitesService(f)
			},
			NewAuthenticated: func(config.Manager, bool, string) (websites.Service, error) {
				return newFakeWebsitesService(f), nil
			},
			GetAuthToken: func() string { return f.Token },
		},
		DNS: catalogops.DNSDeps{
			CfgMgr: f.CfgMgr,
			Secure: f.secure,
			ServiceFactory: func(config.Manager, bool, ...dns.Option) dns.Service {
				return newFakeDNSService(f)
			},
			NewAuthenticated: func(config.Manager, bool, string) dns.Service {
				return newFakeDNSService(f)
			},
			GetAuthToken: func() string { return f.Token },
		},
		IPNS: ipnsDeps,
		ENS:  catalogops.ENSDeps{IPNS: ipnsDeps},
		APIKeys: catalogops.APIKeysDeps{
			Service: func(map[string]any) apikeys.Service {
				return &fakeAPIKeysService{svc: f}
			},
			OOBKeyDropBuild: func(map[string]any) catalogops.APIKeyDrop {
				return fakeAPIKeyDrop{}
			},
		},
		Operations: catalogops.OperationsDeps{
			Service: func(map[string]any) operations.Service {
				return &fakeOperationsService{svc: f}
			},
		},
		// Vault and Admin stay unwired: those ops fail at execution time with
		// clear "service unavailable" errors (nil-deps degradation), which is
		// the accepted surface for a harness that must never touch Sia or the
		// portal admin API.
	}
}

// ipnsDepsFor builds the IPNS deps (shared with the ENS domain). The
// NewAuthenticated signature differs from the other domains (token first, and
// it returns an error) so it is wired separately.
func ipnsDepsFor(f *FakeServices) catalogops.IPNSDeps {
	return catalogops.IPNSDeps{
		CfgMgr: f.CfgMgr,
		Secure: f.secure,
		ServiceFactory: func(config.Manager, bool, ...ipns.Option) ipns.Service {
			return newFakeIPNSService(f)
		},
		NewAuthenticated: func(config.Manager, string, bool) (ipns.Service, error) {
			return newFakeIPNSService(f), nil
		},
		GetAuthToken: func() string { return f.Token },
	}
}

// --- presigned transfer wiring ------------------------------------------------

// newFakeUpload builds the presigned-PUT upload coordinator. Its executor
// drains the PUT bytes and deterministically "completes" the upload with a
// fake CID derived from the name — an in-process stand-in for the IPFS pin.
func newFakeUpload() *mptransfer.Upload {
	exec := mptransfer.UploadHandler(
		func(_ context.Context, r io.Reader, _ int64, name string, _ bool, _ string, _ bool) (any, error) {
			_, _ = io.Copy(io.Discard, r)
			sum := sha256.Sum256([]byte(name))
			return map[string]any{
				"cid":  fmt.Sprintf("bafybeiharness%016x", sum[:8]),
				"name": name,
			}, nil
		},
	)
	tasks := mptransfer.NewUploadTaskManager(exec, mptransfer.DefaultHTTPUploadTTL)
	return mptransfer.NewHTTPUpload(tasks, maxUploadBytes)
}

// --- app-only helper tool descriptors ------------------------------------------

// helperSchema builds a minimal JSON object schema from properties (raw JSON
// schema strings, matching the descriptor format).
func helperSchema(properties string) json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":` + properties + `}`)
}

// authSSOStatusDescriptor is the app-only status helper for the Sign In view
// (ui://auth/sso.html). The harness fakes the OOB approval flow: any non-empty
// handle reports an immediately-complete sign-in; without a handle it reports
// a pending approval at the fake portal URL so the app has something to show.
func authSSOStatusDescriptor(f *FakeServices) model.ToolDescriptor {
	in := helperSchema(`{"handle":{"type":"string","description":"Handle from auth_sso (empty = start reporting an approval flow)."}}`)
	return model.ToolDescriptor{
		Name:        "auth_sso_status",
		Title:       "Auth Sign-In Status",
		Description: "Poll a pending out-of-band sign-in by handle. App-only helper for the Sign In view.",
		InputSchema: in,
		Handler: func(_ context.Context, req model.ToolRequest) (model.ToolResult, error) {
			args, err := toolargs.DecodeToolArgs[struct {
				Handle string `json:"handle"`
			}](req)
			if err != nil {
				return model.ToolResult{}, err
			}
			if args.Handle == "" {
				sc := map[string]any{
					"status":     "pending",
					"action_url": fakeBaseEndpoint + "/sso/approve",
					"detail":     "harness fake: waiting for out-of-band approval",
				}
				return model.ToolResult{StructuredContent: sc, Text: toolargs.ResultJSONText(sc)}, nil
			}
			sc := map[string]any{
				"status":  "done",
				"detail":  "harness fake: sign-in approval simulated for " + args.Handle,
				"profile": f.Email,
			}
			return model.ToolResult{StructuredContent: sc, Text: toolargs.ResultJSONText(sc)}, nil
		},
	}
}

// ipfsUploadSubmitDescriptor mints (or continues) a one-time presigned PUT
// endpoint bound to a canonical upload handle, exactly like the CLI's Upload
// to IPFS app-only helper. The URL the iframe PUTs bytes to is served by the
// same transfer.Upload coordinator main.go mounts (HTTP mode) or spins up on
// the loopback listener (stdio mode, via Mint/Prepare's EnsureLoopback).
func ipfsUploadSubmitDescriptor(h *Harness) model.ToolDescriptor {
	in := helperSchema(`{
		"handle":{"type":"string","description":"Optional canonical upload handle to continue instead of minting a new one."},
		"name":{"type":"string","description":"Optional upload name (defaults to 'upload')."},
		"ttl":{"type":"string","description":"Presigned endpoint lifetime (e.g. 5m; default 5 minutes)."}
	}`)
	return model.ToolDescriptor{
		Name:        "ipfs_upload_submit",
		Title:       "Prepare a one-time upload endpoint",
		Description: "Prepare (or continue) a one-time presigned HTTP PUT endpoint bound to a canonical upload handle; the app's Uppy XHR uploader writes file bytes to it out of band. App-only helper for the Upload to IPFS view.",
		InputSchema: in,
		Handler: func(ctx context.Context, req model.ToolRequest) (model.ToolResult, error) {
			args, err := toolargs.DecodeToolArgs[struct {
				Handle string `json:"handle"`
				Name   string `json:"name"`
				TTL    string `json:"ttl"`
			}](req)
			if err != nil {
				return model.ToolResult{}, err
			}
			ttl := parseHelperTTL(args.TTL)

			if args.Handle != "" {
				if url, ok := h.Upload.FindUpload(args.Handle); ok {
					sc := map[string]any{
						"url":           url,
						"upload_handle": args.Handle,
						"ttl":           ttl.String(),
						"max_bytes":     h.Upload.MaxBytes(),
						"poll_tool":     "ipfs_upload_status",
						"continued":     true,
					}
					return model.ToolResult{StructuredContent: sc, Text: toolargs.ResultJSONText(sc)}, nil
				}
				if task, terr := h.Upload.Tasks().Get(args.Handle); terr == nil {
					sc := map[string]any{
						"upload_handle":   args.Handle,
						"already_claimed": true,
						"state":           task.State,
						"poll_tool":       "ipfs_upload_status",
					}
					return model.ToolResult{StructuredContent: sc, Text: toolargs.ResultJSONText(sc)}, nil
				}
				return model.ToolResult{IsError: true, Text: fmt.Sprintf("unknown upload handle %q; start a fresh upload", args.Handle)}, nil
			}

			name := args.Name
			if name == "" {
				name = mptransfer.DefaultUploadName
			}
			url, handle := h.Upload.Prepare(ctx, name, ttl)
			if url == "" || handle == "" {
				return model.ToolResult{}, fmt.Errorf("failed to prepare one-time upload endpoint")
			}
			sc := map[string]any{
				"url":           url,
				"upload_handle": handle,
				"ttl":           ttl.String(),
				"max_bytes":     h.Upload.MaxBytes(),
				"poll_tool":     "ipfs_upload_status",
				"response_body": "the 202 body carries the upload_handle; pass it to poll_tool",
			}
			return model.ToolResult{StructuredContent: sc, Text: toolargs.ResultJSONText(sc) + " PUT the file bytes and poll for the CID."}, nil
		},
	}
}

// ipfsUploadStatusDescriptor poll helper for the Upload to IPFS view: reads
// the shared UploadTaskManager for the terminal state / fake CID.
func ipfsUploadStatusDescriptor(h *Harness) model.ToolDescriptor {
	in := helperSchema(`{"handle":{"type":"string","description":"Opaque upload handle returned in the presigned upload's 202 response body."},"required":{}}`)
	return model.ToolDescriptor{
		Name:        "ipfs_upload_status",
		Title:       "Get upload status",
		Description: "Return the status of an async upload by handle: prepared, queued, running, completed (with CID), failed, or cancelled. App-only helper for the Upload to IPFS view.",
		InputSchema: in,
		Handler: func(_ context.Context, req model.ToolRequest) (model.ToolResult, error) {
			args, err := toolargs.DecodeToolArgs[struct {
				Handle string `json:"handle"`
			}](req)
			if err != nil {
				return model.ToolResult{}, err
			}
			if args.Handle == "" {
				return model.ToolResult{IsError: true, Text: "handle is required"}, nil
			}
			task, err := h.Upload.Tasks().Get(args.Handle)
			if err != nil {
				return model.ToolResult{IsError: true, Text: err.Error()}, nil
			}
			return model.ToolResult{StructuredContent: task, Text: toolargs.ResultJSONText(task)}, nil
		},
	}
}

// vaultUploadSubmitDescriptor mints a presigned PUT URL for the Upload to
// Vault view. The harness uploads land in the same fake IPFS sink (a real
// vault upload requires a live Sia vault, which the harness deliberately
// fakes), so the 202-CORS-PUT surface is exercised identically.
func vaultUploadSubmitDescriptor(h *Harness) model.ToolDescriptor {
	in := helperSchema(`{
		"name":{"type":"string","description":"Optional upload name (defaults to 'upload')."},
		"ttl":{"type":"string","description":"Presigned endpoint lifetime (e.g. 5m; default 5 minutes)."}
	}`)
	return model.ToolDescriptor{
		Name:        "vault_upload_submit",
		Title:       "Prepare a one-time vault upload endpoint",
		Description: "Prepare a one-time presigned HTTP PUT endpoint; the app's Uppy XHR uploader writes file bytes to it out of band. App-only helper for the Upload to Vault view.",
		InputSchema: in,
		Handler: func(ctx context.Context, req model.ToolRequest) (model.ToolResult, error) {
			args, err := toolargs.DecodeToolArgs[struct {
				Name string `json:"name"`
				TTL  string `json:"ttl"`
			}](req)
			if err != nil {
				return model.ToolResult{}, err
			}
			ttl := parseHelperTTL(args.TTL)
			name := args.Name
			if name == "" {
				name = mptransfer.DefaultUploadName
			}
			url, handle := h.Upload.Prepare(ctx, name, ttl)
			if url == "" || handle == "" {
				return model.ToolResult{}, fmt.Errorf("failed to prepare one-time vault upload endpoint")
			}
			sc := map[string]any{
				"url":           url,
				"upload_handle": handle,
				"ttl":           ttl.String(),
				"max_bytes":     h.Upload.MaxBytes(),
				"poll_tool":     "ipfs_upload_status",
				"response_body": "the 202 body carries the upload_handle; pass it to poll_tool",
			}
			return model.ToolResult{StructuredContent: sc, Text: toolargs.ResultJSONText(sc) + " PUT the file bytes and poll for the CID."}, nil
		},
	}
}

// parseHelperTTL parses the optional ttl duration argument (default: the
// mcpplane presigned default).
func parseHelperTTL(raw string) time.Duration {
	if raw == "" {
		return mptransfer.DefaultHTTPUploadTTL
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return mptransfer.DefaultHTTPUploadTTL
	}
	return d
}
