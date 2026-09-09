package catalogmcp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpforge"
)

// profileWith builds a profile at the pure-feature level:
// the characterization tests exercise the DSL
// resolution against an explicit FeatureSet carrying the feature key
// "file-host-input".
func profileWith(features ...mcpforge.Feature) MCPProfile {
	fs := mcpforge.FeatureSet{}
	for _, f := range features {
		fs[f] = true
	}
	return MCPProfile{Features: fs}
}

// profileFileHostFakeHost is a file host: FeatFileHostInput on. (Only the
// feature gates affect this description.)
var profileFileHostFakeHost = profileWith(FeatFileHostInput)

// TestWebsitesCreateDescNoFileHost verifies the per-profile MCP description
// for websites_create resolves correctly for mint-only HTTP hosts (no file
// parameter). Key invariants:
//   - The description must contain the CID structure invariant.
//   - It must contain the archive_mode=convert guidance.
//   - It must contain the wrap=true / auto-name guidance.
//   - It must contain the pins_add-unnecessary guidance.
//   - It must NOT contain the file-parameter preferred-path clause (no file).
func TestWebsitesCreateDescNoFileHost(t *testing.T) {
	desc := websitesCreateDesc.Resolve(profileWith())

	require.Contains(t, desc, "Create a website that serves an IPFS CID")
	require.Contains(t, desc, "directory whose root contains index.html")
	require.Contains(t, desc, "rejects a CID whose root has no index.html")
	require.Contains(t, desc, "A multi-file website is published as its component files")
	require.Contains(t, desc, "archive_mode=convert")
	require.Contains(t, desc, "wrap=true")
	require.Contains(t, desc, "auto-names wrapped HTML to index.html")
	require.Contains(t, desc, "starter-site")
	require.Contains(t, desc, `{"cid":"<cid>"}`)
	require.Contains(t, desc, "platform subdomain is auto-minted")
	require.Contains(t, desc, "a domain or label is not invented for a generic request")
	require.Contains(t, desc, "the upload already pinned it, so pins_add after upload is unnecessary")
	require.Contains(t, desc, "publish_website flow")
	require.Contains(t, desc, "Returns the created website")

	require.NotContains(t, desc, "file parameter is the preferred byte path")
}

// TestWebsitesCreateDescFileHost verifies the per-profile MCP description for
// websites_create on a file-parameter host (FeatFileHostInput on). The
// file-parameter preferred-path clause must be present.
func TestWebsitesCreateDescFileHost(t *testing.T) {
	desc := websitesCreateDesc.Resolve(profileFileHostFakeHost)

	require.Contains(t, desc, "Create a website that serves an IPFS CID")
	require.Contains(t, desc, "directory whose root contains index.html")
	require.Contains(t, desc, "A multi-file website is published as its component files")
	require.Contains(t, desc, "archive_mode=convert")
	require.Contains(t, desc, "wrap=true")
	require.Contains(t, desc, "the upload already pinned it, so pins_add after upload is unnecessary")
	require.Contains(t, desc, "file parameter is the preferred byte path",
		"hosts with FeatFileHostInput must see the file-parameter clause")
}

// TestWebsitesCreateDescNilFeatureSet verifies a nil-feature profile resolves
// like a featureless one (empty carrier) instead of panicking.
func TestWebsitesCreateDescNilFeatureSet(t *testing.T) {
	desc := websitesCreateDesc.Resolve(MCPProfile{})
	require.Contains(t, desc, "Create a website that serves an IPFS CID")
	require.NotContains(t, desc, "file parameter is the preferred byte path")
}

// TestWebsitesCreateTargetsCarriesDescFunc verifies the catalog.Target slice
// for websites_create carries a DescFunc (not a static Description), so the
// MCP bridge will resolve it per-profile at runtime.
func TestWebsitesCreateTargetsCarriesDescFunc(t *testing.T) {
	require.Len(t, websitesCreateTargets, 1)
	target := websitesCreateTargets[0]
	require.True(t, target.Visible)
	require.Nil(t, target.Require)
	require.NotNil(t, target.DescFunc,
		"websites_create target must carry a DescFunc for per-profile resolution")
	require.Empty(t, target.Description,
		"websites_create target must not carry a static Description when DescFunc is set")
}

// TestWebsitesCreateDescNoSuperParagraph verifies the resolved description is
// composed from discrete sentences (each ending with a period) rather than
// being a single monolithic string. This is a structural assertion: the
// resolved text must contain multiple sentence boundaries.
func TestWebsitesCreateDescNoSuperParagraph(t *testing.T) {
	desc := websitesCreateDesc.Resolve(profileWith())
	require.Greater(t, len(desc), 200, "description should be substantial")

	segments := websitesCreateDesc.ResolveSegments(profileWith())
	require.Greater(t, len(segments), 5,
		"description must be composed from multiple segments, not a single string")
}
