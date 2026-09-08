package catalogmcp

// Regression tests for the mcpCompiler's InputSchema projection.
//
// opmesh builds each arg's JSON Schema property description from the generic
// a.Help alone (it deliberately owns no audience-specific fields). The
// pre-migration root-pinner compiler preferred AgentHelp for the property
// description when declared; the AgentHelp metadata now lives keyed by
// operation ID + arg name in catalogmeta, so the compiler must re-apply it on
// top of the registry-built descriptor. These tests pin that projection:
// AgentHelp overrides plain Help, args without AgentHelp keep their plain
// Help, and the tool-level description resolution is unaffected.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
)

// agentHelpFixtureCatalog builds a one-operation catalog whose args exercise
// both sides of the description precedence: "cids" pins the real pins_add
// AgentHelp from catalogmeta, "all" is an arg with no frontend metadata and
// therefore must keep its plain Help.
func agentHelpFixtureCatalog(t *testing.T) opmesh.Catalog {
	t.Helper()
	cat := opmesh.NewCatalog()
	op := opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "pins_add",
		Description: "cli-level description",
		Args: []opmesh.OperationArg{
			{
				Name:     "cids",
				Type:     opmesh.ArgTypeString,
				Required: true,
				// Deliberately distinct from the catalogmeta AgentHelp so a
				// regression (Help leaking through or AgentHelp dropping)
				// is detectable either way.
				Help: "generic human help for cids",
			},
			{
				Name: "all",
				Type: opmesh.ArgTypeBool,
				Help: "generic human help for all",
			},
		},
	})
	require.NoError(t, cat.Add(op))
	return cat
}

// propertyDescription unmarshals a descriptor's InputSchema and returns the
// named property's description string.
func propertyDescription(t *testing.T, schema json.RawMessage, prop string) (string, bool) {
	t.Helper()
	var parsed struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(schema, &parsed))
	p, ok := parsed.Properties[prop]
	return p.Description, ok
}

// TestCompilerAppliesAgentHelpToArgSchemas pins that the compiled MCP
// tool descriptors carry the catalogmeta AgentHelp as the property
// description, with plain Help as the fallback for args that declare none.
func TestCompilerAppliesAgentHelpToArgSchemas(t *testing.T) {
	cat := agentHelpFixtureCatalog(t)

	for _, tc := range []struct {
		name     string
		compiler Compiler
	}{
		{"NewCompiler", NewCompiler()},
		{"NewCompilerForProfile-nil", NewCompilerForProfile(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools, err := tc.compiler.Compile(cat)
			require.NoError(t, err)
			require.Len(t, tools, 1)

			// Tool-level description is unchanged: no MCP targets are
			// declared for pins_add, so the CLI-level description is the
			// fallback.
			require.Equal(t, "cli-level description", tools[0].Description)

			// AgentHelp wins for the arg that declares it.
			got, ok := propertyDescription(t, tools[0].InputSchema, "cids")
			require.True(t, ok, "cids property missing from InputSchema")
			require.Equal(t,
				"One or more concrete CIDs to pin. This field is required; supply the values here.",
				got,
				"compiled schema description must use the catalogmeta AgentHelp, not the plain Help")

			// An arg without frontend metadata keeps its plain Help.
			got, ok = propertyDescription(t, tools[0].InputSchema, "all")
			require.True(t, ok, "all property missing from InputSchema")
			require.Equal(t, "generic human help for all", got)
		})
	}
}
