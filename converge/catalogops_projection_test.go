package converge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner"
	"go.lumeweb.com/pinner/catalogops"
	"go.lumeweb.com/pinner/core/operations"
)

// realDomainOps constructs the REAL catalogops domain operation providers
// with degraded deps: handlers still fail with a clear "service unavailable"
// error at execution time (nil service), but registration and projection only
// need the declared metadata, which is exactly what the projection consumes.
// This is the library characterization sample: every operation the pinner
// domain ships, from its real provider functions.
func realDomainOps(t *testing.T) []pinner.Operation {
	t.Helper()
	ops := make([]pinner.Operation, 0, 128)
	nilOperationsService := func(map[string]any) operations.Service { return nil }
	ops = append(ops, catalogops.AuthOperations(catalogops.AuthDeps{})...)
	ops = append(ops, catalogops.AccountOperations(catalogops.AccountDeps{})...)
	ops = append(ops, catalogops.APIKeysOperations(catalogops.APIKeysDeps{})...)
	ops = append(ops, catalogops.VaultSetupOperations(catalogops.VaultDeps{})...)
	ops = append(ops, catalogops.VaultOperations(catalogops.VaultDeps{})...)
	ops = append(ops, catalogops.PinsOperations(catalogops.PinsDeps{})...)
	ops = append(ops, catalogops.WebsitesOperations(catalogops.WebsitesDeps{})...)
	ops = append(ops, catalogops.DNSOperations(catalogops.DNSDeps{})...)
	ops = append(ops, catalogops.IPNSOperations(catalogops.IPNSDeps{})...)
	ops = append(ops, catalogops.ENSOperations(catalogops.ENSDeps{})...)
	ops = append(ops, catalogops.OperationsOperations(catalogops.OperationsDeps{Service: nilOperationsService})...)
	ops = append(ops, catalogops.AdminOperations(catalogops.AdminDeps{})...)

	require.NotEmpty(t, ops)
	for _, op := range ops {
		require.NotNil(t, op)
	}
	return ops
}

// TestProjectRealCatalogopsProviders is the coverage characterization: every
// real catalogops operation from the 12 domain providers projects onto the
// opmesh model, registers into a real opmesh.Catalog, and is discoverable by
// the exact same stable ID with the same opmesh-owned metadata.
func TestProjectRealCatalogopsProviders(t *testing.T) {
	ops := realDomainOps(t)
	cat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(cat, ops...))

	for _, src := range ops {
		projected, ok := cat.Get(src.Name())
		require.True(t, ok, "projected op %s missing from opmesh catalog", src.Name())

		// Stable operation ID and display vocabulary.
		assert.Equal(t, src.Name(), projected.Name(), "operation ID must be identical")
		assert.Equal(t, src.Title(), projected.Title())
		assert.Equal(t, src.Summary(), projected.Summary())
		assert.Equal(t, src.Category(), projected.Category())
		assert.Equal(t, src.Positional(), projected.Positional())

		// Effect classification.
		assert.Equal(t, ProjectSafety(src.Safety()), projected.Safety())
		assert.Equal(t, ProjectInteraction(src.Interaction()), projected.Interaction())
		assert.Equal(t, ProjectVisibility(src.Visibility()), projected.Visibility())

		// Typed inputs: every declared arg survives with its opmesh-owned
		// fields; agent-only prose does not block any arg from projecting.
		srcArgs := src.Args()
		got := projected.Args()
		require.Len(t, got, len(srcArgs), "op %s arg count changed", src.Name())
		for i := range srcArgs {
			assert.Equal(t, srcArgs[i].Name, got[i].Name)
			assert.Equal(t, ProjectArgType(srcArgs[i].Type), got[i].Type)
			assert.Equal(t, srcArgs[i].Required, got[i].Required)
			assert.Equal(t, srcArgs[i].Default, got[i].Default)
			assert.Equal(t, srcArgs[i].SelectionGroup, got[i].SelectionGroup)
			assert.Equal(t, srcArgs[i].AgentConfirm, got[i].AgentConfirm)
			assert.Equal(t, srcArgs[i].Sensitive, got[i].Sensitive)
			assert.Equal(t, srcArgs[i].Help, got[i].Help)
		}
	}
}

// TestProjectRealCatalogopsSpotChecks pins specific real operations because
// their opmesh-owned shape matters downstream (destructive gates, selection
// groups, flexible IDs):
//
//   - pins_rm: destructive, cids/all selection group, all defaults false.
//     headless rollback contract must survive projection.
//     survive projection.
//   - auth_login: EnvLocalOnly FRONTEND metadata must stay in pinner (the
//     projection is not a filter; surface gating happens at assembly/adapters)
//     while its opmesh-owned fields still project.
func TestProjectRealCatalogopsSpotChecks(t *testing.T) {
	ops := realDomainOps(t)
	byID := make(map[string]pinner.Operation, len(ops))
	for _, op := range ops {
		byID[op.Name()] = op
	}

	// pins_rm
	pinsRm := byID["pins_rm"]
	require.NotNil(t, pinsRm)
	rm := ProjectSpec(pinsRm)
	require.Equal(t, opmesh.SafetyDestructive, rm.Safety)
	var cids, all *opmesh.OperationArg
	for i := range rm.Args {
		switch rm.Args[i].Name {
		case "cids":
			cids = &rm.Args[i]
		case "all":
			all = &rm.Args[i]
		}
	}
	require.NotNil(t, cids, "pins_rm must declare cids")
	require.NotNil(t, all, "pins_rm must declare all")
	assert.Equal(t, "remove", cids.SelectionGroup, "selection group must survive projection")
	assert.Equal(t, opmesh.ArgTypeStringSlice, cids.Type)
	assert.Equal(t, "remove", all.SelectionGroup)

	// vault_version_restore
	restore := ProjectSpec(byID["vault_version_restore"])
	require.Equal(t, opmesh.SafetyDestructive, restore.Safety)
	require.Contains(t, argNames(restore.Args), "confirm")
	for _, a := range restore.Args {
		if a.Name == "confirm" {
			assert.True(t, a.AgentConfirm, "headless confirm contract must survive projection")
			assert.Equal(t, opmesh.ArgTypeBool, a.Type)
		}
	}

	// ipns flexid ids
	found := false
	for _, name := range []string{"ipns_keys_get", "ipns_keys_delete", "ipns_publish"} {
		op, ok := byID[name]
		if !ok {
			continue
		}
		found = true
		spec := ProjectSpec(op)
		for _, a := range spec.Args {
			if a.Name == "id" {
				assert.Equal(t, opmesh.ArgTypeFlexibleID, a.Type, "flexible-id codec must survive projection on %s", name)
			}
		}
	}
	assert.True(t, found, "expected at least one ipns key op to exercise FlexibleID")

	// auth_login frontend metadata stays in pinner; opmesh-owned projects.
	login := byID["auth_login"]
	require.NotNil(t, login)
	require.Equal(t, pinner.EnvLocalOnly, login.Environment(),
		"fixture sanity: auth_login carries frontend Environment metadata")
	projectedLogin := ProjectSpec(login)
	require.Equal(t, "auth_login", projectedLogin.Name)
	require.NotEmpty(t, projectedLogin.Args)
	tokenArg, ok := opmeshArgNamed(projectedLogin.Args, "token")
	require.True(t, ok, "auth_login token arg must project")
	assert.True(t, tokenArg.Sensitive)

	// The Env-para frontend surface gate must NOT be applied by projection:
	// projected auth_login is fully present in an opmesh catalog registered
	// via RegisterAll (filtering stays the caller's job, e.g. pinnerops).
	opcat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(opcat, login))
	_, ok = opcat.Get("auth_login")
	assert.True(t, ok, "projection must not itself drop EnvLocalOnly ops")
}

// TestProjectRealHandlersInvocable pins that projected REAL operations keep
// their degraded-handler behavior: dispatching through opmesh with no deps
// wired yields the same handlers (they run and fail with the service-
// unavailable error, exactly as they do through pinner).
func TestProjectRealHandlersInvocable(t *testing.T) {
	ops := realDomainOps(t)
	opcat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(opcat, ops...))

	// operations_list is a read op; with nil deps its handler degrades to a
	// service-unavailable error rather than a panic.
	_, err := opcat.Invoke(context.Background(), "operations_list", map[string]any{}, opmesh.ActorHuman)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operations service unavailable",
		"projected handler must be the real catalogops handler, not a stub")
}

func argNames(args []opmesh.OperationArg) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, a.Name)
	}
	return out
}

func opmeshArgNamed(args []opmesh.OperationArg, name string) (opmesh.OperationArg, bool) {
	for _, a := range args {
		if a.Name == name {
			return a, true
		}
	}
	return opmesh.OperationArg{}, false
}
