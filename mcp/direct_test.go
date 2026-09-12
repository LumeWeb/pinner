package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/pinner/assembly"
)

// Characterization of the direct tool-name sets: the full domain scope exposes
// the seven hand-picked names; a hosted-shaped scope (account+websites, no
// vault) drops exactly the vault lifecycle/share entries.
func TestDirectToolNamesFullDomainScope(t *testing.T) {
	require.Equal(t, []string{
		"auth_status",
		"vault_create",
		"vault_restore",
		"vault_status",
		"vault_share_accept",
		"websites_create",
		"websites_get",
	}, DirectToolNames(assembly.FullDomainScope))
}

func TestDirectToolNamesHostedDomainScope(t *testing.T) {
	require.Equal(t, []string{"auth_status", "websites_create", "websites_get"},
		DirectToolNames(assembly.HostedDomainScope))
}

func TestDirectToolNamesZeroDomainScopeIsFull(t *testing.T) {
	// The zero DomainScope is the implicit FULL scope; the direct set must
	// treat it identically to FullDomainScope.
	require.Equal(t, DirectToolNames(assembly.DomainScope{}), DirectToolNames(assembly.FullDomainScope))
}

func TestDirectToolNamesVaultScopeKeepsVaultEntries(t *testing.T) {
	// An account+vault scope with websites disabled cannot match the hosted
	// branch (needs account+vault-less+websites), so it keeps the full set.
	s := assembly.DomainScope{Account: true, Vault: true}
	require.Equal(t, compiledDirectToolNames, DirectToolNames(s))
}

// stampDirect pins DirectVisible exactly on the names in the direct set for
// the scope: names absent from the descriptor set (e.g. vault tools on a
// hosted scope) are ignored, present ones are stamped.
func TestStampDirectMarksOnlyDirectNames(t *testing.T) {
	descs := []CatalogPresentation{
		{Name: "auth_status"},
		{Name: "vault_create"},
		{Name: "websites_create"},
		{Name: "pins_list"},
	}
	stampDirect(DirectToolNames(assembly.HostedDomainScope), descs)

	require.True(t, descs[0].DirectVisible, "auth_status must be direct-visible")
	require.False(t, descs[1].DirectVisible, "vault_create is not direct on the hosted scope")
	require.True(t, descs[2].DirectVisible, "websites_create must be direct-visible")
	require.False(t, descs[3].DirectVisible, "non-direct ops stay progressive-discovery only")
}

func TestFlatToolNamesHonorsDomainScope(t *testing.T) {
	descs := []CatalogPresentation{
		{Name: "auth_status", DirectVisible: true},
		{Name: "api_keys_list", DirectVisible: true},
		{Name: "vault_status", DirectVisible: true},
		{Name: "websites_get", DirectVisible: true},
		{Name: "pins_list", DirectVisible: true},
	}

	restricted := assembly.DomainScope{Account: true, Websites: true}
	hosted := flatToolNames(descs, restricted)
	require.Equal(t, []string{"auth_status", "api_keys_list", "websites_get"}, hosted)

	full := flatToolNames(descs, assembly.FullDomainScope)
	require.Equal(t, []string{"auth_status", "api_keys_list", "vault_status", "websites_get", "pins_list"}, full)
}
