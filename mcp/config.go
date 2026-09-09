package mcp

import (
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/pinner/assembly"
)

// Config holds everything needed to assemble the Pinner MCP presentation
// surface independent of transport, tunnelling, or any particular
// composition root: the surface, hosted flag, profile, transfer wiring, and
// resource providers are all declared as instance-scoped fields. A hosted
// product is a DIFFERENT ASSEMBLY of the same presentation, declared entirely
// by Config fields.
//
// There are no package globals: the surface, hosted flag, profile, transfer
// wiring, resource providers, and app registry are all instance fields
// consumed only through the assembled Server.
type Config struct {
	// Surface declares which operation domains/tool families this server
	// exposes. The zero value is the full surface.
	Surface assembly.Surface

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
	// catalog is assembled via assembly.AssembleCatalogOps(deps, Surface,
	// Hosted). A hosted server MUST supply Deps (or Catalog) — the catalog is
	// the source of the compiled tool surface.
	Deps *assembly.CatalogDepsBundle

	// Catalog, when set, is consumed as-is instead of assembling from Deps.
	// It exists so a composition root that already owns an assembled
	// opmesh.Catalog can bridge it without a second registration pass. When
	// both Catalog and Deps are set, Catalog wins. A typed-nil Catalog (an
	// interface variable holding a nil concrete value) reads as UNSET — the
	// Deps path applies, since a nil concrete catalog is not a usable
	// registry.
	Catalog opmesh.Catalog

	// Transfer carries the transfer-tool wiring (executor fns and coordinator
	// pointers mcpplane/transfer + transfer own) and the registration-
	// time flags the capabilities tool must stay honest about. Zero fields
	// mean the corresponding tool is simply not registered. See TransferDeps.
	Transfer TransferDeps

	// ResourceProviders supplies the pinner:// resource dependencies. A zero
	// value yields a static-descriptor set with associations to no-op handlers
	// that fail a read with a clear "provider not configured" error.
	ResourceProviders ResourceProviders

	// NOTE: no AppRegistry field. The MCP Apps registry (mcpplane/apps.
	// AppRegistry) sits behind mcpplane/sdk, which imports the MCP SDK —
	// including a *apps.AppRegistry here would pull that SDK into every
	// consumer, violating the package's import-graph isolation. The registry
	// stays a composition-root seam; nothing in this package needs it (the
	// guide references open_app as prose only).
}

// HostProfileOf resolves Config.Profile into a HostProfile for the direct
// presentation (transfer-tool descriptions, capabilities descriptors,
// assembly-time option resolution).
func HostProfileOf(cfg Config) (HostProfile, error) {
	return AdaptHostProfile(cfg.Profile)
}
