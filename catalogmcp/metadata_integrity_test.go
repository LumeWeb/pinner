package catalogmcp

// Metadata-integrity guards for the frontend metadata boundaries.
//
// Frontend metadata is keyed by stable operation ID in two boundary packages:
// catalogmeta (env carve-outs, per-arg agent metadata) and here (MCP
// targets / descriptions). A keyed table can silently drift from the core
// definitions — a renamed operation ID or arg name leaves orphaned metadata —
// so these tests pin the reverse direction: every keyed entry must resolve
// against a real operation declared by the real catalogops providers.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/catalogmeta"
	"go.lumeweb.com/pinner/catalogops"
	"go.lumeweb.com/pinner/core/operations"
)

// allDomainOps returns every operation the twelve catalogops domain providers
// declare, with degraded (nil-service) deps: registration and metadata
// inspection never execute the handlers.
func allDomainOps(t *testing.T) map[string]opmesh.Operation {
	t.Helper()
	nilOperationsService := func(map[string]any) operations.Service { return nil }
	domains := [][]opmesh.Operation{
		catalogops.AuthOperations(catalogops.AuthDeps{}),
		catalogops.AccountOperations(catalogops.AccountDeps{}),
		catalogops.APIKeysOperations(catalogops.APIKeysDeps{}),
		catalogops.VaultSetupOperations(catalogops.VaultDeps{}),
		catalogops.VaultOperations(catalogops.VaultDeps{}),
		catalogops.PinsOperations(catalogops.PinsDeps{}),
		catalogops.WebsitesOperations(catalogops.WebsitesDeps{}),
		catalogops.DNSOperations(catalogops.DNSDeps{}),
		catalogops.IPNSOperations(catalogops.IPNSDeps{}),
		catalogops.ENSOperations(catalogops.ENSDeps{}),
		catalogops.OperationsOperations(catalogops.OperationsDeps{Service: nilOperationsService}),
		catalogops.AdminOperations(catalogops.AdminDeps{}),
	}
	byID := make(map[string]opmesh.Operation, 192)
	for _, ops := range domains {
		require.NotEmpty(t, ops, "a catalogops domain provider must not return an empty set")
		for _, op := range ops {
			require.NotNil(t, op)
			require.NotContains(t, byID, op.Name(), "duplicate operation ID across domains")
			byID[op.Name()] = op
		}
	}
	return byID
}

// TestMCPTargetsResolveToRealOperations pins that every operation ID the MCP
// boundary holds targets for is a real catalogops operation ID, and that the
// declared target shape is well-formed (visible fallback present).
func TestMCPTargetsResolveToRealOperations(t *testing.T) {
	byID := allDomainOps(t)
	require.NotEmpty(t, opTargets, "the MCP boundary must carry the migrated target data")

	for id, targets := range opTargets {
		op, ok := byID[id]
		require.True(t, ok, "opTargets contains entry %q with no matching catalogops operation", id)
		require.NotEmpty(t, targets, "operations with a target entry must carry at least one target")
		hasFallback := false
		for _, tgt := range targets {
			if len(tgt.Require) == 0 && tgt.Visible {
				hasFallback = true
			}
		}
		require.True(t, hasFallback, "op %q must declare a visible, feature-independent fallback target", id)
		_ = op
	}
}

// TestTargetsOfIsTotalConservative pins the lookup contract: unknown IDs
// return nil (no MCP specialization), and the known contract for
// websites_create carries the DescFunc fallback that resolves the DSL.
func TestTargetsOfIsTotalConservative(t *testing.T) {
	if TargetsOf("definitely_not_an_operation") != nil {
		t.Fatal("unknown operation IDs must not yield targets")
	}
	targets := TargetsOf("websites_create")
	require.Len(t, targets, 1)
	require.NotNil(t, targets[0].DescFunc, "websites_create must resolve its description via DescFunc")
}

// TestArgFrontendResolvesToRealOperations pins that every (operation, arg)
// pair the catalogmeta boundary holds is declared by a real catalogops
// operation, so a renamed ID or arg can never strand metadata.
func TestArgFrontendResolvesToRealOperations(t *testing.T) {
	byID := allDomainOps(t)
	require.NotEmpty(t, catalogmeta.ArgFrontendIDs(), "the frontend boundary must carry the migrated arg metadata")

	for _, id := range catalogmeta.ArgFrontendIDs() {
		fes := catalogmeta.ArgFrontendFor(id)
		op, ok := byID[id]
		require.True(t, ok, "argFrontend contains entry %q with no matching catalogops operation", id)
		args := make(map[string]bool, len(op.Args()))
		for _, a := range op.Args() {
			args[a.Name] = true
		}
		for _, fe := range fes {
			require.True(t, args[fe.Name], "op %q argFrontend names arg %q which the operation does not declare", id, fe.Name)
		}
	}
}

// TestArgFrontendSpotChecks pins the semantics-carrying entries:
// the agent-only websites_create platform-claim args, the
// PositionalOnly zones/records args, and the single env-sourced namespace arg.
func TestArgFrontendSpotChecks(t *testing.T) {
	fe := catalogmeta.ArgFrontendForArg("websites_create", "platform")
	require.NotNil(t, fe)
	require.True(t, fe.AgentOnly, "websites_create platform arg must stay AgentOnly")

	fe = catalogmeta.ArgFrontendForArg("dns_records_create", "zone")
	require.NotNil(t, fe)
	require.True(t, fe.PositionalOnly, "dns_records_create zone arg must stay PositionalOnly")

	fe = catalogmeta.ArgFrontendForArg("websites_domains_add", "namespace")
	require.NotNil(t, fe)
	require.Equal(t, []string{"PINNER_DOMAIN_NAMESPACE"}, fe.Sources)
}

// TestEnvironmentCarveOuts pins the four surfaced carve-outs exactly as
// catalogmeta declares them; every other operation must stay EnvBoth.
func TestEnvironmentCarveOuts(t *testing.T) {
	for id, want := range map[string]catalogmeta.Environment{
		"account_update_email":    catalogmeta.EnvCLIOnly,
		"account_update_password": catalogmeta.EnvCLIOnly,
		"auth_login":              catalogmeta.EnvLocalOnly,
		"auth_logout":             catalogmeta.EnvLocalOnly,
	} {
		require.Equal(t, want, catalogmeta.EnvironmentOf(id), "carve-out drifted for %q", id)
	}
	require.Equal(t, catalogmeta.EnvBoth, catalogmeta.EnvironmentOf("auth_status"))
	require.Equal(t, catalogmeta.EnvBoth, catalogmeta.EnvironmentOf("definitely_not_an_operation"))
}
