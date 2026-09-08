package catalogmcp

import (
	"encoding/json"
	"fmt"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/catalogmeta"
)

// Compiler maps an opmesh.Catalog onto the MCP tool surface. T is the element
// type: the model-surface compiler produces opmesh.ToolDescriptor values with
// target-resolved descriptions.
type Compiler interface {
	Compile(cat opmesh.Catalog) ([]opmesh.ToolDescriptor, error)
}

// NewCompiler returns a model-surface Compiler that maps an opmesh.Catalog to
// []opmesh.ToolDescriptor for the *model* surface: it yields every
// model-visible operation (via Catalog.Search with VisibilityModel, matching
// the registry's own visibility boundary) and resolves each tool's
// description from the operation's MCP targets (TargetsOf), falling back to
// the operation's own Description when no target is declared.
func NewCompiler() Compiler { return NewCompilerForProfile(nil) }

// NewCompilerForProfile returns a model-surface Compiler that additionally
// resolves FallbackFunc targets (DescFunc resolvers) against profile when
// building the static descriptor. profile is opaque; the description DSL's
// resolvers adapt it via forgeProfileOf (see profile.go). Passing an unadapted
// non-nil profile is a reported adapter gap (ProfileAdapterGap), never a
// silently featureless description. A nil profile skips DescFunc resolution
// and falls back to the operation's own Description, matching the
// pre-migration pinner compiler semantics.
func NewCompilerForProfile(profile any) Compiler {
	return &mcpCompiler{profile: profile}
}

// mcpCompiler maps a catalog's model-visible operations into MCP tool
// descriptors. It mirrors the pre-migration root-pinner MCP compiler's
// contract: VisibilityModel surface, description resolved from the operation's
// fallback target (static Description, or DescFunc resolved against profile),
// op.Description() as the safety net, and the registry's authoritative shape
// (opmesh.ToolDescriptor) for everything else.
type mcpCompiler struct {
	profile any
}

// Compile converts the catalog's model-visible operations into
// []opmesh.ToolDescriptor. The base descriptor comes from the registry's own
// Describe (same visibility boundary, same schema builder), then the
// description is overridden with the MCP-boundary target resolution so the
// static surface carries agent-critical DSL-composed guidance.
func (m *mcpCompiler) Compile(cat opmesh.Catalog) ([]opmesh.ToolDescriptor, error) {
	if cat == nil {
		return nil, fmt.Errorf("catalogmcp: cannot compile a nil catalog")
	}
	ops := cat.Search("", "", opmesh.VisibilityModel)
	tools := make([]opmesh.ToolDescriptor, 0, len(ops))
	for _, op := range ops {
		desc, ok := cat.Describe(op.Name(), opmesh.ActorModel)
		if !ok {
			continue
		}
		desc.Description = fallbackDescription(op.Description(), TargetsOf(op.Name()), m.profile)
		applyAgentArgHelp(op, &desc)
		tools = append(tools, desc)
	}
	return tools, nil
}

// applyAgentArgHelp re-applies the frontend metadata's AgentHelp onto the
// descriptor's InputSchema property descriptions. opmesh builds every arg's
// property description from the generic a.Help alone (it deliberately owns no
// audience-specific fields), preserving AgentHelp on the operations would
// silently drop the agent-critical prose (e.g. vault_share_accept.share_url's
// "pass the URL through unchanged" guidance). Mirroring the pre-migration
// compiler's precedence: AgentHelp wins when declared; the plain Help already
// emitted by the registry stays as the fallback otherwise. RawSchema args are
// skipped — as before the migration, the author-supplied raw schema (and its
// own description) wins verbatim.
//
// The InputSchema is round-tripped as a generic JSON object: opmesh emits it
// as {"type":"object","properties":{argName:{...},"required":[...]}, so each
// property object is mutated in place under its arg name and the schema is
// re-marshaled back into the descriptor.
func applyAgentArgHelp(op opmesh.Operation, desc *opmesh.ToolDescriptor) {
	if desc == nil || len(desc.InputSchema) == 0 {
		return
	}
	var schema map[string]any
	if err := json.Unmarshal(desc.InputSchema, &schema); err != nil {
		return
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for _, arg := range op.Args() {
		// RawSchema args keep the author-provided property object verbatim,
		// including its description (pre-migration behavior).
		if len(arg.RawSchema) > 0 {
			continue
		}
		meta := catalogmeta.ArgFrontendForArg(op.Name(), arg.Name)
		if meta == nil || meta.AgentHelp == "" {
			continue
		}
		p, ok := props[arg.Name].(map[string]any)
		if !ok {
			continue
		}
		p["description"] = meta.AgentHelp
	}
	out, err := json.Marshal(schema)
	if err != nil {
		return
	}
	desc.InputSchema = out
}

// fallbackDescription returns the fallback target's Description from targets
// (the one with empty Require and Visible=true). If no fallback target
// exists, it returns cliDesc as a safety net so the MCP descriptor always
// has a non-empty description.
//
// A fallback target may be a DescFunc-only variant (empty Description, the
// resolver in DescFunc) built via FallbackFunc. DescFunc is resolved against
// profile — the startup/transport profile when compiling the static surface,
// nil elsewhere — so agent-critical guidance composed from discrete DSL
// segments is not dropped from the static/non-profile descriptor. When profile
// is nil (an unknown profile), the DescFunc fallback is skipped and cliDesc is
// returned, matching the pre-migration behavior.
func fallbackDescription(cliDesc string, targets []Target, profile any) string {
	for _, t := range targets {
		if len(t.Require) == 0 && t.Visible {
			if t.Description != "" {
				return t.Description
			}
			if t.DescFunc != nil && profile != nil {
				if s := t.DescFunc(profile); s != "" {
					return s
				}
			}
		}
	}
	return cliDesc
}
