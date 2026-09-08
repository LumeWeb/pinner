package pinnermcp

import (
	"go.lumeweb.com/pinner/catalogmcp"
	"go.lumeweb.com/pinner/pinnerops"
)

// compiledCuratedToolNames is the product surface of operations exposed
// directly (tools/list) in addition to progressive discovery, for the FULL
// surface (CLI / local MCP). It is the single source of truth for which
// catalog tools are directly visible; applying it to the compiled catalog
// surface stamps each descriptor's DirectVisible flag (see stampCurated).
// Keep the names in a stable, human-reviewable order.
//
// This is a deliberately small front door. The full tool catalog (~170 ops)
// remains behind progressive discovery. Everything listed here is either
// essential for first-call orientation (auth_status), vault lifecycle entry
// points (vault_create, vault_restore, vault_status), the vault's distinctive
// share primitive (vault_share_accept), or website publishing
// (websites_create, websites_get). All other operations — pins CRUD, vault
// file ops, DNS, IPNS, admin, wizards — are discoverable via search. The
// agent_guide tool names the daily-use verbs in its flows, so an agent
// reading the guide learns which tools to search for.
var compiledCuratedToolNames = []string{
	"auth_status",
	"vault_create",
	"vault_restore",
	"vault_status",
	"vault_share_accept",
	"websites_create",
	"websites_get",
}

// CuratedToolNames returns the curated tools/list names for the given surface.
// The full surface is compiledCuratedToolNames (auth status + vault lifecycle
// + website publishing). A surface without the Sia vault drops the vault
// lifecycle/share entries, leaving auth status and website publishing — the
// hosted (account/IPFS/websites) facing set.
func CuratedToolNames(s pinnerops.Surface) []string {
	if s.AccountOn() && !s.VaultOn() && s.WebsitesOn() {
		return []string{"auth_status", "websites_create", "websites_get"}
	}
	return compiledCuratedToolNames
}

// stampCurated stamps DirectVisible=true on the assembled catalog descriptors
// named by the curated set for the configured surface. The presentation layer
// reads DirectVisible rather than re-checking a name predicate, so visibility
// is a property of the descriptor; a curated name absent from the surface
// (e.g. a vault tool on a hosted surface) is simply never stamped and never
// advertised, exactly as the surface gate already excluded it.
func stampCurated(names []string, descriptors []CatalogPresentation) {
	visible := make(map[string]struct{}, len(names))
	for _, name := range names {
		visible[name] = struct{}{}
	}
	for i := range descriptors {
		if _, ok := visible[descriptors[i].Name]; ok {
			descriptors[i].DirectVisible = true
		}
	}
}

// ensure catalogmcp's profile adapter vocabulary is linked into this file's
// doc surface: Config.Profile accepts catalogmcp ForgeFeatureCarrier values
// (including catalogmcp.MCPProfile, which ProfileFromHas produces) alongside
// *canimcp.Profile and *model.Profile — see AdaptHostProfile.
var _ catalogmcpForgeCarrier = catalogmcp.MCPProfile{}
