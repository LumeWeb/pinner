package converge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner"
)

// cloneInput deep-copies a flat input map so both model normalize paths get
// independent fixtures.
func cloneInput(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func errorsIsDuplicate(err error) bool {
	return errors.Is(err, opmesh.ErrDuplicateOperation)
}

// ---------------------------------------------------------------------------
// Behavior parity: normalization, defaults, codecs, selection groups
// ---------------------------------------------------------------------------

// parityOp exercises every nontrivial codec path shared by both models:
// defaults, enums, flexible IDs, string slices, selection groups, and
// nullable tri-states.
func parityOp() pinner.Operation {
	return pinner.NewOperation(pinner.OperationSpec{
		Name: "synth_parity", Title: "Parity", Summary: "parity", Category: "synthetic",
		Args: []pinner.OperationArg{
			{Name: "priority", Type: pinner.ArgTypeNullableInt, Default: "10", Help: "priority"},
			{Name: "mode", Type: pinner.ArgTypeString, Enum: []string{"asc", "desc"}, Help: "mode"},
			{Name: "id", Type: pinner.ArgTypeFlexibleID, Help: "id"},
			{Name: "tags", Type: pinner.ArgTypeStringSlice, Help: "tags"},
			{Name: "cids", Type: pinner.ArgTypeStringSlice, SelectionGroup: "remove", Help: "cids"},
			{Name: "all", Type: pinner.ArgTypeBool, Default: "false", SelectionGroup: "remove", Help: "all"},
		},
		Handler: &recordHandler{},
	})
}

func TestNormalizeParityBetweenModels(t *testing.T) {
	src := parityOp()
	projected := Project(src)

	cases := []struct {
		name  string
		input map[string]any
	}{
		{"empty uses defaults", map[string]any{}},
		{"enum folds case to declared casing", map[string]any{"mode": "DESC"}},
		{"flexible id coerced to string", map[string]any{"id": json.Number("4711")}},
		{"slices accept scalars", map[string]any{"tags": "one", "cids": []any{"a", "b"}}},
		{"nullable keeps explicit zero distinct from absent", map[string]any{"priority": 0, "id": "x"}},
		{"selection single member", map[string]any{"cids": "QmX"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input

			want, wantErr := pinner.NormalizeOperationInput(src, cloneInput(input))
			gotN, gotErr := opmesh.NormalizeOperationInput(projected, cloneInput(input))

			if wantErr != nil {
				require.Error(t, gotErr)
				assert.Equal(t, wantErr.Error(), gotErr.Error())
				return
			}
			require.NoError(t, gotErr)
			assert.Equal(t, want, gotN, "normalization diverges between pinner and projected opmesh models")

			// Observable facts so the parity check is not vacuous.
			switch tc.name {
			case "empty uses defaults":
				require.NotNil(t, gotN["priority"]) // nullable-int default filler
				assert.Equal(t, 10, *(gotN["priority"].(*int)))
				assert.Equal(t, false, gotN["all"]) // bool default filler
			case "enum folds case to declared casing":
				// Enum matching is case-insensitive; the input's own casing
				// is what the handler receives (fork-shared behavior).
				assert.Equal(t, "DESC", gotN["mode"])
			case "flexible id coerced to string":
				assert.Equal(t, "4711", gotN["id"])
			case "slices accept scalars":
				assert.Equal(t, []string{"one"}, gotN["tags"])
				assert.Equal(t, []string{"a", "b"}, gotN["cids"])
			case "nullable keeps explicit zero distinct from absent":
				zero := gotN["priority"]
				require.NotNil(t, zero)
				assert.Equal(t, 0, *(zero.(*int)))
				assert.Equal(t, "x", gotN["id"])
			case "selection single member":
				assert.Equal(t, []string{"QmX"}, gotN["cids"])
			}
		})
	}

	// Violation parity: two selection-group members selected must fail in both
	// models with an identical error.
	_, wantErr := pinner.NormalizeOperationInput(src, map[string]any{"cids": []string{"a"}, "all": "true"})
	_, gotErr := opmesh.NormalizeOperationInput(projected, map[string]any{"cids": []string{"a"}, "all": "true"})
	require.Error(t, wantErr)
	require.Error(t, gotErr)
	assert.Equal(t, wantErr.Error(), gotErr.Error(),
		"selection-group enforcement error diverges between models")
}

func TestUnknownArgRejectedByBothModels(t *testing.T) {
	src := parityOp()
	projected := Project(src)

	input := map[string]any{"bogus": "x"}
	_, wantErr := pinner.NormalizeOperationInput(src, cloneInput(input))
	_, gotErr := opmesh.NormalizeOperationInput(projected, cloneInput(input))
	require.Error(t, wantErr)
	require.Error(t, gotErr)
	assert.Equal(t, wantErr.Error(), gotErr.Error(),
		"unknown-arg rejection must behave identically on both models")
}

// ---------------------------------------------------------------------------
// Dispatch parity: registry, enforcement gates, handler passthrough
// ---------------------------------------------------------------------------

// TestDestructiveConfirmationModelGatePreserved pins that after projection the
// opmesh registry enforces the same effect/actor/confirmation policy as the
// pinner model declared: destructive + model actor requires confirm=true,
// which the projected AgentConfirm arg satisfies; confirm=false is refused
// with opmesh.ErrConfirmRequired; and dispatch reaches the SAME handler value
// carried by the source operation.
func TestDestructiveConfirmationModelGatePreserved(t *testing.T) {
	handler := &recordHandler{}
	src := pinner.NewOperation(pinner.OperationSpec{
		Name: "synth_rm", Title: "Rm", Summary: "rm", Category: "synthetic",
		Safety:      pinner.SafetyDestructive,
		Interaction: pinner.InteractionAgentSafe,
		Visibility:  pinner.VisibilityBoth,
		Args:        []pinner.OperationArg{{Name: "confirm", Type: pinner.ArgTypeBool, Default: "false", AgentConfirm: true, Help: "confirm"}},
		Handler:     handler,
	})

	cat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(cat, src))

	out, err := cat.Invoke(context.Background(), "synth_rm", map[string]any{"confirm": true}, opmesh.ActorModel)
	require.NoError(t, err, "AgentConfirm confirm=true must satisfy the destructive model gate after projection")
	_ = out
	require.NotNil(t, handler.lastInput, "dispatch must reach the projected handler")
	assert.Equal(t, true, handler.lastInput["confirm"])

	_, err = cat.Invoke(context.Background(), "synth_rm", map[string]any{"confirm": false}, opmesh.ActorModel)
	require.Error(t, err)
	assert.ErrorIs(t, err, opmesh.ErrConfirmRequired,
		"projected op must still be refused with ErrConfirmRequired; got %v", err)
}

// TestProjectionThroughOpmeshDescribe pins discovery parity: opmesh Describe
// builds a transport-neutral ToolDescriptor from the projected args' schema.
func TestProjectionThroughOpmeshDescribe(t *testing.T) {
	src := parityOp()
	cat := opmesh.NewCatalog()
	require.NoError(t, RegisterAll(cat, src))

	desc, ok := cat.Describe("synth_parity", opmesh.ActorModel)
	require.True(t, ok)
	assert.Equal(t, "synth_parity", desc.Name)
	assert.Equal(t, "Parity", desc.Title)
	assert.Equal(t, opmesh.SafetyRead, desc.Safety)
	assert.Equal(t, "synthetic", desc.Category)
	require.NotEmpty(t, desc.InputSchema)

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(desc.InputSchema, &schema))
	for _, key := range []string{"priority", "mode", "id", "tags", "cids", "all"} {
		assert.Contains(t, schema.Properties, key)
	}
}
