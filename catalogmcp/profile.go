package catalogmcp

import (
	"fmt"
	"sync"

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
// probe instead of exposing a mcpforge.FeatureSet — pinner-cli's
// hostenv.PlatformProfile (Has/IsTransport/IsHost/CloneFeatures) is the
// canonical example — do NOT satisfy ForgeFeatureCarrier, and hand-adapting is
// where features get silently lost. The migration path is therefore explicit,
// on the module side, and lossless:
//
//	// pinner-cli, migrating its hostenv.PlatformProfile:
//	carrier := catalogops.ProfileFromHas(func(f string) bool {
//	    return prof.Has(hostenv.Feature(f)) // hostenv.Feature is a string
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
// feature. It is the Has-style capability surface of host profiles such as
// pinner-cli's hostenv.PlatformProfile, without any host-package import.
type FeatureProbe func(feature string) bool

// profileFeatures lists every feature a description in this package gates on.
// Keep it in lockstep with the When(...) segments in websites_mcp.go: when a
// new feature-gated segment is added, add its feature here so ProfileFromHas
// adaptation stays lossless.
var profileFeatures = []mcpforge.Feature{FeatFileHostInput}

// ProfileFromHas adapts a Has-style host profile (e.g. pinner-cli's
// hostenv.PlatformProfile) into this package's feature carrier. It probes the
// given FeatureProbe for every feature in this package's description
// vocabulary, so no feature-gated segment can be silently lost at the
// any-typed catalogmcp.NewCompilerForProfile boundary. A nil probe yields an
// empty MCPProfile (equivalent to an intentionally featureless profile).
//
// Example (migration path from a hostenv.PlatformProfile `prof`):
//
//	carrier := catalogops.ProfileFromHas(func(f string) bool {
//	    return prof.Has(hostenv.Feature(f))
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

// profileGap records the most recent adapter-gap diagnostic raised while a
// description resolver adapted an any-typed profile. DescFunc resolvers can
// only return a string and must never panic, so the error path above cannot
// propagate through them; this slot keeps the gap observable instead of
// silent. profileGapMu guards both fields (resolvers may run concurrently,
// e.g. per-request description resolution in the MCP bridge).
var (
	profileGapMu sync.Mutex
	profileGap   error
)

// ProfileAdapterGap returns and clears the most recent adapter-gap diagnostic
// raised while a description resolver adapted an any-typed profile. A nil
// result means no non-nil, non-carrier profile has been seen. Consumer
// integration code (e.g. a CLI compiling its startup surface) can call this
// after catalogmcp.NewCompilerForProfile(...).Compile to assert its profile
// adaptation is wired: behind this function sits the only path where an
// unknown profile shape degrades to a featureless description.
func ProfileAdapterGap() error {
	profileGapMu.Lock()
	defer profileGapMu.Unlock()
	err := profileGap
	profileGap = nil
	return err
}

// forgeProfileOf adapts the opaque p (`any` profile in a DescFunc resolver)
// into a mcpforge.FeatureCarrier. It is total and never panics:
//
//   - nil → MCPProfile{}: the documented profile-less case.
//   - a ForgeFeatureCarrier → adopted as-is.
//   - anything else → an adapter gap. A resolver cannot error, so the gap is
//     REPORTED (readable via ProfileAdapterGap) and the base description
//     resolves; it is never conveyed as a silently featureless profile.
func forgeProfileOf(p any) mcpforge.FeatureCarrier {
	// A nil profile is the documented intentional no-profile case resolved by
	// AdaptProfile; handle it here so the description resolver's hot path
	// answers it without allocating.
	if p == nil {
		return MCPProfile{}
	}
	c, err := AdaptProfile(p)
	if err != nil {
		profileGapMu.Lock()
		profileGap = err
		profileGapMu.Unlock()
		return MCPProfile{}
	}
	return c
}
