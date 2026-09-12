package mcp

import (
	"fmt"

	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/mcpplane/model"

	"go.lumeweb.com/pinner/assembly"
	"go.lumeweb.com/pinner/catalogmcp"
)

// Assemble is the single construction seam for the Pinner MCP presentation
// layer. It accepts explicit dependencies only — an operation-catalog deps
// bundle (or a pre-assembled catalog), the transfer wiring, resource
// providers — and produces the fully-assembled presentation artifacts:
//
//   - Tools: the compiled catalog surface (via catalogmcp over the profile)
//     projected onto model descriptors, with the direct set stamped
//     DirectVisible.
//   - Direct: the direct-only tools outside the catalog — agent_guide,
//     capabilities, and the wired transfer tools (upload_file, upload_data,
//     download_file).
//   - Prompts / Resources / ResourceTemplates: the surface-gated pinner://
//     presentation set.
//
// It does NOT wire a protocol server, dispatch, or a transport: orchestration
// stays with the composition root, which consumes Catalog.Invoke through the
// standard gates for all Tools. Every dependency is explicit — no hidden
// package state, no assembly-side protocol construction.
func Assemble(cfg Config) (*Server, error) {
	// The configured profile is adapted EXACTLY ONCE, here, and the resulting
	// HostProfile flows to both the direct presentation (this Server) and the
	// catalog compiler below. Errors propagate loudly — an un-adaptable
	// profile is the assembly mistake it is, never a silently featureless
	// description surface (see ProfileAdapterError).
	// The app inventory is the guide's truth input for MCP Apps prose, so it
	// is validated before anything reads it: unknown launcher names fail the
	// assembly (a table-vocabulary lie is a provable one), and a composition
	// root that can see its registry asserts the listed set against the
	// registered one via VerifyInstalledApps. The blessed inventory source is
	// mcp/appswire/install's Install — the names whose views actually
	// registered — which keeps the guide's claims and the wire surface in
	// lockstep by construction.
	if err := validateInstalledApps(cfg.InstalledApps); err != nil {
		return nil, fmt.Errorf("mcp: assemble: %w", err)
	}
	if cfg.VerifyInstalledApps != nil {
		if err := cfg.VerifyInstalledApps(cfg.InstalledApps); err != nil {
			return nil, fmt.Errorf("mcp: assemble: %w", err)
		}
	}
	profile, err := HostProfileOf(cfg)
	if err != nil {
		return nil, fmt.Errorf("mcp: assemble: %w", err)
	}
	// Hosted is server-construction-time state (Config.Hosted), so the
	// normalized profile carries it, keeping one consistent platform context
	// across the assembly even when the wire profile reported itself
	// unhosted. The direct presentation consumes Config.Hosted directly for
	// its hosted gating (AgentGuideDescriptor's hosted notices) and the
	// prompt/resource sets gate on the DomainScope; the compiled catalog surface
	// gates on feature sets rather than the hosted predicate. The overlay
	// exists so anything resolving HostProfile.Hosted (e.g. the HostedIs
	// predicate fragments) reads the single explicit setting.
	profile.Hosted = cfg.Hosted
	// The catalog description DSL resolves against feature sets only (the
	// any-typed boundary carries no HostProfile.Hosted field), so the same
	// deployment fact is mirrored as a feature for the subscription-related
	// account descriptions, whose hosted variant must carry no subscription
	// or upgrade promotion (platform commerce policy). Only the hosted state
	// is stamped: an absent feature resolves as non-hosted, the safe default
	// for profile-less or Has-style-adapted profiles.
	if cfg.Hosted {
		profile = profile.CloneFeatures()
		profile.Features[catalogmcp.FeatHosted] = true
	}

	// Resolve the listing policy ONCE: the explicit Config.Listing when set
	// (validated loudly — an unsupported strategy is a construction mistake),
	// else DefaultPolicy. The shared host selector (PolicyForHost) is what
	// composition roots call to derive the policy for a detected host; Assemble
	// itself never infers a host from feature-carrier profiles that name none.
	listing := DefaultPolicy()
	if cfg.Listing != nil {
		if err := cfg.Listing.Validate(); err != nil {
			return nil, fmt.Errorf("mcp: assemble: %w", err)
		}
		listing = *cfg.Listing
	}

	cat, err := resolveCatalog(cfg)
	if err != nil {
		return nil, err
	}

	presentations, err := populateCatalogTools(cat, profile, listing)
	if err != nil {
		return nil, fmt.Errorf("mcp: assemble: %w", err)
	}

	directNames := DirectToolNames(cfg.DomainScope)
	if listing.Strategy == ListingFlat {
		directNames = flatToolNames(presentations, cfg.DomainScope)
	}
	stampDirect(directNames, presentations)

	srv := &Server{
		config:          cfg,
		catalog:         cat,
		profile:         profile,
		listing:         listing,
		Tools:           presentations,
		DirectToolNames: directNames,
	}

	// Prompts and resources are surface-gated presentation sets.
	srv.Prompts = PromptDescriptorsForScope(cfg.DomainScope, cfg.Hosted)
	srv.Resources, srv.ResourceTemplates = ResourceDescriptorsForScope(cfg.ResourceProviders, cfg.DomainScope)

	// Direct-only tools outside the catalog: the guide is always registered;
	// capabilities with the honest registration wiring; the transfer tools
	// only when their executor is wired.
	srv.Direct = srv.buildDirectTools()

	return srv, nil
}

// Server holds the assembled Pinner MCP presentation surface. It is pure
// instance state: every artifact derives from the Config it was built from,
// and no package-level mutable state is involved anywhere in the assembly.
type Server struct {
	config  Config
	catalog opmesh.Catalog
	profile HostProfile

	// Tools is the compiled catalog surface: one presentation descriptor per
	// model-visible, MCP-visible operation, with the direct set stamped
	// DirectVisible. Dispatch goes through catalog.Invoke at the composition
	// root — these descriptors carry metadata only.
	Tools []CatalogPresentation

	// listing is the resolved tool-listing policy (Config.Listing or the
	// DefaultPolicy): the shared strategy/meta-on-flat axes the composition
	// root's tools/list materialization honors. Exposed read-only via
	// ListingPolicy().
	listing ListingPolicy

	// Direct carries the direct-only tools outside the catalog: agent_guide,
	// the capabilities tool, and any wired transfer tools.
	Direct []model.ToolDescriptor

	// DirectToolNames lists the direct tools/list names for the assembled domain scope.
	DirectToolNames []string

	// Prompts is the surface-gated prompt set (4 Pinner workflows).
	Prompts []model.PromptDescriptor

	// Resources / ResourceTemplates are the pinner:// resource descriptors.
	Resources         []model.ResourceDescriptor
	ResourceTemplates []model.ResourceTemplateDescriptor
}

// Config returns the assembly configuration the server was built from.
func (s *Server) Config() Config { return s.config }

// Catalog returns the owning operation catalog. The composition root
// dispatches every Tools invocation through catalog.Invoke so the
// Interaction, Visibility, and Safety gates hold.
func (s *Server) Catalog() opmesh.Catalog { return s.catalog }

// Profile returns the resolved HostProfile the presentation was assembled
// with. The zero value is the documented intentionally profile-less case.
func (s *Server) Profile() HostProfile { return s.profile }

// DomainScope returns the assembled domain scope (zero value = full scope).
func (s *Server) DomainScope() assembly.DomainScope { return s.config.DomainScope }

// Hosted reports whether this is a hosted (Portal-embedded) assembly.
func (s *Server) Hosted() bool { return s.config.Hosted }

// ListingPolicy returns the resolved tool-listing policy for this assembly
// (Config.Listing when declared, else DefaultPolicy). The registration /
// materialization seam consults it to honor the listing strategy — flat for
// the flat-listing web hosts (Claude Web, Grok Web, ChatGPT/OpenAI web)
// selected by PolicyForHost, progressive otherwise — so the assembled
// presentation, the meta-tool decision, and any derived instructions/card all
// read the one policy value.
func (s *Server) ListingPolicy() ListingPolicy { return s.listing }

// buildDirectTools orders the direct-only tool set: agent_guide first (the
// "start here" orientation), then capabilities, then the wired transfer tools
// in registration order (upload_file, upload_url, upload_data,
// download_file): the relay-fetched URL upload sits between the file upload
// and the inline data upload, matching the capabilities chooser order.
func (s *Server) buildDirectTools() []model.ToolDescriptor {
	wiring := s.config.Transfer

	// The registration-time effective feature set, derived ONCE and shared by
	// every consumer: the capabilities report AND the upload_file descriptor.
	// With no explicit RelayFeatures it falls back to the transport's
	// startup-effective set. For the embedded OpenAI tunnel that set
	// carries the ChatGPT host capabilities (FeatFileHostInput, FeatXMcpFile,
	// FeatMCPApps, FeatElicitation) merged into the mechanism set, so a
	// tunnel assembled with RelayFeatures unset publishes the FULL surface
	// (host-file schema property, ChatGPT metadata, host-file description
	// segments, relay-tool registration honesty) instead of silently
	// degrading to the mechanism-only shape. Deriving it once here keeps the
	// registered tools and the advertised capabilities from ever disagreeing.
	features := wiring.RelayFeatures
	if features == nil {
		features = transportStartupFeatures(UploadFileTransport(wiring.CoLocated, wiring.TunnelOpenAI))
	}

	// The relay-tool gates, computed ONCE and consumed by BOTH the
	// registration branches below and the capabilities report: registration
	// and advertising can never disagree. upload_url additionally requires
	// the executor itself (TransferDeps.RelayURLRegistered — the honest gate:
	// RelayURLWired && Relay != nil && FeatSourceURL).
	relayURLWired := wiring.RelayURLRegistered(features)

	// The copy feature set: when a relay tool is NOT registered, its feature
	// must not drive any description/schema copy that names the tool —
	// otherwise the guide, the capabilities chooser, and upload_file's
	// source.mode prose would advertise a tool tools/list does not serve. The
	// stripes are additive to the clamps capabilitiesDescriptionFor already
	// applies (host-file/drop need wired tools too).
	featuresForCopy := features
	strip := func(f mcpforge.Feature) {
		if featuresForCopy.Has(f) {
			featuresForCopy = featuresForCopy.Clone()
			delete(featuresForCopy, f)
		}
	}
	if !relayURLWired {
		strip(FeatSourceURL)
	}
	// upload_data is registered only when the wired flag AND the effective
	// feature set both declare the data: URI relay (FeatSourceData) — the
	// same combined condition as the registration branch below — so when
	// either half fails, the data feature must not drive any copy that
	// names the tool either.
	if !(wiring.DataURIWired && features.Has(FeatSourceData)) {
		strip(FeatSourceData)
	}
	// The drop sink is only real when a FileDrop coordinator is wired and the
	// transport has a reachable HTTP mux (not the embedded OpenAI tunnel) —
	// the exact condition the capabilities report's sinkModesFor and the
	// download_file tool's drop gate (DownloadSinksAllowed with hd != nil)
	// apply. Otherwise no download tool accepts sink=drop, so the
	// FeatSinkDrop-bearing copy (the capabilities description's drop prose)
	// must not advertise it either.
	dropAvailable := wiring.FileDrop != nil && !wiring.TunnelOpenAI
	if !dropAvailable {
		strip(FeatSinkDrop)
	}

	direct := []model.ToolDescriptor{AgentGuideDescriptor(s.config.DomainScope, s.config.Hosted, dropAvailable, s.config.InstalledApps)}
	direct = append(direct, NewCapabilitiesDescriptor(CapabilityWiring{
		CoLocated:     wiring.CoLocated,
		TunnelOpenAI:  wiring.TunnelOpenAI,
		UploadFile:    wiring.UploadFile,
		VaultPutFile:  wiring.VaultPutFile,
		DownloadFile:  wiring.DownloadFile,
		VaultGetFile:  wiring.VaultGetFile,
		DropWired:     wiring.FileDrop != nil,
		RelayURLWired: relayURLWired,
		DataURIWired:  wiring.DataURIWired,
		DraftXFile:    wiring.DataURIWired,
		RelayMaxBytes: wiring.MaxRelayBytes,
		RelayFeatures: featuresForCopy,
	}))

	if wiring.UploadFile {
		direct = append(direct, NewUploadFileDescriptor(
			featuresForCopy,
			wiring.CoLocated,
			wiring.TunnelOpenAI,
			wiring.PathUpload,
			wiring.PresignedUpload,
			wiring.Relay,
			wiring.RelayAllowedHosts,
			wiring.MaxRelayBytes,
		))
	}
	// The async upload-management tools (upload_status / upload_cancel /
	// upload_list) register exactly when a presigned upload coordinator is
	// wired. Mint (source.mode=mint) returns an upload_handle the caller must
	// poll via upload_status until a terminal state; upload_file's own
	// description and the agent guide name upload_status as that completion
	// contract, so a server with a presigned PUT coordinator but no async
	// surface would advertise a poll tools/list does not serve.
	//
	// upload_list (the enumerator) is registered only when the composition
	// root opts in via TransferDeps.AsyncUploadList: it discloses every handle
	// the manager tracks, which is unsafe on a manager shared across principals
	// in a multi-tenant deployment. upload_status / upload_cancel are
	// capability-guarded by the opaque minted handle and always register.
	if wiring.UploadFile && wiring.PresignedUpload != nil {
		for _, d := range NewAsyncUploadTools(wiring.PresignedUpload.Tasks()) {
			if d.Name == "upload_list" && !wiring.AsyncUploadList {
				continue
			}
			direct = append(direct, d)
		}
	}
	// upload_url registers when (and ONLY when) the reconciled gate holds:
	// RelayURLWired && Relay != nil && FeatSourceURL — the exact gate the
	// capabilities report above consumed, so the registered set and the
	// advertised set can never disagree.
	if relayURLWired {
		direct = append(direct, RelayURLUploadDescriptor(
			wiring.Relay,
			wiring.RelayAllowedHosts,
			wiring.MaxRelayBytes,
			featuresForCopy,
		))
	}
	// upload_data registers only when the wired flag AND the effective
	// feature set both declare the data: URI relay (FeatSourceData) — the
	// same combined condition the capabilities report uses, so a tool is
	// never registered without the capabilities report advertising it (and
	// vice versa).
	if wiring.DataURIWired && features.Has(FeatSourceData) {
		direct = append(direct, DataURIUploadDescriptor(wiring.Relay, wiring.MaxRelayBytes))
	}
	if wiring.DownloadFile {
		direct = append(direct, NewDownloadFileDescriptor(
			wiring.IPFSDownload,
			wiring.FileDrop,
			wiring.DownloadRoot,
			wiring.MaxDownloadBytes,
			wiring.TunnelOpenAI,
		))
	}
	// The dev_* introspection tools are direct-only diagnostics: never part of
	// the production surface, appended only when Config.DevTools declares them.
	if s.config.DevTools {
		direct = append(direct, devToolDescriptors()...)
	}
	return direct
}

// isNilCatalog reports whether cat is a nil interface or an interface holding
// a typed nil (nil pointer/map/... value). An interface variable carrying a
// nil concrete value is non-nil as an interface yet unusable, so the assembly
// must treat it as the absent catalog it effectively is.
func isNilCatalog(cat opmesh.Catalog) bool {
	return isNilValue(cat)
}

// resolveCatalog resolves the operation catalog for the assembly: a
// pre-assembled Catalog when configured, else assembly.AssembleCatalogOps
// over the Deps bundle. A nil catalog (neither configured — including a
// typed-nil Catalog interface) is an error.
func resolveCatalog(cfg Config) (opmesh.Catalog, error) {
	if !isNilCatalog(cfg.Catalog) {
		return cfg.Catalog, nil
	}
	if cfg.Deps != nil {
		cat, err := assembly.AssembleCatalogOps(cfg.Deps, cfg.DomainScope, cfg.Hosted)
		if err != nil {
			return nil, fmt.Errorf("mcp: assemble: %w", err)
		}
		return cat, nil
	}
	return nil, fmt.Errorf("mcp: assemble: no operation catalog: set Config.Catalog or Config.Deps")
}
