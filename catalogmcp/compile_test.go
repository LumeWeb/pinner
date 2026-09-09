package catalogmcp

// Regression tests for the mcpCompiler's InputSchema projection.
//
// opmesh builds each arg's JSON Schema property description from the generic
// a.Help alone (it deliberately owns no audience-specific fields). The
// compiler prefers AgentHelp for the property
// description when declared; the AgentHelp metadata lives keyed by
// operation ID + arg name in catalogmeta, so the compiler re-applies it on
// top of the registry-built descriptor. These tests pin that projection:
// AgentHelp overrides plain Help, args without AgentHelp keep their plain
// Help, and the tool-level description resolution is unaffected.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/catalogops"
)

// requiredArgs unmarshals a descriptor's InputSchema and returns its
// top-level "required" array (nil when absent).
func requiredArgs(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var parsed struct {
		Required []string `json:"required"`
	}
	require.NoError(t, json.Unmarshal(schema, &parsed))
	return parsed.Required
}

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
			// declared for pins_add, so the operation's own description is
			// the fallback.
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

// agentRequiredFixtureCatalog builds a catalog from the REAL catalogops pins
// and IPNS domain providers (nil deps: registration and compilation never run
// the handlers). pins_add exercises an AgentRequired arg without a declared
// Required flag (cids); ipns_keys_delete exercises an AgentRequired arg WITH a
// Default (confirm) plus a plain Required arg (id).
func agentRequiredFixtureCatalog(t *testing.T) opmesh.Catalog {
	t.Helper()
	cat := opmesh.NewCatalog()
	ops := append(
		catalogops.PinsOperations(catalogops.PinsDeps{}),
		catalogops.IPNSOperations(catalogops.IPNSDeps{})...,
	)
	for _, op := range ops {
		require.NoError(t, cat.Add(op))
	}
	return cat
}

// requiredOf returns the compiled descriptor for the named operation.
func requiredOf(t *testing.T, tools []opmesh.ToolDescriptor, id string) opmesh.ToolDescriptor {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == id {
			return tool
		}
	}
	t.Fatalf("compiled tools contain no descriptor for %q", id)
	return opmesh.ToolDescriptor{}
}

// TestCompilerIncludesAgentRequiredArgsInSchemaRequired pins the
// requiredness projection: the compiled MCP InputSchema's
// "required" array must include every AgentRequired arg, not just the
// Registry-Required args opmesh's schema builder emits. pins_add.cids and the
// confirm-gated destructive args are AgentRequired; opmesh deliberately
// excludes AgentRequired (agent dispatch layers enforce it
// themselves), so the compiler re-adds it. Args that are neither Required nor
// AgentRequired must stay optional.
func TestCompilerIncludesAgentRequiredArgsInSchemaRequired(t *testing.T) {
	cat := agentRequiredFixtureCatalog(t)

	tools, err := NewCompiler().Compile(cat)
	require.NoError(t, err)

	// pins_add: "cids" is AgentRequired-only (opmesh omits it); the optional
	// defaulted args must not be pulled into "required".
	pinsAdd := requiredOf(t, tools, "pins_add")
	reqs := requiredArgs(t, pinsAdd.InputSchema)
	require.Contains(t, reqs, "cids",
		"pins_add AgentRequired arg cids must appear in the compiled schema's required array")
	require.NotContains(t, reqs, "name",
		"optional args must remain out of required")
	require.NotContains(t, reqs, "wait",
		"defaulted non-required args must remain out of required")

	// ipns_keys_delete: the plain Required arg keeps its requiredness and the
	// AgentRequired-with-Default confirm arg gains it.
	ipnsDelete := requiredOf(t, tools, "ipns_keys_delete")
	reqs = requiredArgs(t, ipnsDelete.InputSchema)
	require.Contains(t, reqs, "id",
		"plain Required args must stay in required")
	require.Contains(t, reqs, "confirm",
		"AgentRequired arg with a Default (confirm) must appear in required")

	// AgentHelp property descriptions set by the same pass must survive:
	// pins_add.cids carries the catalogmeta AgentHelp.
	got, ok := propertyDescription(t, pinsAdd.InputSchema, "cids")
	require.True(t, ok, "cids property missing from InputSchema")
	require.Equal(t,
		"One or more concrete CIDs to pin. This field is required; supply the values here.",
		got,
		"requiredness restoration must not drop the AgentHelp description")
}
