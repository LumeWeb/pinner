package catalogmcp

import (
	"fmt"

	"go.lumeweb.com/mcpforge"
)

// Profile adaptation contract.
//
// pinner's MCP bridge is any-typed: catalogmcp.NewCompilerForProfile takes an
// opaque `any` and hands it to DescFunc description resolvers. The ONLY thing
// this package needs from a profile is its mcpforge.FeatureSet, expressed via
// ForgeFeatureCarrier (FeatureSet() mcpforge.FeatureSet). Two shapes cross the
// boundary legitimately:
//
//   - nil — intentionally profile-less. Feature-gated description segments
//     resolve as omitted (the base description). This is a documented,
//     meaningful configuration, not an adapter gap.
//   - any value implementing ForgeFeatureCarrier — including this package's
//     MCPProfile. Adopted as-is; nothing is lost.
//
// Consumers that own a richer host profile which gates features by boolean
// probe instead of exposing a mcpforge.FeatureSet — a profile with a
// Has/IsTransport/IsHost/CloneFeatures surface is the
// canonical example — do NOT satisfy ForgeFeatureCarrier, and hand-adapting is
// where features get silently lost. The adaptation path is therefore explicit,
// on the consumer side, and lossless:
//
//	// adapting a Has-style host profile `prof`:
//	carrier := catalogops.ProfileFromHas(func(f string) bool {
//	    return prof.Has(f)
//	})
//	compiler := catalogmcp.NewCompilerForProfile(carrier)
//
// ProfileFromHas probes every feature this package's descriptions gate on, so
// adapting through it cannot drop a segment the DSL knows about
// (IsTransport/IsHost predicates stay consumer-side; this package's DSL gates
// exclusively on features). Consumers that already carry a full feature set
// may implement ForgeFeatureCarrier directly and bypass the probe entirely.
//
// AdaptProfile converts an opaque profile into its carrier and returns a
// ProfileAdapterError (instead of a silently empty feature set) for any shape
// it cannot adapt, so integration mistakes surface as errors, never as
// description text that quietly lost a clause.

// FeatureProbe reports whether the underlying host profile enables the named
// feature. It is the Has-style capability surface of rich host profiles,
// without any host-package import.
type FeatureProbe func(feature string) bool

// profileFeatures lists every feature a description in this package gates on.
// Keep it in lockstep with the When(...) segments in websites_mcp.go: when a
// new feature-gated segment is added, add its feature here so ProfileFromHas
// adaptation stays lossless.
var profileFeatures = []mcpforge.Feature{FeatFileHostInput}

// ProfileFromHas adapts a Has-style host profile (e.g. one exposing
// Has(feature string) bool) into this package's feature carrier. It probes the
// given FeatureProbe for every feature in this package's description
// vocabulary, so no feature-gated segment can be silently lost at the
// any-typed catalogmcp.NewCompilerForProfile boundary. A nil probe yields an
// empty MCPProfile (equivalent to an intentionally featureless profile).
//
// Example (adapting a Has-style host profile `prof`):
//
//	carrier := catalogops.ProfileFromHas(func(f string) bool {
//	    return prof.Has(f)
//	})
//	compiler := catalogmcp.NewCompilerForProfile(carrier)
func ProfileFromHas(has FeatureProbe) MCPProfile {
	if has == nil {
		return MCPProfile{}
	}
	fs := make(mcpforge.FeatureSet, len(profileFeatures))
	for _, f := range profileFeatures {
		fs[f] = has(string(f))
	}
	return MCPProfile{Features: fs}
}

// ProfileAdapterError reports a profile that crossed the any-typed profile
// boundary without adaptation: it is neither nil nor a ForgeFeatureCarrier.
type ProfileAdapterError struct {
	// ProfileType is the Go type of the unadapted profile (fmt %T).
	ProfileType string
}

// Error implements error.
func (e *ProfileAdapterError) Error() string {
	return "catalogmcp: profile of type " + e.ProfileType +
		" does not implement ForgeFeatureCarrier (FeatureSet() mcpforge.FeatureSet); " +
		"adapt Has-style host profiles via ProfileFromHas before passing them " +
		"to catalogmcp.NewCompilerForProfile, or every feature-gated description " +
		"segment is omitted"
}

// AdaptProfile is the explicit adapter at the any-typed boundary (the value
// handed to catalogmcp.NewCompilerForProfile). Accepted shapes:
//
//   - nil → (MCPProfile{}, nil): intentionally profile-less; the base
//     description resolves with feature segments omitted.
//   - a ForgeFeatureCarrier (including MCPProfile) → returned as-is.
//
// Anything else is an adapter gap and returns a *ProfileAdapterError rather
// than a silently empty feature set.
func AdaptProfile(p any) (mcpforge.FeatureCarrier, error) {
	if p == nil {
		return MCPProfile{}, nil
	}
	if c, ok := p.(ForgeFeatureCarrier); ok {
		return c, nil
	}
	return nil, &ProfileAdapterError{ProfileType: fmt.Sprintf("%T", p)}
}

// forgeProfileOf adapts the opaque p (`any` profile in a DescFunc resolver)
// into a mcpforge.FeatureCarrier. It is total and never panics:
//
//   - nil → MCPProfile{}: the documented profile-less case.
//   - a ForgeFeatureCarrier → adopted as-is.
//   - anything else → an adapter gap. A resolver cannot error, so the gap is
//     REPORTED (readable via (*mcpCompiler).AdapterGap on the compiler whose
//     Compile triggered the resolution) and the base description resolves; it
//     is never conveyed as a silently featureless profile.
//
// DescFunc has a fixed signature (func(any) string, defined in a lower layer)
// and cannot accept a recorder, so the adapter-gap diagnostic is recorded on
// the compiler currently compiling: mcpCompiler.Compile installs itself as
// the package-level activeCompiler for the duration of its call, and because
// Compile resolves DescFunc synchronously in the same goroutine, the active
// compiler is correctly scoped. The atomic pointer + per-compiler mutex keep
// the recording race-free without a package-global mutable gap slot.
func forgeProfileOf(p any) mcpforge.FeatureCarrier {
	// A nil profile is the documented intentional no-profile case resolved by
	// AdaptProfile; handle it here so the description resolver's hot path
	// answers it without allocating.
	if p == nil {
		return MCPProfile{}
	}
	c, err := AdaptProfile(p)
	if err != nil {
		if m := activeCompiler.Load(); m != nil {
			m.gapMu.Lock()
			m.gap = err
			m.gapMu.Unlock()
		}
		return MCPProfile{}
	}
	return c
}
