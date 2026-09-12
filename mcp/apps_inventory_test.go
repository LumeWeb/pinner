package mcp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// The installed-app inventory is the guide's truth input for MCP Apps prose,
// so the assembly asserts it before anything reads it: duplicates fail
// loudly, and the registry-side assertion hook runs when the composition
// root supplies one. The launcher-vocabulary check (unknown names) rides in
// with the appswire table layer.

func TestValidateInstalledAppsRejectsDuplicate(t *testing.T) {
	err := validateInstalledApps([]string{"open_pin_list", "open_pin_list"})
	require.ErrorContains(t, err, "duplicated")
}

func TestValidateInstalledAppsAcceptsCleanInventory(t *testing.T) {
	require.NoError(t, validateInstalledApps(nil))
	require.NoError(t, validateInstalledApps([]string{"open_pin_list"}))
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
