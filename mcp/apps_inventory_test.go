package mcp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/pinner/mcp/appswire"
)

// The installed-app inventory is the guide's truth input for MCP Apps prose,
// so the assembly asserts it before anything reads it: unknown launcher
// names and duplicates fail loudly, and the registry-side assertion hook
// runs when the composition root supplies one. With the appswire table in
// this layer, the vocabulary check is table-backed.

func TestValidateInstalledAppsRejectsDuplicate(t *testing.T) {
	err := validateInstalledApps([]string{"open_pin_list", "open_pin_list"})
	require.ErrorContains(t, err, "duplicated")
}

func TestValidateInstalledAppsAcceptsCleanInventory(t *testing.T) {
	require.NoError(t, validateInstalledApps(nil))
	require.NoError(t, validateInstalledApps([]string{appswire.LauncherPinList}))
}

func TestValidateInstalledAppsRejectsUnknownLauncher(t *testing.T) {
	err := validateInstalledApps([]string{appswire.LauncherPinList, "open_totally_made_up"})
	require.ErrorContains(t, err, `not a declared app-view launcher`)
	require.ErrorContains(t, err, "open_totally_made_up")
}

// TestAssembleVerifyInstalledAppsHook pins the registry-side assertion seam:
// a verification error surfaces as an assembly error, and a passing verifier
// lets the assembly proceed.
func TestAssembleVerifyInstalledAppsHook(t *testing.T) {
	verifyErr := errors.New("registry disagrees")

	_, err := Assemble(Config{
		Catalog:       testCatalog(),
		InstalledApps: []string{"open_pin_list"},
		VerifyInstalledApps: func([]string) error {
			return verifyErr
		},
	})
	require.ErrorIs(t, err, verifyErr,
		"a failing registry-side assertion must fail the assembly")

	srv, err := Assemble(Config{
		Catalog:       testCatalog(),
		InstalledApps: []string{"open_pin_list"},
		VerifyInstalledApps: func([]string) error {
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, srv)
}
