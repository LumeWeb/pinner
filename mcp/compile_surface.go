package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/pinner/catalogmcp"
	"go.lumeweb.com/pinner/catalogmeta"
)

// errNilCatalog is the assembly error for a Config that resolves to a nil
// operation catalog.
var errNilCatalog = fmt.Errorf("mcp: populateCatalogTools: nil operation catalog")

// catalogmcpCompile compiles the catalog for the model surface through the
// catalogmcp compiler for the given (already adapted) profile.
func catalogmcpCompile(profile any, cat opmesh.Catalog) ([]opmesh.ToolDescriptor, error) {
	return catalogmcp.NewCompilerForProfile(profile).Compile(cat)
}

// The catalog-surface projection: the bridge between the operation catalog
// (the compiler-backed source of truth for MCP tool descriptions/schemas,
// compiled via catalogmcp) and the presentation descriptors the composition
// root registers as the model-facing tool surface.
//
// The catalogmcp compiler yields opmesh.ToolDescriptor values whose
// Description/InputSchema come from the catalogops MCPTargets fallback and
// catalogmeta's AgentHelp re-application, so CLI help prose and global flag
// bags never leak into the model surface. This file projects them onto the
// mcpplane model.ToolDescriptor presentation shape: safety-derived wire hints,
// the per-classification output schema, direct stamping, and the
// environment carve-out skips.
//
// State lives on the assembled Server: the domain scope, hosted flag, and
// profile are Config fields threaded through Assemble, and the projection
// below is a
// pure function of its arguments. The dispatch/gate plumbing (catalog.Handler,
// CredentialFromContext, the needs_human result mapping) stays where it
// belongs — with the composition root; this package carries only declared,
// non-executable metadata (opmesh.ToolDescriptor carries no Handler by
// design, and dispatch must go through the owning Catalog.Invoke).

// CatalogPresentation is the presentation shape a compiled catalog operation
// projects onto. It is mcpplane's SDK-neutral tool descriptor: the same shape
// the direct tools (agent_guide, capabilities, transfer tools) use, so the
// whole assembled surface is homogeneous for the protocol seam.
type CatalogPresentation = model.ToolDescriptor

// readOnlyOverride records the platform-required annotation values for tools
// whose wire hints cannot be derived from the catalog Safety tier alone.
//
// auth_status is the only one so far: it can trigger out-of-band sign-in
// communication (the SSO hand-off emails the human a verification link, which
// cannot be unsent), so the Claude/MCP directory validators classify it as
// non-read, destructive and open-world — a sent message is irreversible. Its
// hints must declare that contract rather than the local "reads config only"
// shape.
var readOnlyOverride = map[string]struct {
	readOnly, destructive, openWorld bool
}{
	"auth_status": {readOnly: false, destructive: true, openWorld: true},
}

// catalogEnvelopeSchema is the typed shape of a catalog tool's *success*
// StructuredContent: an object whose `status` is always "ok" and whose optional
// `value` holds the op result. Only the success path emits this {status,value}
// envelope (the dispatch result-to-envelope mapping); error results carry no
// StructuredContent (they are signaled by the wire IsError flag) and
// needs_human results use a different shape entirely, so neither belongs here.
// The type is reflected with the project's invopop/jsonschema reflector,
// keeping schema generation consistent with every other tool schema and free
// of ad-hoc JSON strings.
type catalogEnvelopeSchema struct {
	Status string `json:"status" jsonschema:"required,description=Always ok on success"`
	Value  any    `json:"value,omitempty" jsonschema:"description=Operation result"`
}

// catalogEnvelopeReflector derives the envelope schema. AllowAdditionalProperties
// keeps the schema open so the dynamic per-operation `value` and any transport
// fields validate — unlike closed input schemas, the output envelope must
// accept the concrete op result the handler returns.
var catalogEnvelopeReflector = &jsonschema.Reflector{
	DoNotReference:            true,
	Anonymous:                 true,
	AllowAdditionalProperties: true,
}

// catalogOutputSchema is the JSON Schema describing the StructuredContent that
// a catalog-dispatched tool emits on success: the canonical {status:"ok", value}
// envelope. Error and needs_human responses are not described here — errors
// carry no structured content (IsError signals them), and needs_human uses
// catalogNeedsHumanOutputSchema — so the declared schema matches the shape the
// handler actually returns on the success path.
var catalogOutputSchema = func() json.RawMessage {
	b, err := json.Marshal(catalogEnvelopeReflector.Reflect(catalogEnvelopeSchema{}))
	if err != nil {
		// Only possible on an un-marshalable struct; the envelope is fully
		// serializable, so this cannot happen in practice.
		panic(err)
	}
	return b
}()

// catalogNeedsHumanSchema is the typed shape of a needs_human hand-off's
// StructuredContent: a {status:"needs_human", reason, ...} object. It is the
// declared output schema for tools whose dispatch path returns the needs_human
// hand-off shape (the vault_create / vault_restore setup swaps, confirm
// refusals) rather than the {status,value} success envelope. The URL key is
// tool-specific: SSO/account tools emit action_url, while the vault setup
// hand-offs emit create_url / restore_url — so the schema declares both sets,
// keeping every emitting tool's shape covered.
type catalogNeedsHumanSchema struct {
	Status     string `json:"status" jsonschema:"required,description=Always needs_human"`
	Reason     string `json:"reason" jsonschema:"required,description=Why human action is required"`
	ActionURL  string `json:"action_url,omitempty" jsonschema:"description=Short-lived URL the human opens (SSO and account hand-offs)"`
	CreateURL  string `json:"create_url,omitempty" jsonschema:"description=Out-of-band vault create URL (vault_create hand-off)"`
	RestoreURL string `json:"restore_url,omitempty" jsonschema:"description=Out-of-band vault restore URL (vault_restore hand-off)"`
	Handle     string `json:"handle,omitempty" jsonschema:"description=Async handle for a matching resume/status tool"`
	ResumeTool string `json:"resume_tool,omitempty" jsonschema:"description=Tool name to poll or resume with"`
	Detail     string `json:"detail,omitempty" jsonschema:"description=Optional human-readable context"`
}

// catalogNeedsHumanReflector derives the needs_human schema with the same
// open-additional-properties policy as the success envelope.
var catalogNeedsHumanReflector = &jsonschema.Reflector{
	DoNotReference:            true,
	Anonymous:                 true,
	AllowAdditionalProperties: true,
}

// catalogNeedsHumanOutputSchema is the JSON Schema describing a needs_human
// StructuredContent. It is emitted as the outputSchema for tools whose
// dispatch path returns the needs_human hand-off, so their declared schema
// matches what they actually return.
var catalogNeedsHumanOutputSchema = func() json.RawMessage {
	b, err := json.Marshal(catalogNeedsHumanReflector.Reflect(catalogNeedsHumanSchema{}))
	if err != nil {
		panic(err)
	}
	return b
}()

// catalogOutputUnionSchema is the JSON Schema a destructive or
// interactive-only compiled operation emits: an anyOf of the success envelope
// ({status:"ok", value}) and the needs_human hand-off shape. A destructive
// operation invoked by a model is first refused with the confirm gate
// (mapped to the needs_human hand-off by the dispatch layer), and only after
// human confirmation resumes does it run and return the {status:ok,value}
// success envelope — so its declared output schema must admit both shapes.
// The two members reuse the typed envelope and needs_human schemas above,
// keeping schema generation consistent and free of ad-hoc JSON. See
// outputSchemaForCompiled for per-classification selection.
//
// The root is explicitly object-typed: an outputSchema describes the
// StructuredContent of a tool result, which is always a JSON object, and the
// MCP tool schema contract requires an object-rooted output schema. Without the
// top-level type:object the root serializes to a bare anyOf object, which is
// neither a valid object schema nor accepted by 2025-era (15.x) model
// connectors that import tools/list. Every anyOf branch already requires
// type:object, so adding the root type does not change which values validate.
var catalogOutputUnionSchema = func() json.RawMessage {
	s := &jsonschema.Schema{
		Type: "object",
		AnyOf: []*jsonschema.Schema{
			catalogEnvelopeReflector.Reflect(catalogEnvelopeSchema{}),
			catalogNeedsHumanReflector.Reflect(catalogNeedsHumanSchema{}),
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return b
}()

// outputSchemaForCompiled selects the output schema for a compiled operation
// from its Safety/Interaction classification, so the declared shape matches
// what the operation actually emits for a model actor (the MCP surface runs as
// the model actor). The dispatch layer maps the catalog gate's refusals onto
// the needs_human hand-off shape, so the effective StructuredContent range is:
//
//   - InteractionHumanOnly / InteractionNeedsHandoff: the catalog Invoke gate
//     always refuses a model actor before a handler runs, so these tools
//     return only the needs_human hand-off on the model path.
//
//   - SafetyDestructive: the catalog Invoke gate refuses a model actor on
//     first invocation (manual-confirm hand-off), then — after human
//     confirmation resumes — the op runs and returns the {status:ok,value}
//     success envelope. Both shapes are emitted, so a union (anyOf) schema is
//     declared.
//
//   - Otherwise (SafetyRead / SafetyMutate, agent-safe): the op runs directly
//     and returns only the {status:ok,value} success envelope.
func outputSchemaForCompiled(safety opmesh.Safety, interaction opmesh.Interaction) json.RawMessage {
	switch {
	case interaction == opmesh.InteractionHumanOnly || interaction == opmesh.InteractionNeedsHandoff:
		return catalogNeedsHumanOutputSchema
	case safety == opmesh.SafetyDestructive:
		return catalogOutputUnionSchema
	default:
		return catalogOutputSchema
	}
}

// catalogDescriptorToPresentation converts a compiler-produced opmesh
// ToolDescriptor into the model-surface presentation descriptor. It maps the
// catalog Safety classification onto the presentation descriptor's
// ReadOnly/Destructive semantics so tool metadata is truthful for the model
// surface:
//
//	SafetyRead        -> ReadOnly=true
//	SafetyDestructive -> Destructive=true
//	SafetyMutate      -> neither
//
// The open-world hint derives from Safety: mutating/destructive operations
// change publicly visible internet state (pins, websites, DNS), while reads
// change nothing external, so openWorldHint stays false for any SafetyRead
// operation. The hints may be corrected per tool via readOnlyOverride where
// the platform contract demands it (see auth_status).
//
// DirectVisible is left to stampDirect (the direct product surface),
// matching how every other descriptor is promoted to tools/list.
func catalogDescriptorToPresentation(d opmesh.ToolDescriptor) CatalogPresentation {
	readOnly := d.Safety == opmesh.SafetyRead
	destructive := d.Safety == opmesh.SafetyDestructive
	openWorld := !readOnly
	if override, ok := readOnlyOverride[d.Name]; ok {
		readOnly = override.readOnly
		destructive = override.destructive
		openWorld = override.openWorld
	}
	return CatalogPresentation{
		Name:          d.Name,
		Title:         d.Title,
		Description:   d.Description,
		Category:      model.ToolCategory(d.Category),
		InputSchema:   d.InputSchema,
		OutputSchema:  outputSchemaForCompiled(d.Safety, d.Interaction),
		ReadOnly:      readOnly,
		Destructive:   destructive,
		OpenWorldHint: openWorld,
	}
}

// isModelVisibleOnMCP reports whether a compiled operation belongs on the MCP
// surface. EnvCLIOnly operations are valid only on the urfave CLI frontend:
// they pass credentials through the LLM channel and duplicate the OOB tools
// that hand off to a browser form. They remain available to the CLI frontend
// through the operation catalog; only the MCP surface omits them. This is
// declared on the operation (catalogmeta's environment carve-outs), not by a
// hard-coded name list here.
func isModelVisibleOnMCP(d opmesh.ToolDescriptor) bool {
	return catalogmeta.EnvironmentOf(d.Name) != catalogmeta.EnvCLIOnly
}

// populateCatalogTools compiles every model-visible operation from cat and
// projects it onto the presentation surface. It returns the set of compiled
// operation names so a composition root can route those invocations through
// the owning Catalog.Invoke gate (Interaction, Visibility, Safety, and
// required-arg enforcement hold there). tools/list prominence is decided by
// stampDirect.
//
// profile must already be an adapted catalogmcp-compatible shape (Assemble
// adapts Config.Profile exactly once via HostProfileOf/AdaptHostProfile and
// passes the same HostProfile here and to the direct presentation) so the
// DescFunc-only fallback targets resolve against it: a catalogmcp compiler
// built with a nil profile would collapse the DSL-composed descriptions (e.g.
// websites_create's feature-gated guidance) to the short CLI description.
func populateCatalogTools(cat opmesh.Catalog, profile any) ([]CatalogPresentation, error) {
	if cat == nil {
		return nil, errNilCatalog
	}

	descs, err := catalogmcpCompile(profile, cat)
	if err != nil {
		return nil, err
	}
	out := make([]CatalogPresentation, 0, len(descs))
	for _, d := range descs {
		if d.Name == "" {
			continue
		}
		// Environment carve-outs are declared on the operation, not by a
		// hard-coded name list here; see isModelVisibleOnMCP.
		if !isModelVisibleOnMCP(d) {
			continue
		}
		out = append(out, catalogDescriptorToPresentation(d))
	}
	return out, nil
}
