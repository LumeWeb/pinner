package catalogmcp

// Regression tests for error propagation in Compile.
//
// applyAgentArgHelp round-trips the descriptor's InputSchema through a generic
// JSON object to re-apply the AgentHelp prose. A malformed InputSchema used to
// be silently swallowed, which returned tools that looked complete but had
// dropped their argument-level guidance. These tests pin that a malformed
// schema now fails compilation (carrying the operation name) instead.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
)

// malformedSchemaCatalog injects a deliberately unparseable InputSchema into
// the registry-built descriptor so the compiler's decode path is exercised.
type malformedSchemaCatalog struct {
	opmesh.Catalog
}

func (c malformedSchemaCatalog) Describe(name string, actor opmesh.Actor) (opmesh.ToolDescriptor, bool) {
	desc, ok := c.Catalog.Describe(name, actor)
	if !ok {
		return desc, false
	}
	desc.InputSchema = []byte("{not valid json")
	return desc, true
}

// TestCompilerPropagatesMalformedInputSchema pins that Compilation fails (and
// yields no tools) when a descriptor's InputSchema cannot be decoded, with the
// offending operation named in the error.
func TestCompilerPropagatesMalformedInputSchema(t *testing.T) {
	cat := malformedSchemaCatalog{agentHelpFixtureCatalog(t)}

	tools, err := NewCompiler().Compile(cat)
	require.Error(t, err)
	require.Nil(t, tools)
	require.Contains(t, err.Error(), "pins_add",
		"error must name the operation whose schema was malformed")
}

// TestApplyAgentArgHelpMalformedSchema pins the helper-level decode error.
func TestApplyAgentArgHelpMalformedSchema(t *testing.T) {
	op, ok := agentHelpFixtureCatalog(t).Get("pins_add")
	require.True(t, ok)

	desc := &opmesh.ToolDescriptor{InputSchema: json.RawMessage("nope")}
	err := applyAgentArgHelp(op, desc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode InputSchema")
}
