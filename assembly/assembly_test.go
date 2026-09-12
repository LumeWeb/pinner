package assembly

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/pinner/catalogops"
)

// testBundle returns an all-domain bundle with zero-valued (unwired) deps for
// each catalogops domain. Unwired domains degrade to ops that fail at
// execution time, which is what the characterization relies on: assembly only
// REGISTERS operations, it never executes them.
func testBundle() *CatalogDepsBundle {
	return &CatalogDepsBundle{
		Auth:       catalogops.AuthDeps{},
		Account:    catalogops.AccountDeps{},
		Vault:      catalogops.VaultDeps{},
		VaultSetup: catalogops.VaultDeps{},
		Pins:       catalogops.PinsDeps{},
		Websites:   catalogops.WebsitesDeps{},
		DNS:        catalogops.DNSDeps{},
		IPNS:       catalogops.IPNSDeps{},
		ENS:        catalogops.ENSDeps{},
		APIKeys:    catalogops.APIKeysDeps{},
		Operations: catalogops.OperationsDeps{},
		Admin:      catalogops.AdminDeps{},
	}
}

// TestAssembleCatalogOpsHostedExcludesVault verifies that assembling the
// catalog for the hosted scope excludes the Sia vault domain entirely, while
// the account/IPFS/websites/DNS surfaces remain. This is the core "no Sia in
// hosted" guarantee.
func TestAssembleCatalogOpsHostedExcludesVault(t *testing.T) {
	bundle := testBundle()

	hosted, err := AssembleCatalogOps(bundle, HostedDomainScope, true)
	require.NoError(t, err, "hosted catalog must assemble")
	_, vaultHosted := hosted.Get("vault_status")
	assert.False(t, vaultHosted, "hosted scope must not register the Sia vault domain")

	full, err := AssembleCatalogOps(bundle, FullDomainScope, false)
	require.NoError(t, err, "full catalog must assemble")
	_, vaultFull := full.Get("vault_status")
	assert.True(t, vaultFull, "full scope must register the Sia vault domain")
}

// TestAssembleCatalogOpsHostedExcludesEnvLocal verifies that a hosted assembly
// drops the EnvLocalOnly operations (auth_login / auth_logout, which mutate
// shared local config and are moot in a stateless Portal-embedded server) and
// the EnvCLIOnly operations (account_update_email / account_update_password),
// while the full/local scope keeps them in the operation catalog (for the
// urfave CLI frontend and, for the local ops, the local MCP scope).
func TestAssembleCatalogOpsHostedExcludesEnvLocal(t *testing.T) {
	// Hosted is declared explicitly; it is not inferred from the presence of a
	// CredentialResolver (that only supplies the per-request token).
	bundle := &CatalogDepsBundle{
		Auth:    catalogops.AuthDeps{},
		Account: catalogops.AccountDeps{},
	}

	hosted, err := AssembleCatalogOps(bundle, HostedDomainScope, true)
	require.NoError(t, err, "hosted catalog must assemble")
	for _, name := range []string{"auth_login", "auth_logout", "account_update_email", "account_update_password"} {
		if _, ok := hosted.Get(name); ok {
			t.Errorf("hosted assembly must not register %q", name)
		}
	}
	if _, ok := hosted.Get("auth_status"); !ok {
		t.Errorf("hosted assembly must keep auth_status")
	}

	full, err := AssembleCatalogOps(bundle, FullDomainScope, false)
	require.NoError(t, err, "full catalog must assemble")
	// EnvLocalOnly + EnvCLIOnly ops remain in the full/local catalog so the
	// CLI and local MCP frontends can use them.
	for _, name := range []string{"auth_login", "auth_logout", "account_update_email", "account_update_password"} {
		if _, ok := full.Get(name); !ok {
			t.Errorf("full scope must keep %q in the operation catalog", name)
		}
	}
}

// TestAssembleCatalogOpsRestrictedLocalKeepsEnvLocal verifies that a scope
// restriction alone does NOT turn a local stdio server into hosted mode: a
// local server that disables Vault/Admin keeps the EnvLocalOnly ops
// (auth_login / auth_logout) because hosted is declared explicitly and remains
// false here. This is the regression guard for the design footgun where hosted
// was inferred from scope equality / CredentialResolver presence.
func TestAssembleCatalogOpsRestrictedLocalKeepsEnvLocal(t *testing.T) {
	bundle := &CatalogDepsBundle{
		Auth:    catalogops.AuthDeps{},
		Account: catalogops.AccountDeps{},
	}
	// Restricted but NOT hosted: Vault and Admin disabled. This scope field-
	// equals HostedDomainScope, but because hosted is passed explicitly as false,
	// it must stay local.
	restricted := FullDomainScope
	restricted.Vault = false
	restricted.Admin = false
	restrictedLocal, err := AssembleCatalogOps(bundle, restricted, false)
	require.NoError(t, err, "restricted local catalog must assemble")
	for _, name := range []string{"auth_login", "auth_logout"} {
		if _, ok := restrictedLocal.Get(name); !ok {
			t.Errorf("restricted local scope (field-equals HostedDomainScope, hosted=false) must keep %q", name)
		}
	}
}

// TestAssembleCatalogOpsHostedExplicitRegardlessOfSurface verifies that hosted
// mode is declared explicitly by the construction path, not by the scope: a
// full scope assembled as hosted still excludes EnvLocalOnly ops, and the
// hosted preset assembled as local keeps them. The hosted argument is the
// single source of truth.
func TestAssembleCatalogOpsHostedExplicitRegardlessOfSurface(t *testing.T) {
	bundle := &CatalogDepsBundle{
		Auth:    catalogops.AuthDeps{},
		Account: catalogops.AccountDeps{},
	}

	// FullDomainScope but hosted=true: still drops EnvLocalOnly/EnvCLIOnly.
	fullHosted, err := AssembleCatalogOps(bundle, FullDomainScope, true)
	require.NoError(t, err, "hosted full-scope catalog must assemble")
	for _, name := range []string{"auth_login", "auth_logout", "account_update_email", "account_update_password"} {
		if _, ok := fullHosted.Get(name); ok {
			t.Errorf("hosted=true must drop %q regardless of full scope", name)
		}
	}

	// HostedDomainScope hosted=true: drops EnvLocalOnly.
	hosted, err := AssembleCatalogOps(bundle, HostedDomainScope, true)
	require.NoError(t, err, "hosted-preset catalog must assemble")
	for _, name := range []string{"auth_login", "auth_logout"} {
		if _, ok := hosted.Get(name); ok {
			t.Errorf("hosted-preset assembly must not register %q", name)
		}
	}
	if _, ok := hosted.Get("auth_status"); !ok {
		t.Errorf("hosted-preset assembly must keep auth_status")
	}
}

// TestAssembleCatalogOpsHostedSurfaceKeepsDomains verifies the hosted preset
// still registers every non-admin/non-vault domain it is supposed to expose.
func TestAssembleCatalogOpsHostedSurfaceKeepsDomains(t *testing.T) {
	hosted, err := AssembleCatalogOps(testBundle(), HostedDomainScope, true)
	require.NoError(t, err, "hosted catalog must assemble")
	for _, name := range []string{"auth_status", "account_info", "account_subscription", "dns_zones_list", "ipns_keys_list"} {
		if _, ok := hosted.Get(name); !ok {
			t.Errorf("hosted scope must keep %q", name)
		}
	}
}

// TestAssembleCatalogOpsRejectsNilBundle pins that a nil deps bundle is a
// wiring bug rejected up-front, before any registration happens.
func TestAssembleCatalogOpsRejectsNilBundle(t *testing.T) {
	cat, err := AssembleCatalogOps(nil, FullDomainScope, false)
	assert.Error(t, err)
	assert.Nil(t, cat)
}

// TestAssembleCatalogOpsStampsHostedPlugin pins the deployment-context stamp
// the hosted-plugin subscription gate depends on: AssembleCatalogOps mirrors
// its hosted construction flag onto the account domain deps so account
// handlers can suppress subscription deep-links on hosted (plugin)
// assemblies; a local assembly keeps HostedPlugin false.
func TestAssembleCatalogOpsStampsHostedPlugin(t *testing.T) {
	bundle := testBundle()
	_, err := AssembleCatalogOps(bundle, HostedDomainScope, true)
	require.NoError(t, err, "hosted-preset catalog must assemble")
	require.True(t, bundle.Account.HostedPlugin,
		"a hosted assembly must stamp HostedPlugin=true onto the account deps")

	networkBundle := testBundle()
	_, err = AssembleCatalogOps(networkBundle, FullDomainScope, false)
	require.NoError(t, err, "local catalog must assemble")
	require.False(t, networkBundle.Account.HostedPlugin,
		"a local assembly must leave HostedPlugin false")
}

// TestAssembleCatalogOpsZeroSurfaceIsFull pins that the zero DomainScope (all
// fields unset) assembles the same set of domains as FullDomainScope.
func TestAssembleCatalogOpsZeroSurfaceIsFull(t *testing.T) {
	bundle := testBundle()

	zeroCat, err := AssembleCatalogOps(bundle, DomainScope{}, false)
	require.NoError(t, err)
	fullCat, err := AssembleCatalogOps(bundle, FullDomainScope, false)
	require.NoError(t, err)

	for _, name := range []string{"vault_status", "auth_status", "account_info"} {
		_, inZero := zeroCat.Get(name)
		_, inFull := fullCat.Get(name)
		assert.Equal(t, inFull, inZero, "zero and full scope must treat %q identically", name)
	}
}
