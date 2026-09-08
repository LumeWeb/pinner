package catalogops

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/pinner"
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
func websitesCreateOp(t *testing.T) pinner.Operation {
	t.Helper()
	for _, op := range WebsitesOperations(WebsitesDeps{}) {
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

	cat := pinner.NewCatalog()
	require.NoError(t, cat.Add(websitesCreateOp(t)))

	tools, err := pinner.NewMCPCompilerForProfile(carrier).Compile(cat)
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
	require.NoError(t, ProfileAdapterGap(),
		"an adapted carrier must not raise adapter-gap diagnostics")
}

// TestUnadaptedPlatformProfileRaisesAdapterGap pins the loud (not silent)
// behavior for a profile that crosses the boundary unadapted: AdaptProfile
// errors, the description resolver degrades to the base description, and the
// gap is REPORTED via ProfileAdapterGap rather than dropping features
// invisibly.
func TestUnadaptedPlatformProfileRaisesAdapterGap(t *testing.T) {
	prof := openAITunnelProfileDouble()

	// The explicit adapter refuses the shape instead of synthesizing an
	// empty feature set.
	carrier, err := AdaptProfile(prof)
	require.Error(t, err, "a non-carrier profile must be an explicit adapter error")
	var gapErr *ProfileAdapterError
	require.ErrorAs(t, err, &gapErr)
	require.Nil(t, carrier)

	// The DescFunc resolver path (what the any-typed bridge actually calls)
	// must surface the same gap, not just quietly drop the clause.
	require.NoError(t, ProfileAdapterGap(), "test setup: gap channel must be clear")
	desc := websitesCreateTargets[0].DescFunc(prof)
	require.Error(t, ProfileAdapterGap(),
		"an unadapted non-nil profile must raise a detectable adapter-gap diagnostic")
	require.NotContains(t, desc, "file parameter is the preferred byte path")

	// The gap channel consumed by callers is what turns the previously
	// silent drop into a detectable failure.
}
