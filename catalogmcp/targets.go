// Package catalogmcp is the MCP (agent) frontend boundary for the pinner
// catalog: the only place in this module where the per-profile MCP tool
// target/description metadata, the feature-gated description DSL, and the
// mcpforge dependency live.
//
// # Why this package exists
//
// The core operation definitions in catalogops
// are expressed against go.lumeweb.com/opmesh, which deliberately owns only
// the frontend-neutral operation vocabulary (stable IDs, typed inputs,
// codecs/normalization/defaults/validation, effect classification, actor +
// confirmation policy, registry execution) and must never grow
// audience-specific fields (AgentHelp/AgentOnly/PositionalOnly/Sources,
// MCPTargets/DescFunc, Environment flags). MCP presentation metadata for
// those definitions is keyed here by
// the stable operation ID:
//
//   - Target and the TargetFor/Fallback/Hidden/FallbackFunc/MCPTargets
//     helpers — the per-profile tool description contract (requires/visible/
//     description/descFunc) and TargetsOf look-up.
//   - opTargets — the per-operation MCPTargets data for the catalogops
//     definitions.
//   - The description DSL (DescBuilder/Static/When + FeatFileHostInput) and
//     its per-request ProfileFeature resolution.
//   - The profile adapter (ForgeFeatureCarrier, ProfileFromHas, AdaptProfile)
//     and the compiler that maps an opmesh.Catalog to MCP tool descriptors
//     with target-backed descriptions.
//
// Everything that describes how non-MCP frontends present an operation (flag
// layout, positional bindings, env sources, human Help variants) stays
// outside this package; nothing about it is duplicated here.
//
// Unlike catalogmeta (stdlib-only), this package imports mcpforge. Consumers
// that must not pull MCP machinery (assembly/hosting layers) therefore depend
// on catalogmeta instead.
package catalogmcp

// Target is a per-profile presentation variant for the MCP surface: Require holds opaque
// feature-name strings the MCP bridge maps to its host feature set, and the
// core model never sees a Target.
type Target struct {
	// Require lists feature names that must all be present for this target to
	// be eligible. Empty = matches any MCP profile (universal fallback).
	Require []string
	// Visible controls whether the tool appears at all for matching profiles.
	// false = suppress the tool entirely for this profile.
	Visible bool
	// Description is the MCP description for this target. Consumers use the
	// fallback target's Description as the static descriptor description;
	// per-request resolution may override it with a more specific target's
	// Description.
	Description string
	// DescFunc is a per-profile description resolver. When non-nil, the
	// consumer calls it with the resolved host profile to produce a dynamic,
	// feature-gated description. It overrides Description at resolution time.
	// The parameter type is any because this boundary cannot know the host's
	// profile type; adapters (see profile.go) convert it to a feature carrier.
	DescFunc func(any) string
}

// MCPTargets wraps a variadic list of Targets into a slice. Use it in frontend
// adapters for readability.
func MCPTargets(targets ...Target) []Target { return targets }

// TargetFor creates a visible Target that requires all given feature names.
// Among all matching targets, the one with the most required features wins.
func TargetFor(desc string, features ...string) Target {
	return Target{Require: features, Visible: true, Description: desc}
}

// Fallback creates a visible Target with no feature requirements. It always
// matches (score 0), so it only wins when no specific target does. Every
// operation's targets should include a Fallback to guarantee resolution.
func Fallback(desc string) Target {
	return Target{Visible: true, Description: desc}
}

// Hidden creates an invisible Target that suppresses the tool entirely for
// MCP profiles matching the given features.
func Hidden(features ...string) Target {
	return Target{Require: features, Visible: false}
}

// FallbackFunc creates a visible Target with no feature requirements whose
// description is resolved dynamically via fn at per-request resolution time.
// The consumer calls fn with the resolved host profile (see profile.go for
// the adapter contract). Use it when a tool's MCP description is composed
// from feature-gated segments via the description DSL.
func FallbackFunc(fn func(any) string) Target {
	return Target{Visible: true, DescFunc: fn}
}

// TargetsOf returns the per-profile MCP targets declared for the operation
// with the given ID. It returns nil for operations with no MCP target
// declaration; such operations have no MCP-specific description and a
// compiler falls back to the operation's own Description.
func TargetsOf(opID string) []Target {
	targets := opTargets[opID]
	if targets == nil {
		return nil
	}
	out := make([]Target, len(targets))
	copy(out, targets)
	return out
}
