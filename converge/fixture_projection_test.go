package converge

// Fixture-based characterization for the projection seam.
//
// HISTORY/SCOPE (opmesh migration): in the pre-migration module, catalogops
// providers returned []pinner.Operation and this suite projected the REAL
// domain providers onto opmesh (it read operations straight from the
// catalogops provider functions). The opmesh convergence rewrote those
// providers to define their operations directly against the opmesh model, so
// the projection no longer sees them at all: zero catalogops content flows
// through this seam today, and pinnerops assembles opmesh catalogs natively.
// The spot-check invariants the real-provider suite pinned (stable IDs, arg
// codecs, selection groups, AgentConfirm contracts, handler invocability, no
// Environment filtering in the projection) are now pinned on two sides:
//
//   - on the opmesh-native side, by the catalogops tests executing their real
//     operations through opmesh.Catalog.Invoke (no projection involved), and
//   - here, by fixture operations built directly on the root pinner model,
//     which is the only remaining producer of pinner.Operation content: the
//     bridging surface the root package retains for consumers that still
//     assemble from it.
//
// The fixtures replicate the shapes the real-provider suite spot-checked so
// the projection contracts stay characterized even though the producers
// changed. No test was dropped: each original assertion has a dedicated
// successor below.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner"
	"go.lumeweb.com/pinner/catalogmeta"
)

// handlerFunc adapts an Execute-shaped func into a pinner.Handler.
type handlerFunc func(ctx context.Context, input map[string]any) (any, error)

func (f handlerFunc) Execute(ctx context.Context, input map[string]any) (any, error) {
	return f(ctx, input)
}

// degradedHandler returns a handler whose behavior matches the real catalogops
// pattern under nil deps at characterization time: it runs and reports a clear
// error instead of panicking.
func degradedHandler() pinner.Handler {
	return handlerFunc(func(_ context.Context, _ map[string]any) (any, error) {
		return nil, errors.New("service unavailable")
	})
}

// fixtureOps builds the pinner model fixture operations the projection
// characterization spot-checks, mirroring the real shapes that were pinned
// against the catalogops providers pre-migration:
//
//   - f_pins_rm: destructive, cids/all "remove" selection group.
//   - f_vault_version_restore: destructive with an AgentConfirm headless contract.
//   - f_auth_login: sensitive token arg + EnvLocalOnly frontend carve-out.
//   - f_ipns_keys_get: FlexibleID codec arg.
//   - f_unavailable: a read op whose degraded handler errors (invocability).
func fixtureOps(t *testing.T) []pinner.Operation {
	t.Helper()
	return []pinner.Operation{
		pinner.NewOperation(pinner.OperationSpec{
			Name:        "f_pins_rm",
			Title:       "Fixture pins op",
			Summary:     "Fixture destructive unpin",
			Description: "Fixture replicating the pins unpin shape.",
			Safety:      pinner.SafetyDestructive,
			Visibility:  pinner.VisibilityBoth,
			Args: []pinner.OperationArg{
				{Name: "cids", Type: pinner.ArgTypeStringSlice, SelectionGroup: "remove"},
				{Name: "all", Type: pinner.ArgTypeBool, Default: "false", SelectionGroup: "remove"},
			},
			Handler: degradedHandler(),
		}),
		pinner.NewOperation(pinner.OperationSpec{
			Name:        "f_vault_version_restore",
			Title:       "Fixture restore",
			Summary:     "Fixture destructive restore",
			Description: "Fixture replicating the vault restore shape.",
			Safety:      pinner.SafetyDestructive,
			Visibility:  pinner.VisibilityBoth,
			Args: []pinner.OperationArg{
				{Name: "path", Type: pinner.ArgTypeString, Required: true},
				{Name: "version_id", Type: pinner.ArgTypeString, Required: true},
				{Name: "confirm", Type: pinner.ArgTypeBool, Required: true, AgentConfirm: true},
			},
			Handler: degradedHandler(),
		}),
		pinner.NewOperation(pinner.OperationSpec{
			Name:        "f_auth_login",
			Title:       "Fixture login",
			Summary:     "Fixture token save",
			Description: "Fixture replicating the token-save shape with a sensitive arg.",
			Environment: pinner.EnvLocalOnly,
			Safety:      pinner.SafetyMutate,
			Visibility:  pinner.VisibilityBoth,
			Args: []pinner.OperationArg{
				{Name: "token", Type: pinner.ArgTypeString, Required: true, Sensitive: true},
			},
			Handler: degradedHandler(),
		}),
		pinner.NewOperation(pinner.OperationSpec{
			Name:        "f_ipns_keys_get",
			Title:       "Fixture key get",
			Summary:     "Fixture key lookup",
			Description: "Fixture replicating the FlexibleID codec shape.",
			Visibility:  pinner.VisibilityBoth,
			Args: []pinner.OperationArg{
				{Name: "id", Type: pinner.ArgTypeFlexibleID},
			},
			Handler: degradedHandler(),
		}),
		pinner.NewOperation(pinner.OperationSpec{
			Name:        "f_unavailable",
			Title:       "Fixture unavailable",
			Summary:     "Fixture degraded handler",
			Description: "Fixture replicating a real provider's degraded nil-deps handler.",
			Safety:      pinner.SafetyRead,
			Visibility:  pinner.VisibilityBoth,
			Handler:     degradedHandler(),
		}),
	}
}

// TestProjectFixtureProviders is the coverage characterization: every fixture
// operation built on the root pinner model projects onto the opmesh model,
// registers into a real opmesh.Catalog, and is discoverable by the exact same
// stable ID with the same opmesh-owned metadata. Successor of
// TestProjectRealCatalogopsProviders.
func TestProjectFixtureProviders(t *testing.T) {
	ops := fixtureOps(t)
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

// TestProjectFixtureSpotChecks pins specific fixture operations because their
// opmesh-owned shape matters downstream (destructive gates, selection groups,
// flexible IDs, sensitive args). Successor of TestProjectRealCatalogopsSpotChecks.
func TestProjectFixtureSpotChecks(t *testing.T) {
	ops := fixtureOps(t)
	byID := make(map[string]pinner.Operation, len(ops))
	for _, op := range ops {
		byID[op.Name()] = op
	}

	// f_pins_rm: destructive + cids/all selection group survives projection.
	fixtureSpec := ProjectSpec(byID["f_pins_rm"])
	require.Equal(t, opmesh.SafetyDestructive, fixtureSpec.Safety)
	var cids, all *opmesh.OperationArg
	for i := range fixtureSpec.Args {
		switch fixtureSpec.Args[i].Name {
		case "cids":
			cids = &fixtureSpec.Args[i]
		case "all":
			all = &fixtureSpec.Args[i]
		}
	}
	require.NotNil(t, cids, "fixture must declare cids")
	require.NotNil(t, all, "fixture must declare all")
	assert.Equal(t, "remove", cids.SelectionGroup, "selection group must survive projection")
	assert.Equal(t, opmesh.ArgTypeStringSlice, cids.Type)
	assert.Equal(t, "remove", all.SelectionGroup)
	assert.Equal(t, "false", all.Default)

	// f_vault_version_restore: headless confirm contract survives projection.
	restore := ProjectSpec(byID["f_vault_version_restore"])
	require.Equal(t, opmesh.SafetyDestructive, restore.Safety)
	require.Contains(t, argNames(restore.Args), "confirm")
	for _, a := range restore.Args {
		if a.Name == "confirm" {
			assert.True(t, a.AgentConfirm, "headless confirm contract must survive projection")
			assert.Equal(t, opmesh.ArgTypeBool, a.Type)
		}
	}

	// FlexibleID codec survives projection.
	flexSpec := ProjectSpec(byID["f_ipns_keys_get"])
	for _, a := range flexSpec.Args {
		if a.Name == "id" {
			assert.Equal(t, opmesh.ArgTypeFlexibleID, a.Type, "flexible-id codec must survive projection")
		}
	}

	// Frontend Environment metadata stays on the frontend-metadata boundary
	// (catalogmeta post-migration); the projection is not a filter.
	login := byID["f_auth_login"]
	require.NotNil(t, login)
	require.Equal(t, pinner.EnvLocalOnly, login.Environment(),
		"fixture sanity: the pinner-model login carries frontend Environment metadata")
	require.Equal(t, catalogmeta.EnvLocalOnly, catalogmeta.EnvironmentOf("auth_login"),
		"frontend sanity: the real auth-login carve-out still lives on the catalogmeta boundary")
	projectedLogin := ProjectSpec(login)
	require.Equal(t, "f_auth_login", projectedLogin.Name)
	require.NotEmpty(t, projectedLogin.Args)
	tokenArg, ok := opmeshArgNamed(projectedLogin.Args, "token")
	require.True(t, ok, "token arg must project")
	assert.True(t, tokenArg.Sensitive)

	// The Environment frontend gate must NOT be applied by projection: the
	// projected EnvLocalOnly op is fully present in an opmesh catalog
	// registered via RegisterAll (filtering stays the caller's job, e.g.
	// pinnerops).
	opcat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(opcat, login))
	_, ok = opcat.Get("f_auth_login")
	assert.True(t, ok, "projection must not itself drop EnvLocalOnly ops")
}

// TestProjectFixtureHandlersInvocable pins that projected operations keep
// their handler behavior: dispatching through opmesh returns the projected
// handler's error unchanged. Successor of TestProjectRealHandlersInvocable.
func TestProjectFixtureHandlersInvocable(t *testing.T) {
	ops := fixtureOps(t)
	opcat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(opcat, ops...))

	_, err := opcat.Invoke(context.Background(), "f_unavailable", map[string]any{}, opmesh.ActorHuman)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "service unavailable"),
		"projected handler must be the real fixture handler, not a stub")
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
