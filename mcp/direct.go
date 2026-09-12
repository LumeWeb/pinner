package mcp

import (
	"strings"

	"go.lumeweb.com/mcpplane/model"

	"go.lumeweb.com/pinner/assembly"
	"go.lumeweb.com/pinner/catalogmcp"
)

const (
	catalogCategoryVault model.ToolCategory = "vault"
	catalogCategoryIPNS  model.ToolCategory = "ipns"
)

// compiledDirectToolNames is the product surface of operations exposed
// directly (tools/list) in addition to progressive discovery, for the FULL
// domain scope (CLI / local MCP). It is the single source of truth for which
// catalog tools are directly visible; applying it to the compiled catalog
// surface stamps each descriptor's DirectVisible flag (see stampDirect).
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
var compiledDirectToolNames = []string{
	"auth_status",
	"vault_create",
	"vault_restore",
	"vault_status",
	"vault_share_accept",
	"websites_create",
	"websites_get",
}

// DirectToolNames returns the direct tools/list names for the given domain
// scope. The full scope is compiledDirectToolNames (auth status + vault
// lifecycle + website publishing). A scope without the Sia vault drops the
// vault lifecycle/share entries, leaving auth status and website publishing —
// the hosted (account/IPFS/websites) facing set.
func DirectToolNames(s assembly.DomainScope) []string {
	if s.AccountOn() && !s.VaultOn() && s.WebsitesOn() {
		return []string{"auth_status", "websites_create", "websites_get"}
	}
	return compiledDirectToolNames
}

// flatToolNames returns the compiled presentation names promoted by a flat
// listing policy and allowed by the deployment scope. The projection has
// already excluded model-ineligible, admin, wizard, and non-agent-safe
// operations; the scope check remains necessary for caller-supplied catalogs
// that were not assembled through the normal scope-filtering path.
func flatToolNames(descriptors []CatalogPresentation, scope assembly.DomainScope) []string {
	names := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if descriptor.DirectVisible && scopeAllowsTool(scope, descriptor.Name, descriptor.Category) {
			names = append(names, descriptor.Name)
		}
	}
	return names
}

func scopeAllowsTool(scope assembly.DomainScope, name string, category model.ToolCategory) bool {
	if scope.IsZero() {
		return true
	}

	switch category {
	case model.CategoryAccount:
		return scope.AccountOn()
	case model.CategoryStorage, catalogCategoryVault:
		return scope.VaultOn()
	case model.CategoryNames, catalogCategoryIPNS:
		return scope.IPNSOn()
	case model.CategoryOperations:
		return scope.OperationsOn()
	case model.CategoryAdmin:
		return scope.AdminOn()
	}

	prefixes := []struct {
		prefix  string
		enabled func() bool
	}{
		{"auth_", scope.AccountOn},
		{"apikeys_", scope.AccountOn},
		{"account_", scope.AccountOn},
		{"vault_", scope.VaultOn},
		{"pins_", scope.PinsOn},
		{"websites_", scope.WebsitesOn},
		{"dns_", scope.DNSOn},
		{"ipns_", scope.IPNSOn},
		{"ens_", scope.ENSOn},
		{"operations_", scope.OperationsOn},
		{"admin_", scope.AdminOn},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p.prefix) {
			return p.enabled()
		}
	}
	return true
}

// stampDirect stamps DirectVisible=true on the assembled catalog descriptors
// named by the direct set for the configured domain scope. The presentation
// layer reads DirectVisible rather than re-checking a name predicate, so
// visibility is a property of the descriptor; a direct name absent from the
// scope (e.g. a vault tool on a hosted scope) is simply never stamped and
// never advertised, exactly as the scope gate already excluded it.
func stampDirect(names []string, descriptors []CatalogPresentation) {
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
