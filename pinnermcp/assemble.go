package pinnermcp

import (
	"fmt"
	"reflect"

	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/mcpplane/model"

	"go.lumeweb.com/pinner/pinnerops"
)

// Assemble is the single construction seam for the Pinner MCP presentation
// layer. It accepts explicit dependencies only — an operation-catalog deps
// bundle (or a pre-assembled catalog), the transfer wiring, resource
// providers — and produces the fully-assembled presentation artifacts:
//
//   - Tools: the compiled catalog surface (via catalogmcp over the profile)
//     projected onto model descriptors, with the curated set stamped
//     DirectVisible.
//   - Direct: the direct-only tools outside the catalog — agent_guide,
//     capabilities, and the wired transfer tools (upload_file, upload_data,
//     download_file).
//   - Prompts / Resources / ResourceTemplates: the surface-gated pinner://
//     presentation set.
//
// It does NOT wire a protocol server, dispatch, or a transport: orchestration
// stays with the composition root, which consumes Catalog.Invoke through the
// standard gates for all Tools. Assemble never accepts a *cli.Command and
// never calls a CLI service factory — every dependency is explicit, per
// Stage 5 of the package-boundaries overhaul.
func Assemble(cfg Config) (*Server, error) {
	// The configured profile is adapted EXACTLY ONCE, here, and the resulting
	// HostProfile flows to both the direct presentation (this Server) and the
	// catalog compiler below. Errors propagate loudly — an un-adaptable
	// profile is the assembly mistake it is, never a silently featureless
	// description surface (see ProfileAdapterError).
	profile, err := HostProfileOf(cfg)
	if err != nil {
		return nil, fmt.Errorf("pinnermcp: assemble: %w", err)
	}
	// Hosted is server-construction-time state (Config.Hosted), so the
	// normalized profile carries it, keeping one consistent platform context
	// across the assembly even when the wire profile reported itself
	// unhosted. The direct presentation consumes Config.Hosted directly for
	// its hosted gating (AgentGuideDescriptor's hosted notices) and the
	// prompt/resource sets gate on the Surface; the compiled catalog surface
	// gates on feature sets rather than the hosted predicate. The overlay
	// exists so anything resolving HostProfile.Hosted (e.g. the HostedIs
	// predicate fragments) reads the single explicit setting.
	profile.Hosted = cfg.Hosted

	cat, err := resolveCatalog(cfg)
	if err != nil {
		return nil, err
	}

	presentations, err := populateCatalogSurface(cat, profile)
	if err != nil {
		return nil, fmt.Errorf("pinnermcp: assemble: %w", err)
	}

	curated := CuratedToolNames(cfg.Surface)
	stampCurated(curated, presentations)

	srv := &Server{
		config:  cfg,
		catalog: cat,
		profile: profile,
		Tools:   presentations,
		Curated: curated,
	}

	// Prompts and resources are surface-gated presentation sets.
	srv.Prompts = PromptDescriptorsForSurface(cfg.Surface)
	srv.Resources, srv.ResourceTemplates = ResourceDescriptorsForSurface(cfg.ResourceProviders, cfg.Surface)

	// Direct-only tools outside the catalog: the guide is always registered;
	// capabilities with the honest registration wiring; the transfer tools
	// only when their executor is wired.
	srv.Direct = srv.buildDirectTools()

	return srv, nil
}

// Server holds the assembled Pinner MCP presentation surface. It is pure
// instance state: every artifact derives from the Config it was built from,
// and no package-level mutable state is involved anywhere in the assembly
// (fitness rule 13).
type Server struct {
	config  Config
	catalog opmesh.Catalog
	profile HostProfile

	// Tools is the compiled catalog surface: one presentation descriptor per
	// model-visible, MCP-visible operation, with the curated set stamped
	// DirectVisible. Dispatch goes through catalog.Invoke at the composition
	// root — these descriptors carry metadata only.
	Tools []CatalogPresentation

	// Direct carries the direct-only tools outside the catalog: agent_guide,
	// the capabilities tool, and any wired transfer tools.
	Direct []model.ToolDescriptor

	// Curated lists the curated tools/list names for the assembled surface.
	Curated []string

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

// Surface returns the assembled surface (zero value = full surface).
func (s *Server) Surface() pinnerops.Surface { return s.config.Surface }

// Hosted reports whether this is a hosted (Portal-embedded) assembly.
func (s *Server) Hosted() bool { return s.config.Hosted }

// buildDirectTools orders the direct-only tool set: agent_guide first (the
// "start here" orientation), then capabilities, then the wired transfer tools
// in registration order (upload_file, upload_data, download_file).
func (s *Server) buildDirectTools() []model.ToolDescriptor {
	wiring := s.config.Transfer

	// The registration-time effective feature set, derived ONCE and shared by
	// every consumer: the capabilities report AND the upload_file descriptor.
	// With no explicit RelayFeatures it falls back to the transport's
	// startup-effective set — the same fallback pinner-cli's
	// effectiveFeaturesFor applied. For the embedded OpenAI tunnel that set
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

	direct := []model.ToolDescriptor{AgentGuideDescriptor(s.config.Surface, s.config.Hosted)}
	direct = append(direct, NewCapabilitiesDescriptor(CapabilityWiring{
		CoLocated:     wiring.CoLocated,
		TunnelOpenAI:  wiring.TunnelOpenAI,
		UploadFile:    wiring.UploadFile,
		VaultPutFile:  wiring.VaultPutFile,
		DownloadFile:  wiring.DownloadFile,
		VaultGetFile:  wiring.VaultGetFile,
		DropWired:     wiring.DropWired || wiring.FileDrop != nil,
		RelayURLWired: wiring.RelayURLWired,
		DataURIWired:  wiring.DataURIWired,
		DraftXFile:    wiring.DataURIWired,
		RelayMaxBytes: wiring.MaxRelayBytes,
		RelayFeatures: features,
	}))

	if wiring.UploadFile {
		direct = append(direct, NewUploadFileDescriptor(
			features,
			wiring.CoLocated,
			wiring.TunnelOpenAI,
			wiring.PathUpload,
			wiring.PresignedUpload,
			wiring.Relay,
			wiring.RelayAllowedHosts,
			wiring.MaxRelayBytes,
		))
	}
	// upload_data registers only when the wired flag AND the effective
	// feature set both declare the data: URI relay (FeatSourceData) — the
	// same combined condition pinner-cli's registration used, so a tool is
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
	return direct
}

// isNilCatalog reports whether cat is a nil interface or an interface holding
// a typed nil (nil pointer/map/... value). A plain `cat == nil` comparison
// misses the typed-nil shape: an interface variable carrying a nil concrete
// value is non-nil as an interface yet unusable, so the assembly must treat it
// as the absent catalog it effectively is.
func isNilCatalog(cat opmesh.Catalog) bool {
	if cat == nil {
		return true
	}
	v := reflect.ValueOf(cat)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

// resolveCatalog resolves the operation catalog for the assembly: a
// pre-assembled Catalog when configured, else pinnerops.AssembleCatalogOps
// over the Deps bundle. A nil catalog (neither configured — including a
// typed-nil Catalog interface) is an error.
func resolveCatalog(cfg Config) (opmesh.Catalog, error) {
	if !isNilCatalog(cfg.Catalog) {
		return cfg.Catalog, nil
	}
	if cfg.Deps != nil {
		cat, err := pinnerops.AssembleCatalogOps(cfg.Deps, cfg.Surface, cfg.Hosted)
		if err != nil {
			return nil, fmt.Errorf("pinnermcp: assemble: %w", err)
		}
		return cat, nil
	}
	return nil, fmt.Errorf("pinnermcp: assemble: no operation catalog: set Config.Catalog or Config.Deps")
}
