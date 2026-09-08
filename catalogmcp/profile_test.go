package catalogmcp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/catalogops"
)

// platformProfileDouble mirrors the capability surface of pinner-cli's
// hostenv.PlatformProfile (internal/mcp/hostenv/profile.go:166-185): typed
// HostType/Transport, a boolean-keyed Features set (FeatureSet semantics),
// and the Has/IsTransport/IsHost/CloneFeatures capability methods. It
// deliberately does NOT implement FeatureSet() mcpforge.FeatureSet — exactly
// like the real profile — so the tests below exercise the documented
// migration path (profile_adapter.go) rather than a synthetic carrier.
type hostKind string

type transportKind string

type platformProfileDouble struct {
	HostType  hostKind
	Transport transportKind
	Remote    bool
	Features  map[string]bool
}

// Has reports whether the profile supports the given feature.
func (p platformProfileDouble) Has(f string) bool {
	return p.Features[f]
}

// IsTransport reports whether the profile's transport matches t.
func (p platformProfileDouble) IsTransport(t transportKind) bool {
	return p.Transport == t
}

// IsHost reports whether the profile's host type matches h.
func (p platformProfileDouble) IsHost(h hostKind) bool {
	return p.HostType == h
}

// CloneFeatures returns a shallow copy of this profile with a cloned Features
// set, mirroring hostenv.PlatformProfile.CloneFeatures.
func (p platformProfileDouble) CloneFeatures() platformProfileDouble {
	clone := p
	clone.Features = make(map[string]bool, len(p.Features))
	for f, on := range p.Features {
		clone.Features[f] = on
	}
	return clone
}

// openAIFunnelProfile mirrors hostenv.ProfileOpenAITunnel's capability half:
// an OpenAI-style host on the openai transport with FeatFileHostInput enabled
// (the feature that gates the file-parameter description clause).
func openAITunnelProfileDouble() platformProfileDouble {
	return platformProfileDouble{
		HostType:  "openai",
		Transport: "openai",
		Features:  map[string]bool{string(FeatFileHostInput): true},
	}
}

// websitesCreateOp returns the websites_create operation from the domain set.
func websitesCreateOp(t *testing.T) opmesh.Operation {
	t.Helper()
	for _, op := range catalogops.WebsitesOperations(catalogops.WebsitesDeps{}) {
		if op.Name() == "websites_create" {
			return op
		}
	}
	t.Fatal("websites_create not found in WebsitesOperations")
	return nil
}

// TestAdaptedPlatformProfilePreservesFileHostDescription regresses the
// extraction-audit HIGH finding: a host profile adapted at the module boundary
// must keep its features, so the file-host `file` parameter clause survives.
// The profile double mirrors hostenv.PlatformProfile's capability surface
// (Has/IsTransport/IsHost/CloneFeatures) and does NOT satisfy
// ForgeFeatureCarrier; the test follows the documented migration path —
// ProfileFromHas(prof.Has) — and asserts the clause on the COMPILED model
// surface (the same path the CLI's MCP bridge uses).
func TestAdaptedPlatformProfilePreservesFileHostDescription(t *testing.T) {
	prof := openAITunnelProfileDouble()
	require.True(t, prof.Has(string(FeatFileHostInput)),
		"test setup: the host profile double must carry the file-host feature")

	carrier := ProfileFromHas(prof.Has)
	require.True(t, carrier.FeatureSet().Has(FeatFileHostInput),
		"adaptation must carry FeatFileHostInput across the boundary")

	cat := opmesh.NewCatalog()
	require.NoError(t, cat.Add(websitesCreateOp(t)))

	compiler := NewCompilerForProfile(carrier).(*mcpCompiler)
	tools, err := compiler.Compile(cat)
	require.NoError(t, err)
	require.NotEmpty(t, tools)

	var desc string
	for _, tool := range tools {
		if tool.Name == "websites_create" {
			desc = tool.Description
			break
		}
	}
	require.NotEmpty(t, desc, "websites_create must be on the compiled model surface")
	require.Contains(t, desc, "file parameter is the preferred byte path",
		"the file-host input description clause must survive profile adaptation "+
			"(a silently dropped clause is the audited regression)")
	require.NoError(t, compiler.AdapterGap(),
		"an adapted carrier must not raise adapter-gap diagnostics on its own compiler")
}

// TestUnadaptedPlatformProfileRaisesAdapterGap pins the loud (not silent)
// behavior for a profile that crosses the boundary unadapted: AdaptProfile
// errors, the description resolver degrades to the base description, and the
// gap is REPORTED on the compiling compiler's AdapterGap rather than dropping
// features invisibly.
func TestUnadaptedPlatformProfileRaisesAdapterGap(t *testing.T) {
	prof := openAITunnelProfileDouble()

	// The explicit adapter refuses the shape instead of synthesizing an
	// empty feature set.
	carrier, err := AdaptProfile(prof)
	require.Error(t, err, "a non-carrier profile must be an explicit adapter error")
	var gapErr *ProfileAdapterError
	require.ErrorAs(t, err, &gapErr)
	require.Nil(t, carrier)

	// The DescFunc resolver path (what the any-typed bridge actually calls,
	// driven through Compile) must surface the same gap on the compiler
	// instance, not just quietly drop the clause.
	compiler := NewCompilerForProfile(prof).(*mcpCompiler)
	tools, err := compiler.Compile(compileGapCatalog(t))
	require.NoError(t, err)
	require.NotEmpty(t, tools)
	require.NotContains(t, tools[0].Description, "file parameter is the preferred byte path")
	require.Error(t, compiler.AdapterGap(),
		"an unadapted non-nil profile must raise a detectable adapter-gap diagnostic "+
			"on the compiling compiler")
	require.NoError(t, compiler.AdapterGap(),
		"reading AdapterGap must clear the diagnostic")
}

// compileGapCatalog builds a minimal catalog whose websites_create op has a
// DescFunc fallback target, so compiling it exercises the profile-adaptation
// resolvers (forgeProfileOf) and can raise an adapter gap.
func compileGapCatalog(t *testing.T) opmesh.Catalog {
	t.Helper()
	cat := opmesh.NewCatalog()
	require.NoError(t, cat.Add(websitesCreateOp(t)))
	return cat
}

// TestAdapterGapIsScopedPerCompiler regresses the compiler-instance scoping of
// the adapter-gap diagnostic: the gap lives on the compiler that raised it,
// so compiling with one profile/compiler can neither pollute nor be cleared
// by another compiler's compile (the old package-global slot could produce a
// stale false alarm after compiling a valid carrier, and a later read could
// clear a real gap before its owner saw it).
func TestAdapterGapIsScopedPerCompiler(t *testing.T) {
	// Compiler A compiles with an unadaptable non-carrier profile: Compile
	// succeeds (the base description resolves) but a gap must be recorded on
	// A alone.
	compilerA := NewCompilerForProfile(openAITunnelProfileDouble()).(*mcpCompiler)
	toolsA, err := compilerA.Compile(compileGapCatalog(t))
	require.NoError(t, err)
	require.NotEmpty(t, toolsA)
	require.Error(t, compilerA.AdapterGap(),
		"compiler A must record the adapter gap raised while compiling the unadapted profile")
	require.NoError(t, compilerA.AdapterGap(),
		"reading AdapterGap must clear the diagnostic")

	// Compiler B immediately compiles the SAME catalog with a valid carrier:
	// no gap is expected, and no stale gap may leak from A's earlier compile.
	compilerB := NewCompilerForProfile(ProfileFromHas(openAITunnelProfileDouble().Has)).(*mcpCompiler)
	toolsB, err := compilerB.Compile(compileGapCatalog(t))
	require.NoError(t, err)
	require.NotEmpty(t, toolsB)
	require.NoError(t, compilerB.AdapterGap(),
		"compiler B must not inherit a stale gap leaked from compiler A")
	require.Contains(t, toolsB[0].Description, "file parameter is the preferred byte path",
		"the valid-carrier compile must resolve the feature-gated clause")

	// Re-reading A after B's compile must stay nil: A's slot was already
	// cleared and B's clean compile must not repopulate it.
	require.NoError(t, compilerA.AdapterGap(),
		"a later compile by compiler B must not repopulate compiler A's gap slot")
}
