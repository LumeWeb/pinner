package converge

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// recordHandler is a Handler implementation; both the pinner and opmesh
// models consume it through the identical Handler interface.
type recordHandler struct {
	lastInput map[string]any
	result    any
}

func (h *recordHandler) Execute(_ context.Context, input map[string]any) (any, error) {
	h.lastInput = input
	return h.result, nil
}

// fullFrontendArg sets EVERY field the projection carries AND every
// frontend-only field, so coverage/exclusion tests exercise both sides.
// Defaults and enums are chosen per type so the fixture operations remain
// VALID under both registries' registration-time metadata validation
// (opmesh rejects enum defaults outside the enum, enum-on-non-string, and
// unparseable typed defaults — exercising that the projection preserves
// metadata well enough to satisfy them).
func fullFrontendArg(name string, typ pinner.ArgType) pinner.OperationArg {
	arg := pinner.OperationArg{
		Name:           name,
		Type:           typ,
		Required:       true,
		Sensitive:      true,
		Help:           name + " human help",
		AgentHelp:      name + " AGENT prose (frontend-only: must NOT project)",
		AgentRequired:  true,
		AgentConfirm:   name == "confirm",
		AgentOnly:      true, // frontend-only
		SelectionGroup: "pick_one",
		Sources:        []string{"PINNER_" + name}, // frontend-only
		PositionalOnly: true,                       // frontend-only
		RawSchema:      json.RawMessage(`{"type":["string","integer"]}`),
	}
	switch typ {
	case pinner.ArgTypeNullableInt:
		arg.Default = "10"
	case pinner.ArgTypeFlexibleID:
		arg.Default = "42"
	case pinner.ArgTypeString:
		arg.Default = "Alpha"
		arg.Enum = []string{"Alpha", "beta"}
	default:
		arg.Default = "Alpha" // bool/string-slice-parsable plain default
	}
	return arg
}

// fullSpecOp exercises every opmesh-owned spec/arg field plus every
// frontend-only field (all non-zero) — the fixture that pins the exhaustive
// field mapping.
func fullSpecOp() pinner.Operation {
	return pinner.NewOperation(pinner.OperationSpec{
		Name:        "synth_full",
		Title:       "Full Coverage Op",
		Summary:     "full coverage summary",
		Description: "full coverage description",
		Category:    "synthetic",
		Positional:  "<zone> <record>",
		Safety:      pinner.SafetyDestructive,
		Interaction: pinner.InteractionNeedsHandoff,
		Visibility:  pinner.VisibilityBoth,
		// frontend-only fields: must not leak into the projection
		MCPTargets:  pinner.MCPTargets(pinner.Fallback("agent fallback"), pinner.TargetFor("specific", "feature")),
		Environment: pinner.EnvCLIOnly,
		Args: []pinner.OperationArg{
			fullFrontendArg("zone", pinner.ArgTypeString),
			fullFrontendArg("priority", pinner.ArgTypeNullableInt),
			fullFrontendArg("ids", pinner.ArgTypeFlexibleID),
			fullFrontendArg("weights", pinner.ArgTypeStringSlice),
			{Name: "confirm", Type: pinner.ArgTypeBool, Required: true, AgentConfirm: true, Help: "confirm destructive op"},
		},
		Handler: &recordHandler{},
	})
}

// ---------------------------------------------------------------------------
// Coverage: every opmesh-owned field is faithfully projected
// ---------------------------------------------------------------------------

// opmeshSpecFields are exactly the fields opmesh's OperationSpec declares.
// If opmesh changes its shape, this list and ProjectSpec must be updated
// together; the reflect assertions below then make any omission fail loudly.
var opmeshSpecFields = []string{
	"Name", "Title", "Summary", "Description", "Args", "Positional",
	"Safety", "Interaction", "Visibility", "Category", "Handler",
}

// opmeshArgFields are exactly the fields opmesh's OperationArg declares.
var opmeshArgFields = []string{
	"Name", "Type", "Required", "Default", "Enum", "Sensitive", "Help",
	"AgentRequired", "AgentConfirm", "SelectionGroup", "RawSchema",
}

func fieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		names = append(names, t.Field(i).Name)
	}
	return names
}

func structFieldNames(v any) []string {
	return fieldNames(reflect.TypeOf(v))
}

func TestProjectionCarriesEveryOpmeshField(t *testing.T) {
	src := fullSpecOp()
	got := ProjectSpec(src)

	// The opmesh model declares exactly the fields we claim to project.
	assert.ElementsMatch(t, opmeshSpecFields, structFieldNames(opmesh.OperationSpec{}),
		"opmesh.OperationSpec gained/lost fields; update the projection contract")
	assert.ElementsMatch(t, opmeshArgFields, structFieldNames(opmesh.OperationArg{}),
		"opmesh.OperationArg gained/lost fields; update the projection contract")

	assert.Equal(t, "synth_full", got.Name)
	assert.Equal(t, "Full Coverage Op", got.Title)
	assert.Equal(t, "full coverage summary", got.Summary)
	assert.Equal(t, "full coverage description", got.Description)
	assert.Equal(t, "synthetic", got.Category)
	assert.Equal(t, "<zone> <record>", got.Positional)
	assert.Equal(t, opmesh.SafetyDestructive, got.Safety)
	assert.Equal(t, opmesh.InteractionNeedsHandoff, got.Interaction)
	assert.Equal(t, opmesh.VisibilityBoth, got.Visibility)
	require.NotNil(t, got.Handler)

	// Arg fields: compare every opmesh-owned arg field across all args,
	// field-by-field. The fixture sets every carried field non-zero, so any
	// dropped or swapped field fails this comparison.
	srcArgs := src.Args()
	require.Len(t, got.Args, len(srcArgs))
	for i, want := range srcArgs {
		have := got.Args[i]
		for _, f := range opmeshArgFields {
			haveField := reflect.ValueOf(have).FieldByName(f)
			require.True(t, haveField.IsValid(), "opmesh arg field %s vanished", f)
			// Expectation first: the source side is converted through the
			// SAME converters under test (Type), and otherwise mirrors the
			// similarly named pinner field. Distinct enum types must be
			// compared converted, not raw.
			wantField := reflect.ValueOf(want).FieldByName(f)
			require.True(t, wantField.IsValid(), "opmesh arg field %s has no pinner counterpart", f)
			var expected any
			if f == "Type" {
				expected = ProjectArgType(want.Type)
			} else {
				expected = wantField.Interface()
			}
			assert.True(t, reflect.DeepEqual(haveField.Interface(), expected),
				"arg %d field %s: projected %#v != source %#v", i, f,
				haveField.Interface(), expected)
		}
	}
}

func TestProjectedOperationExposesSpec(t *testing.T) {
	src := fullSpecOp()
	got := Project(src)

	assert.Equal(t, "synth_full", got.Name())
	assert.Equal(t, "Full Coverage Op", got.Title())
	assert.Equal(t, "full coverage summary", got.Summary())
	assert.Equal(t, "full coverage description", got.Description())
	assert.Equal(t, opmesh.SafetyDestructive, got.Safety())
	assert.Equal(t, opmesh.InteractionNeedsHandoff, got.Interaction())
	assert.Equal(t, opmesh.VisibilityBoth, got.Visibility())
	assert.Equal(t, "synthetic", got.Category())
	assert.Equal(t, "<zone> <record>", got.Positional())
	require.NotNil(t, got.Handler())
	assert.Len(t, got.Args(), len(src.Args()))
}

func TestEnumConversionsExhaustive(t *testing.T) {
	for _, s := range []pinner.Safety{pinner.SafetyRead, pinner.SafetyMutate, pinner.SafetyDestructive} {
		assert.Equal(t, opmesh.Safety(s), ProjectSafety(s))
	}
	for _, i := range []pinner.Interaction{pinner.InteractionAgentSafe, pinner.InteractionHumanOnly, pinner.InteractionNeedsHandoff} {
		assert.Equal(t, opmesh.Interaction(i), ProjectInteraction(i))
	}
	for _, v := range []pinner.Visibility{pinner.VisibilityModel, pinner.VisibilityAppOnly, pinner.VisibilityBoth} {
		assert.Equal(t, opmesh.Visibility(v), ProjectVisibility(v))
	}
	for _, a := range []pinner.ArgType{
		pinner.ArgTypeString, pinner.ArgTypeBool, pinner.ArgTypeNullableBool,
		pinner.ArgTypeNullableInt, pinner.ArgTypeInt, pinner.ArgTypeFloat,
		pinner.ArgTypeDuration, pinner.ArgTypeStringSlice,
		pinner.ArgTypeFlexibleID, pinner.ArgTypeRawJSON,
	} {
		assert.Equal(t, opmesh.ArgType(a), ProjectArgType(a))
	}
}

// TestEnumOrdinalParity pins that the ordinal-fallback casts in the
// converters stay exact: pinner and opmesh declare their enums in identical
// iota order. The loop covers the whole ordinals range; either fork reordering
// or adding values out of sync makes this fail.
func TestEnumOrdinalParity(t *testing.T) {
	for i := 0; i <= int(pinner.ArgTypeRawJSON); i++ {
		assert.Equal(t, opmesh.ArgType(i), ProjectArgType(pinner.ArgType(i)),
			"ArgType ordinals diverged at %d", i)
	}
	assert.Equal(t, int(opmesh.SafetyDestructive), int(pinner.SafetyDestructive))
	assert.Equal(t, int(opmesh.InteractionNeedsHandoff), int(pinner.InteractionNeedsHandoff))
	assert.Equal(t, int(opmesh.VisibilityBoth), int(pinner.VisibilityBoth))

	// Unsynced/unknown values still project (totality) instead of panicking.
	assert.Equal(t, opmesh.Safety(42), ProjectSafety(pinner.Safety(42)))
}

// TestProjectionArgsAreDefensiveCopies pins that the projection never aliases
// mutable source state (Enum / RawSchema slices).
func TestProjectionArgsAreDefensiveCopies(t *testing.T) {
	src := fullSpecOp()
	got := ProjectSpec(src)

	got.Args[0].Enum[0] = "MUTATED"
	assert.Equal(t, "Alpha", src.Args()[0].Enum[0], "Enum slice aliases source")

	gotRaw := append(got.Args[0].RawSchema, 'X')
	assert.NotEqual(t, string(gotRaw), string(src.Args()[0].RawSchema),
		"RawSchema slice aliases source")
}

// ---------------------------------------------------------------------------
// Frontend-field exclusion (structural)
// ---------------------------------------------------------------------------

func TestFrontendFieldsCannotLeakIntoProjection(t *testing.T) {
	ProjectSpec(fullSpecOp()) // exercising the projection path before structural checks

	// Frontend-only pinner fields must have no seat in the opmesh model. The
	// exact field lists asserted in TestProjectionCarriesEveryOpmeshField
	// enforce this structurally; these explicit name checks keep the intent
	// readable and diff-able.
	haveSpec := structFieldNames(opmesh.OperationSpec{})
	for _, banned := range []string{"MCPTargets", "Environment"} {
		assert.NotContains(t, haveSpec, banned, "frontend field %s leaked into opmesh spec", banned)
	}
	haveArg := structFieldNames(opmesh.OperationArg{})
	for _, banned := range []string{"AgentHelp", "AgentOnly", "Sources", "PositionalOnly"} {
		assert.NotContains(t, haveArg, banned, "frontend field %s leaked into opmesh arg", banned)
	}

	// The frontend values WERE set on the source (so this genuinely covers
	// the leak), yet opmesh's Operation interface structurally cannot expose
	// them: it declares no MCPTargets/Environment methods.
	opType := reflect.TypeOf((*opmesh.Operation)(nil)).Elem()
	for _, banned := range []string{"MCPTargets", "Environment"} {
		m, ok := opType.MethodByName(banned)
		assert.False(t, ok, "opmesh.Operation must not expose %s (found %v)", banned, m)
	}

	// pinner keeps its frontend metadata untouched by the projection.
	src := fullSpecOp()
	assert.Equal(t, pinner.EnvCLIOnly, src.Environment())
	assert.NotEmpty(t, src.MCPTargets())
}

func TestFrontendProjectionCarriesGenericHelpOnly(t *testing.T) {
	// opmesh keeps a single generic arg description; the projection carries
	// pinner's Help verbatim and leaves the audience-split AgentHelp behind.
	src := fullSpecOp()
	got := ProjectSpec(src)
	assert.Equal(t, src.Args()[0].Help, got.Args[0].Help)
	assert.NotEqual(t, src.Args()[0].AgentHelp, got.Args[0].Help)
	assert.NotContains(t, got.Args[0].Help, "AGENT prose",
		"agent-oriented prose must not be smuggled into the generic Help")
}

func TestProjectionTotality(t *testing.T) {
	// nil operation: never panics.
	assert.Nil(t, Project(nil))
	assert.Equal(t, opmesh.OperationSpec{}, ProjectSpec(nil))
	assert.Empty(t, ProjectAll(nil))
	assert.Empty(t, ProjectAll([]pinner.Operation{nil}))

	// bare-bones operation still projects.
	zero := pinner.NewOperation(pinner.OperationSpec{Name: "synth_zero"})
	got := ProjectSpec(zero)
	assert.Equal(t, "synth_zero", got.Name)
	assert.Empty(t, got.Args)
	assert.Equal(t, opmesh.SafetyRead, got.Safety)
	assert.Nil(t, got.Handler)

	// nil/empty arg slices project to nil, not empty-non-nil.
	spec := pinner.OperationSpec{Name: "synth_noargs", Handler: &recordHandler{}}
	assert.Nil(t, ProjectSpec(pinner.NewOperation(spec)).Args)
	empty := pinner.NewOperation(pinner.OperationSpec{Name: "synth_emptyargs", Handler: &recordHandler{}})
	assert.Nil(t, ProjectSpec(empty).Args)
}

func TestProjectAllAndRegisterAll(t *testing.T) {
	ops := []pinner.Operation{fullSpecOp(), parityOp(), nil}
	projected := ProjectAll(ops)
	require.Len(t, projected, 2, "nil elements are dropped")
	assert.Equal(t, "synth_full", projected[0].Name())
	assert.Equal(t, "synth_parity", projected[1].Name())

	cat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(cat, ops...))
	got, ok := cat.Get("synth_full")
	require.True(t, ok)
	assert.Equal(t, "Full Coverage Op", got.Title())

	// Duplicate registration surfaces opmesh's own error (no swallowing).
	err := RegisterAll(cat, fullSpecOp())
	require.Error(t, err)
	assert.True(t, errorsIsDuplicate(err), "got %v", err)

	// Nil catalog is an error, not a panic.
	assert.ErrorIs(t, RegisterAll(nil, fullSpecOp()), ErrNilCatalog)
}
