package pinnermcp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/pinner/pinnerops"
)

// Characterization of the curated tool-name sets, pinned against
// pinner-cli/internal/mcp/curated.go: the full surface exposes the seven
// hand-picked names; a hosted-shaped surface (account+websites, no vault)
// drops exactly the vault lifecycle/share entries.
func TestCuratedToolNamesFullSurface(t *testing.T) {
	require.Equal(t, []string{
		"auth_status",
		"vault_create",
		"vault_restore",
		"vault_status",
		"vault_share_accept",
		"websites_create",
		"websites_get",
	}, CuratedToolNames(pinnerops.FullSurface))
}

func TestCuratedToolNamesHostedSurface(t *testing.T) {
	require.Equal(t, []string{"auth_status", "websites_create", "websites_get"},
		CuratedToolNames(pinnerops.HostedSurface))
}

func TestCuratedToolNamesZeroSurfaceIsFull(t *testing.T) {
	// The zero Surface is the implicit FULL surface; the curated set must
	// treat it identically to FullSurface.
	require.Equal(t, CuratedToolNames(pinnerops.Surface{}), CuratedToolNames(pinnerops.FullSurface))
}

func TestCuratedToolNamesVaultSurfaceKeepsVaultEntries(t *testing.T) {
	// An account+vault surface with websites disabled cannot match the hosted
	// branch (needs account+vault-less+websites), so it keeps the full set.
	s := pinnerops.Surface{Account: true, Vault: true}
	require.Equal(t, compiledCuratedToolNames, CuratedToolNames(s))
}

// stampCurated pins DirectVisible exactly on the names in the curated set for
// the surface: names absent from the descriptor set (e.g. vault tools on a
// hosted surface) are ignored, present ones are stamped.
func TestStampCuratedMarksOnlyCuratedNames(t *testing.T) {
	descs := []CatalogPresentation{
		{Name: "auth_status"},
		{Name: "vault_create"},
		{Name: "websites_create"},
		{Name: "pins_list"},
	}
	stampCurated(CuratedToolNames(pinnerops.HostedSurface), descs)

	require.True(t, descs[0].DirectVisible, "auth_status must be direct-visible")
	require.False(t, descs[1].DirectVisible, "vault_create is not curated on the hosted surface")
	require.True(t, descs[2].DirectVisible, "websites_create must be direct-visible")
	require.False(t, descs[3].DirectVisible, "non-curated ops stay progressive-discovery only")
}
