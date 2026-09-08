package pinnermcp

import (
	"fmt"

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
	profile, err := AdaptHostProfile(cfg.Profile)
	if err != nil {
		return nil, fmt.Errorf("pinnermcp: assemble: %w", err)
	}

	cat, err := resolveCatalog(cfg)
	if err != nil {
		return nil, err
	}

	presentations, err := populateCatalogSurface(cat, compileProfile(cfg))
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
		RelayFeatures: wiring.RelayFeatures,
	}))

	if wiring.UploadFile {
		features := wiring.RelayFeatures
		if features == nil {
			features = transportFeaturesFor(UploadFileTransport(wiring.CoLocated, wiring.TunnelOpenAI))
		}
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
	if wiring.DataURIWired {
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

// resolveCatalog resolves the operation catalog for the assembly: a
// pre-assembled Catalog when configured, else pinnerops.AssembleCatalogOps
// over the Deps bundle. A nil catalog (neither configured) is an error.
func resolveCatalog(cfg Config) (opmesh.Catalog, error) {
	if cfg.Catalog != nil {
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

// compileProfile adapts the configured profile into the shape the catalogmcp
// compiler consumes. A nil Config.Profile passes through as nil — the
// documented profile-less case that makes the compiler fall back to the
// operation's own description instead of resolving DescFunc targets. Any
// configured shape is passed as the adapted HostProfile (Assemble validated
// it already); HostProfile is an mcpforge FeatureCarrier, so the compiler's
// resolvers adopt it directly without a second adaptation hop.
func compileProfile(cfg Config) any {
	if cfg.Profile == nil {
		return nil
	}
	hp, err := AdaptHostProfile(cfg.Profile)
	if err != nil {
		// Unreachable: Assemble validated the same shape before calling.
		return nil
	}
	return hp
}
