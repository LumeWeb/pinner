package catalogmcp

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

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
// non-nil profile is a reported adapter gap (readable via the concrete
// compiler's AdapterGap), never a silently featureless description. A nil
// profile skips DescFunc resolution and falls back to the operation's own
// Description, with no profile-dependent behavior.
func NewCompilerForProfile(profile any) Compiler {
	return &mcpCompiler{profile: profile}
}

// activeCompiler holds the mcpCompiler whose Compile is currently running,
// installed by Compile for the duration of the call. DescFunc resolvers have
// a fixed signature (func(any) string) and cannot carry a compiler reference,
// so forgeProfileOf records adapter-gap diagnostics through this pointer.
// Compile resolves DescFunc synchronously in the same goroutine, so the
// active compiler is correctly scoped for the real use; the atomic pointer
// keeps concurrent compiles race-free rather than pollution-free in the
// diagnostic sense (each compiler's gap slot remains per-instance).
var activeCompiler atomic.Pointer[mcpCompiler]

// mcpCompiler maps a catalog's model-visible operations into MCP tool
// descriptors. It follows a fixed contract: VisibilityModel surface,
// description resolved from the operation's
// fallback target (static Description, or DescFunc resolved against profile),
// op.Description() as the safety net, and the registry's authoritative shape
// (opmesh.ToolDescriptor) for everything else.
//
// The adapter-gap diagnostic is per-instance (gap, guarded by gapMu): a gap
// raised while compiling with one profile/compiler cannot pollute or be
// cleared by another compiler instance (the old package-global slot could
// produce stale false alarms and false negatives across compiles).
type mcpCompiler struct {
	profile any
	gapMu   sync.Mutex
	gap     error
}

// AdapterGap returns and clears the adapter-gap diagnostic raised during
// this compiler's most recent Compile. Nil means no non-nil, non-carrier
// profile was adapted.
func (m *mcpCompiler) AdapterGap() error {
	m.gapMu.Lock()
	defer m.gapMu.Unlock()
	err := m.gap
	m.gap = nil
	return err
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
	// Scope this compiler as the adapter-gap recorder for the duration of the
	// synchronous DescFunc resolution below: reset this instance's gap, then
	// install it as the active compiler. Any gap forgeProfileOf records while
	// this Compile is running lands on THIS instance, never on another
	// compiler's slot.
	m.gapMu.Lock()
	m.gap = nil
	m.gapMu.Unlock()
	activeCompiler.Store(m)
	defer activeCompiler.Store(nil)
	ops := cat.Search("", "", opmesh.VisibilityModel)
	tools := make([]opmesh.ToolDescriptor, 0, len(ops))
	for _, op := range ops {
		desc, ok := cat.Describe(op.Name(), opmesh.ActorModel)
		if !ok {
			continue
		}
		desc.Description = fallbackDescription(op.Description(), TargetsOf(op.Name()), m.profile)
		if err := applyAgentArgHelp(op, &desc); err != nil {
			return nil, fmt.Errorf("catalogmcp: op %s: %w", op.Name(), err)
		}
		tools = append(tools, desc)
	}
	return tools, nil
}

// applyAgentArgHelp re-applies the frontend metadata's AgentHelp onto the
// descriptor's InputSchema property descriptions. opmesh builds every arg's
// property description from the generic a.Help alone (it deliberately owns no
// audience-specific fields), preserving AgentHelp on the operations would
// silently drop the agent-critical prose (e.g. vault_share_accept.share_url's
// "pass the URL through unchanged" guidance). The required precedence:
// AgentHelp wins when declared; the plain Help already
// emitted by the registry stays as the fallback otherwise. RawSchema args are
// skipped — the author-supplied raw schema (and its
// own description) wins verbatim.
//
// The InputSchema is round-tripped as a generic JSON object: opmesh emits it
// as {"type":"object","properties":{argName:{...},"required":[...]}, so each
// property object is mutated in place under its arg name and the schema is
// re-marshaled back into the descriptor.
//
// A malformed InputSchema is reported, not swallowed: the helper exists to
// restore agent-critical AgentHelp, so silently dropping it on a decode or
// encode failure would return tools that look complete but lack the
// guidance (see TestCompilerPropagatesMalformedInputSchema).
// applyAgentArgHelp also restores the agent-surface requiredness projection:
// the compiled "required" array must combine requiredArgNames
// PLUS every AgentRequired arg unconditionally (even when defaulted), while
// opmesh's inputSchemaFromArgs deliberately excludes AgentRequired (agent
// dispatch layers enforce it themselves). Without that, ops like pins_add
// (cids) and the confirmation ops (confirm) would be advertised in tools/list
// without their agent-mandatory args in "required". AgentRequired args that
// are already required, or that use a RawSchema (whose author schema wins
// verbatim), are left untouched.
func applyAgentArgHelp(op opmesh.Operation, desc *opmesh.ToolDescriptor) error {
	if desc == nil || len(desc.InputSchema) == 0 {
		return nil
	}
	var schema map[string]any
	if err := json.Unmarshal(desc.InputSchema, &schema); err != nil {
		return fmt.Errorf("decode InputSchema: %w", err)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	var agentRequired []string
	var cliOnly []string
	for _, arg := range op.Args() {
		// RawSchema args keep the author-provided property object verbatim,
		// including its description and own requiredness. A RawSchema arg is
		// never CLIOnly: no RawSchema arg carries the flag today, and an
		// author-supplied schema wins verbatim by contract.
		if len(arg.RawSchema) > 0 {
			continue
		}
		if arg.AgentRequired {
			agentRequired = append(agentRequired, arg.Name)
		}
		if meta := catalogmeta.ArgFrontendForArg(op.Name(), arg.Name); meta != nil && meta.CLIOnly {
			cliOnly = append(cliOnly, arg.Name)
		}
	}
	addRequired(schema, agentRequired)
	// CLIOnly args never appear on the agent/MCP channel: the value must not
	// be transmittable (e.g. private-key import), so the property is removed
	// from the compiled input schema (and defensively from "required").
	for _, name := range cliOnly {
		delete(props, name)
	}
	removeRequired(schema, cliOnly)
	for _, arg := range op.Args() {
		if len(arg.RawSchema) > 0 {
			continue // author-supplied raw schema (and its description) wins verbatim
		}
		meta := catalogmeta.ArgFrontendForArg(op.Name(), arg.Name)
		if meta == nil || meta.AgentHelp == "" || meta.CLIOnly {
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
		return fmt.Errorf("encode InputSchema: %w", err)
	}
	desc.InputSchema = out
	return nil
}

// addRequired merges the given arg names into the schema's top-level
// "required" array without dropping existing entries. Names already present
// are skipped; the merge is deterministic (declaration-order categories first,
// then the added names in catalog declaration order), so recompiling the same
// catalog yields byte-identical schema JSON.
func addRequired(schema map[string]any, names []string) {
	existing := map[string]bool{}
	required, _ := schema["required"].([]any)
	for _, r := range required {
		if s, ok := r.(string); ok {
			existing[s] = true
		}
	}
	for _, name := range names {
		if !existing[name] {
			required = append(required, name)
			existing[name] = true
		}
	}
	if len(required) == 0 {
		return
	}
	schema["required"] = required
}

// removeRequired filters the given arg names out of the schema's top-level
// "required" array (defensive complement of addRequired: an op whose arg is
// required at the core level but CLIOnly must not advertise it on the agent
// surface at all). Removes the key entirely when the array drains, keeping
// the schema minimal and deterministic.
func removeRequired(schema map[string]any, names []string) {
	if len(names) == 0 {
		return
	}
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	required, _ := schema["required"].([]any)
	kept := make([]any, 0, len(required))
	for _, r := range required {
		s, ok := r.(string)
		if ok && drop[s] {
			continue
		}
		kept = append(kept, r)
	}
	if len(kept) == 0 {
		delete(schema, "required")
		return
	}
	schema["required"] = kept
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
// returned, leaving the operation's own description in place.
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
