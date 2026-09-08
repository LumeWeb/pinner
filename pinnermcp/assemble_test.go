package pinnermcp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/opmesh"
)

// Assembly-level regression tests: what Assemble must guarantee about the
// profile overlay, the OpenAI-tunnel effective features, and the loud failure
// paths (un-adaptable profile, nil catalog).

// testCatalog returns a fresh empty opmesh catalog so Assemble runs its full
// projection path (empty catalog -> empty compiled surface).
func testCatalog() opmesh.Catalog { return opmesh.NewCatalog() }

// directTool returns the direct tool registered under name.
func directTool(t *testing.T, srv *Server, name string) (model.ToolDescriptor, bool) {
	t.Helper()
	for _, d := range srv.Direct {
		if d.Name == name {
			return d, true
		}
	}
	return model.ToolDescriptor{}, false
}

// --- Finding: Config.Hosted must overlay HostProfile.Hosted ---

// TestAssembleHostedFlagOverlaysProfile pins the profile contract: Hosted is a
// server-construction-time property overlaid by Config.Hosted, so a hosted
// assembly must never present an unhosted profile (hosted-gated fragments
// would render wrong while Server.Hosted() reports true). It also pins that
// the overlay happens on the ADAPTED profile — every profile shape takes it.
func TestAssembleHostedFlagOverlaysProfile(t *testing.T) {
	cases := []struct {
		name    string
		profile any
	}{
		{"wire model profile reporting unhosted", &model.Profile{Hosted: false}},
		{"underlying feature carrier", HostProfile{}},
		{"nil (intentionally profile-less)", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, err := Assemble(Config{
				Catalog: testCatalog(),
				Hosted:  true,
				Profile: tc.profile,
			})
			require.NoError(t, err)
			require.True(t, srv.Hosted())
			require.True(t, srv.Profile().Hosted,
				"Config.Hosted must overlay HostProfile.Hosted during assembly")
		})
	}

	// The overlay works symmetrically: a hosted wire profile in an unhosted
	// assembly reads unhosted.
	srv, err := Assemble(Config{
		Catalog: testCatalog(),
		Hosted:  false,
		Profile: &model.Profile{Hosted: true},
	})
	require.NoError(t, err)
	require.False(t, srv.Profile().Hosted,
		"Config.Hosted=false must override a hosted wire profile")
}

// --- Finding: nil RelayFeatures must not silently degrade the OpenAI tunnel ---

// TestAssembleOpenAITunnelNilRelayFeaturesKeepsHostCapabilities pins the
// assembly-level contract: a tunnel assembled with TransferDeps.RelayFeatures
// unset falls back to the ORIGINAL pinner-cli effective-features set — the
// transport mechanism features PLUS the ChatGPT host capabilities — so
// upload_file publishes the full surface: the `file` schema property, the
// ChatGPT metadata, and the host-file description instructions.
func TestAssembleOpenAITunnelNilRelayFeaturesKeepsHostCapabilities(t *testing.T) {
	srv, err := Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			TunnelOpenAI: true,
			UploadFile:   true,
			DataURIWired: true,
			// RelayFeatures deliberately nil: the fallback must supply the
			// host capabilities the original OpenAI tunnel effective set
			// carried (FeatFileHostInput, FeatXMcpFile, FeatMCPApps,
			// FeatElicitation) instead of degrading to the mechanism-only
			// features.
		},
	})
	require.NoError(t, err)

	upload, ok := directTool(t, srv, "upload_file")
	require.True(t, ok, "upload_file must be registered when UploadFile is wired")

	// Schema: the host-file property survives the nil-RelayFeatures fallback.
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(upload.InputSchema, &schema))
	require.Contains(t, schema.Properties, "file",
		"tunnel schema must keep the host-provided `file` input (FeatFileHostInput)")
	source, ok := schema.Properties["source"]
	require.True(t, ok)
	require.Contains(t, string(source), `"url"`, "tunnel source.mode enum must keep url")
	require.Contains(t, string(source), `"data"`, "tunnel source.mode enum must keep data")

	// Metadata: the ChatGPT fileParams advertisement survives.
	require.NotNil(t, upload.Meta, "ChatGPT file-handoff metadata must be advertised")
	require.Equal(t,
		[]string{"file"},
		upload.Meta["openai/fileParams"],
		"ChatGPT fileParams metadata must advertise the `file` argument")

	// Description: the host-file instructions render alongside the tunnel
	// relay segments, matching pinner-cli's original tunnel description.
	require.Contains(t, upload.Description, "the OpenAI runtime converts it",
		"tunnel description must carry the host-file handoff guidance")
	require.Contains(t, upload.Description, "file=<host file> and archive_mode=convert",
		"tunnel description must carry the website-ZIP host-file route")

	// Capabilities consistency: the effective feature set the fallback
	// seeded is the SAME set the capabilities report gates its upload_tools
	// listing on — a DataURIWired relay tool registered off the seeded
	// FeatSourceData is also reported in upload_tools.
	cap, ok := directTool(t, srv, "capabilities")
	require.True(t, ok, "capabilities tool must always be registered")
	res, err := cap.Handler(t.Context(), model.ToolRequest{})
	require.NoError(t, err)
	report, ok := res.StructuredContent.(CapabilityReport)
	require.True(t, ok, "capabilities handler must return a CapabilityReport")
	require.Equal(t, []UploadToolCapability{UploadToolFile, UploadToolData}, report.UploadTools,
		"upload_tools must mirror the registered relay tools under the seeded feature set")
}

// TestAssembleDataURIRegistrationMatchesEffectiveFeatures pins the combined
// registration condition for upload_data: the DataURIWired flag AND the
// effective feature set carrying FeatSourceData must both hold, and the
// capabilities report must agree with the registration either way.
func TestAssembleDataURIRegistrationMatchesEffectiveFeatures(t *testing.T) {
	// HTTP with DataURIWired but nil RelayFeatures: the HTTP mechanism
	// fallback does NOT carry FeatSourceData, so upload_data is NOT
	// registered and NOT reported.
	srv, err := Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:   true,
			DataURIWired: true,
		},
	})
	require.NoError(t, err)
	_, present := directTool(t, srv, "upload_data")
	require.False(t, present, "no FeatSourceData in the HTTP fallback -> upload_data not registered")
	cap, ok := directTool(t, srv, "capabilities")
	require.True(t, ok)
	res, err := cap.Handler(t.Context(), model.ToolRequest{})
	require.NoError(t, err)
	report := res.StructuredContent.(CapabilityReport)
	require.Equal(t, []UploadToolCapability{UploadToolFile}, report.UploadTools)

	// Explicit RelayFeatures declaring FeatSourceData: upload_data IS
	// registered and reported.
	srv, err = Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:   true,
			DataURIWired: true,
			RelayFeatures: mcpforge.FeatureSet{
				FeatSourceMint: true,
				FeatSourceData: true,
				FeatSinkLocal:  true,
			},
		},
	})
	require.NoError(t, err)
	_, present = directTool(t, srv, "upload_data")
	require.True(t, present, "explicit FeatSourceData -> upload_data registered")
	cap, ok = directTool(t, srv, "capabilities")
	require.True(t, ok)
	res, err = cap.Handler(t.Context(), model.ToolRequest{})
	require.NoError(t, err)
	report = res.StructuredContent.(CapabilityReport)
	require.Equal(t, []UploadToolCapability{UploadToolFile, UploadToolData}, report.UploadTools,
		"the capabilities report must mirror the registered data relay")
}

// --- Finding: profile adaptation must happen once and fail loudly ---

// TestAssembleRejectsUnadaptableProfile pins the loud-failure contract: an
// un-adaptable Config.Profile surfaces as a *ProfileAdapterError from
// Assemble (propagated, never swallowed into a nil profile that would
// silently compile a featureless catalog surface).
func TestAssembleRejectsUnadaptableProfile(t *testing.T) {
	_, err := Assemble(Config{
		Catalog: testCatalog(),
		Profile: "not a supported profile shape",
	})
	require.Error(t, err)
	var adapterErr *ProfileAdapterError
	require.ErrorAs(t, err, &adapterErr, "expected a *ProfileAdapterError")
	require.Contains(t, adapterErr.Error(), "cannot be adapted")
}

// --- Finding: typed-nil Catalog must not masquerade as a configured catalog ---

// TestAssembleRejectsTypedNilCatalog pins the typed-nil guard: an interface
// variable holding a nil concrete catalog is NOT a configured catalog — the
// assembly must report the absent catalog loudly instead of passing a nil
// catalog into the compiler.
func TestAssembleRejectsTypedNilCatalog(t *testing.T) {
	concrete := opmesh.NewCatalog()
	typedNil := reflect.Zero(reflect.TypeOf(concrete)).Interface().(opmesh.Catalog)

	require.True(t, isNilCatalog(typedNil), "typed-nil catalog must read as nil")
	require.True(t, isNilCatalog(opmesh.Catalog(nil)))
	require.False(t, isNilCatalog(testCatalog()))

	_, err := Assemble(Config{Catalog: typedNil})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no operation catalog")
}

// TestTransportStartupFeatures pins the effective-feature fallback per
// transport: mechanism-only for stdio/HTTP, mechanism + ChatGPT host caps for
// the embedded OpenAI tunnel — matching pinner-cli's effectiveFeaturesFor.
func TestTransportStartupFeatures(t *testing.T) {
	stdio := transportStartupFeatures(canimcp.TransportStdio)
	require.True(t, stdio.Has(FeatSourcePath))
	require.False(t, stdio.Has(FeatFileHostInput), "stdio startup set carries no host-file cap")

	httpFS := transportStartupFeatures(canimcp.TransportHTTP)
	require.True(t, httpFS.Has(FeatSourceMint))
	require.False(t, httpFS.Has(FeatFileHostInput), "generic HTTP startup set carries no host-file cap")

	tunnel := transportStartupFeatures(canimcp.TransportOpenAI)
	require.True(t, tunnel.Has(FeatSourceURL))
	require.True(t, tunnel.Has(FeatSourceData))
	for _, f := range []mcpforge.Feature{FeatFileHostInput, FeatXMcpFile, FeatMCPApps, FeatElicitation} {
		require.True(t, tunnel.Has(f), "tunnel startup set must carry %s", f)
	}

	// The fallback returns a fresh set: callers may overlay it without
	// corrupting the next call's set.
	tunnel[FeatSourcePath] = true
	require.False(t, transportStartupFeatures(canimcp.TransportOpenAI).Has(FeatSourcePath),
		"transportStartupFeatures must return a set no caller can mutate package state with")
}
