package pinnermcp

import (
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/pinner/catalogmcp"
	"go.lumeweb.com/pinner/pinnerops"
)

// Config holds everything needed to assemble the Pinner MCP presentation
// surface independent of CLI flags, transport, or tunnelling — the distilled,
// instance-scoped form of pinner-cli's ServerConfig (which carried CLI-lived
// fields like *cli.Command pointers, OOB links, and custom-tool closure
// wiring that this package must never see). A hosted product is a DIFFERENT
// ASSEMBLY of the same presentation, declared entirely by Config fields.
//
// Fitness rule 13 (no package globals) applies: the surface, hosted flag,
// profile, transfer wiring, resource providers, and app registry are all
// instance fields consumed only through the assembled Server. The historic
// pinner-cli package-global setters (SetSurface / SetHosted /
// SetTransportFlags and friends) are superseded by these fields — no
// equivalent setter exists here.
type Config struct {
	// Surface declares which operation domains/tool families this server
	// exposes. The zero value is the full surface.
	Surface pinnerops.Surface

	// Hosted reports whether this is a hosted (Portal-embedded) assembly. It
	// is the single, explicit source of truth for hosted mode: the guide's
	// hosted notices, the prompt surface, and the resource gating all render
	// from this flag rather than from structural signals that are orthogonal
	// to deployment context.
	Hosted bool

	// Profile is the platform profile the presentation resolves against at
	// assembly time. It is opaque; accepted shapes (see AdaptHostProfile):
	//   - nil — intentionally profile-less: feature-gated description segments
	//     resolve as omitted (the base description). Documented and meaningful.
	//   - a catalogmcp.ForgeFeatureCarrier (including catalogmcp.MCPProfile,
	//     which catalogops.ProfileFromHas produces for Has-style host profiles).
	//   - a *canimcp.Profile or a *model.Profile.
	//
	// Profile flows to BOTH the catalog compiler
	// (catalogmcp.NewCompilerForProfile — DescFunc targets resolve against it)
	// and the direct presentation (transfer-tool descriptions, capabilities).
	Profile any

	// Deps, when set, supplies the operation-catalog dependency bundle and the
	// catalog is assembled via pinnerops.AssembleCatalogOps(deps, Surface,
	// Hosted). A hosted server MUST supply Deps (or Catalog) — the catalog is
	// the source of the compiled tool surface.
	Deps *pinnerops.CatalogDepsBundle

	// Catalog, when set, is consumed as-is instead of assembling from Deps.
	// It exists so a composition root that already owns an assembled
	// opmesh.Catalog can bridge it without a second registration pass. When
	// both Catalog and Deps are set, Catalog wins.
	Catalog opmesh.Catalog

	// Transfer carries the transfer-tool wiring (executor fns and coordinator
	// pointers mcpplane/transfer + pinnertransfer own) and the registration-
	// time flags the capabilities tool must stay honest about. Zero fields
	// mean the corresponding tool is simply not registered. See TransferDeps.
	Transfer TransferDeps

	// ResourceProviders supplies the pinner:// resource dependencies. A zero
	// value yields static-descriptor set with associations to no-go handlers
	// that fail a read with a clear "provider not configured" error, exactly
	// like the source's nil-provider handling.
	ResourceProviders ResourceProviders

	// NOTE: no AppRegistry field. The MCP Apps registry (mcpplane/apps.
	// AppRegistry) sits behind mcpplane/sdk, which imports the MCP SDK —
	// including a *apps.AppRegistry here would pull that SDK into every
	// pinnermcp consumer, violating the package's import-graph isolation. The
	// registry stays a composition-root seam until the Apps registration spec
	// is flattened; nothing in pinnermcp needs it (the guide references
	// open_app as prose only).
}

// compileProfile returns the any-typed profile handed to
// catalogmcp.NewCompilerForProfile. It is the adapted-shape pass-through: the
// configured profile is adopted as-is (catalogmcp adapts ForgeFeatureCarrier
// values and reports any other non-nil shape as a gap), so a Config that
// carries a *canimcp.Profile must first be normalised via AdaptHostProfile by
// Assemble — see compiledProfileFor.
func compiledProfileFor(cfg Config) (any, error) {
	if cfg.Profile == nil {
		return nil, nil
	}
	hp, err := AdaptHostProfile(cfg.Profile)
	if err != nil {
		return nil, err
	}
	return hp, nil
}

// CompileCatalog compiles the configured catalog for the model surface using
// the configured profile. It is the single place pinnermcp touches
// catalogmcp's compiler, mirroring the de-globalized startup profile the
// source derived from set-boxed transport flags.
func CompileCatalog(cfg Config, cat opmesh.Catalog) ([]opmesh.ToolDescriptor, error) {
	profile, err := compiledProfileFor(cfg)
	if err != nil {
		return nil, err
	}
	return catalogmcp.NewCompilerForProfile(profile).Compile(cat)
}

// HostProfileOf resolves Config.Profile into a HostProfile for the direct
// presentation (transfer-tool descriptions, capabilities descriptors,
// assembly-time option resolution). It is the run-time replacement for the
// source's startupProfile() global reader.
func HostProfileOf(cfg Config) (HostProfile, error) {
	return AdaptHostProfile(cfg.Profile)
}
