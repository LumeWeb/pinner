// Package converge projects go.lumeweb.com/pinner's operation model onto the
// operation model of go.lumeweb.com/opmesh.
//
// The root pinner package and go.lumeweb.com/opmesh declare parallel
// operation vocabularies: the pinner model carries frontend metadata
// (Environment, MCPTargets, AgentHelp, AgentOnly, PositionalOnly, arg
// Sources) that opmesh deliberately omits, treating CLI/MCP presentation as
// the concern of the consumers of the operation model. Instead of
// maintaining a second fork of the operation vocabulary forever, pinner
// converges onto opmesh: this package provides a total, panic-free
// projection of pinner.Operation content onto the opmesh model so the
// module's operations can be expressed (registered, discovered, dispatched,
// normalized) against opmesh's Catalog without changing any existing
// behavior.
//
// # What is projected
//
// Only the opmesh-owned vocabulary is carried over — that is, exactly the
// fields opmesh's OperationSpec/OperationArg declare:
//
//   - Operation identity: Name, Title, Summary, Description, Category,
//     Positional.
//   - Inputs: every OperationArg's Name, Type, Required, Default, Enum,
//     Sensitive, Help, AgentRequired, AgentConfirm, SelectionGroup, and
//     RawSchema (Enum and RawSchema are defensively copied).
//   - Effect classification: Safety (read / mutate / destructive),
//     Interaction (agent-safe / human-only / needs-handoff), Visibility
//     (model / app-only / both).
//   - Execution: the Handler itself (both modules share the identical
//     Handler.Execute(ctx, input) contract, so the same handler value serves
//     both models unchanged).
//
// Because pinner and opmesh enums are iota-ordered in lockstep (a parity
// contract asserted by the tests in this package), the converters map via
// exhaustive switches with an ordinal-parity fallback so the projection stays
// total and never panics for values added to both forks in sync.
//
// # What is intentionally NOT projected
//
// The frontend-only metadata stays behind in the pinner package, where the
// presentation consumers of that model read it:
//
//   - pinner.Environment (EnvBoth/EnvCLIOnly/EnvLocalOnly/EnvHostedOnly) —
//     surface gating; callers apply it themselves, e.g. before projecting.
//   - pinner.MCPTargets / pinner.Target (with its DescFunc) and the
//     TargetFor/Fallback/Hidden/FallbackFunc helpers — MCP per-profile
//     presentation, an adapter concern.
//   - pinner.OperationArg.AgentHelp — audience-specific arg prose; opmesh
//     carries a single generic Help and lets adapters compose their own.
//   - pinner.OperationArg.AgentOnly, PositionalOnly, and Sources — CLI/MCP
//     flag-emission and env-sourcing presentation.
//
// The projection is deliberately NOT a normalization or validation step: it
// mirrors declared metadata only. It does not filter by Environment (the
// caller filters before projecting), does not validate
// operations (opmesh.Catalog.Add rejects genuinely malformed ones with
// ErrDuplicateOperation/arg errors), and does not close over handler state.
//
// All functions are nil-safe and total: they never panic on partial or
// degenerate input.
package converge

import "go.lumeweb.com/pinner"
import "go.lumeweb.com/opmesh"

// ProjectSpec maps the opmesh-owned metadata of a pinner.Operation onto an
// opmesh.OperationSpec. The projection is total: a nil operation yields the
// zero spec, and any pinner.Operation implementation (including bespoke ones,
// not just simpleOperation) is projected via its interface methods alone.
// Frontend-only fields (Environment, MCPTargets, AgentHelp, AgentOnly,
// PositionalOnly, Sources) are deliberately not represented — see the package
// documentation for the full not-projected list.
func ProjectSpec(op pinner.Operation) opmesh.OperationSpec {
	if op == nil {
		return opmesh.OperationSpec{}
	}
	return opmesh.OperationSpec{
		Name:        op.Name(),
		Title:       op.Title(),
		Summary:     op.Summary(),
		Description: op.Description(),
		Args:        ProjectArgs(op.Args()),
		Positional:  op.Positional(),
		Safety:      ProjectSafety(op.Safety()),
		Interaction: ProjectInteraction(op.Interaction()),
		Visibility:  ProjectVisibility(op.Visibility()),
		Category:    op.Category(),
		Handler:     op.Handler(),
	}
}

// Project wraps ProjectSpec's result in a concrete opmesh.Operation. The
// returned value shares the source operation's Handler value (no wrap of
// Execute) and copies the projected arg metadata defensively. A nil
// operation projects to a nil opmesh.Operation.
func Project(op pinner.Operation) opmesh.Operation {
	if op == nil {
		return nil
	}
	return opmesh.NewOperation(ProjectSpec(op))
}

// ProjectAll projects every operation, dropping nils. It never panics.
// Like RegisterAll, it does not filter by Environment; see RegisterAll.
func ProjectAll(ops []pinner.Operation) []opmesh.Operation {
	out := make([]opmesh.Operation, 0, len(ops))
	for _, op := range ops {
		if op == nil {
			continue
		}
		out = append(out, Project(op))
	}
	return out
}

// ProjectArgs projects pinner operation args onto opmesh.OperationArg,
// carrying every opmesh-owned arg field and leaving the frontend-only ones
// (AgentHelp, AgentOnly, Sources, PositionalOnly) in the pinner model. Enum
// and RawSchema values are copied so the projection never
// aliases mutable source state. A nil/empty slice projects to nil.
func ProjectArgs(args []pinner.OperationArg) []opmesh.OperationArg {
	if len(args) == 0 {
		return nil
	}
	out := make([]opmesh.OperationArg, 0, len(args))
	for _, a := range args {
		out = append(out, opmesh.OperationArg{
			Name:           a.Name,
			Type:           ProjectArgType(a.Type),
			Required:       a.Required,
			Default:        a.Default,
			Enum:           cloneStrings(a.Enum),
			Sensitive:      a.Sensitive,
			Help:           a.Help,
			AgentRequired:  a.AgentRequired,
			AgentConfirm:   a.AgentConfirm,
			SelectionGroup: a.SelectionGroup,
			RawSchema:      cloneBytes(a.RawSchema),
		})
	}
	return out
}

// ProjectSafety maps pinner.Safety onto opmesh.Safety. The switch documents
// the full mapping; the default keeps the projection total by falling back to
// an ordinal cast, which is exact because the two enums are iota-ordered in
// lockstep (asserted by the tests).
func ProjectSafety(s pinner.Safety) opmesh.Safety {
	switch s {
	case pinner.SafetyRead:
		return opmesh.SafetyRead
	case pinner.SafetyMutate:
		return opmesh.SafetyMutate
	case pinner.SafetyDestructive:
		return opmesh.SafetyDestructive
	default:
		return opmesh.Safety(s)
	}
}

// ProjectInteraction maps pinner.Interaction onto opmesh.Interaction.
// Same contract as ProjectSafety.
func ProjectInteraction(i pinner.Interaction) opmesh.Interaction {
	switch i {
	case pinner.InteractionAgentSafe:
		return opmesh.InteractionAgentSafe
	case pinner.InteractionHumanOnly:
		return opmesh.InteractionHumanOnly
	case pinner.InteractionNeedsHandoff:
		return opmesh.InteractionNeedsHandoff
	default:
		return opmesh.Interaction(i)
	}
}

// ProjectVisibility maps pinner.Visibility onto opmesh.Visibility.
// Same contract as ProjectSafety.
func ProjectVisibility(v pinner.Visibility) opmesh.Visibility {
	switch v {
	case pinner.VisibilityModel:
		return opmesh.VisibilityModel
	case pinner.VisibilityAppOnly:
		return opmesh.VisibilityAppOnly
	case pinner.VisibilityBoth:
		return opmesh.VisibilityBoth
	default:
		return opmesh.Visibility(v)
	}
}

// ProjectArgType maps pinner.ArgType onto opmesh.ArgType.
// Same contract as ProjectSafety.
func ProjectArgType(a pinner.ArgType) opmesh.ArgType {
	switch a {
	case pinner.ArgTypeString:
		return opmesh.ArgTypeString
	case pinner.ArgTypeBool:
		return opmesh.ArgTypeBool
	case pinner.ArgTypeNullableBool:
		return opmesh.ArgTypeNullableBool
	case pinner.ArgTypeNullableInt:
		return opmesh.ArgTypeNullableInt
	case pinner.ArgTypeInt:
		return opmesh.ArgTypeInt
	case pinner.ArgTypeFloat:
		return opmesh.ArgTypeFloat
	case pinner.ArgTypeDuration:
		return opmesh.ArgTypeDuration
	case pinner.ArgTypeStringSlice:
		return opmesh.ArgTypeStringSlice
	case pinner.ArgTypeFlexibleID:
		return opmesh.ArgTypeFlexibleID
	case pinner.ArgTypeRawJSON:
		return opmesh.ArgTypeRawJSON
	default:
		return opmesh.ArgType(a)
	}
}

// RegisterAll projects and registers each operation into an opmesh.Catalog,
// returning the first registration error. A nil catalog is a wiring bug and
// is reported as ErrNilCatalog rather than a panic; nil operation elements
// are intentionally skipped, consistent with ProjectAll, which also drops
// nils.
//
// RegisterAll performs no environment or surface gating of its own: it
// registers every operation passed to it, including EnvLocalOnly and
// EnvCLIOnly carve-out operations such as auth_login and auth_logout. A
// caller serving the resulting catalog to a target surface must therefore
// filter by each operation's Environment (see pinner.Operation.Environment
// and the pinner root model, or the surface's own gating) before calling
// RegisterAll, or before serving the catalog, otherwise local-only
// operations would be exposed on a hosted surface. This is the projection
// contract stated above: the projection carries only the opmesh-owned
// vocabulary and never filters by Environment — surface gating belongs to
// the consumption layer.
func RegisterAll(cat opmesh.Catalog, ops ...pinner.Operation) error {
	if cat == nil {
		return ErrNilCatalog
	}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := cat.Add(Project(op)); err != nil {
			return err
		}
	}
	return nil
}

func cloneStrings(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	out := make([]string, len(v))
	copy(out, v)
	return out
}

func cloneBytes(v []byte) []byte {
	if len(v) == 0 {
		return nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out
}
