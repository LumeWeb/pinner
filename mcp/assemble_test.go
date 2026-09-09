package mcp

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/transfer"
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
// unset falls back to the full startup-effective set — the
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
	// relay segments, matching the startup-effective tunnel description.
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

// --- Finding: typed-nil forge feature carrier must not panic in adaptation ---

// panickyCarrier implements catalogmcpForgeCarrier with a FeatureSet that
// dereferences its receiver. Calling it on a nil pointer panics, so any test
// reaching it through a typed-nil profile fails loudly instead of silently
// passing (the guard must route the typed-nil away from the method call).
type panickyCarrier struct{ features mcpforge.FeatureSet }

func (c *panickyCarrier) FeatureSet() mcpforge.FeatureSet { return c.features }

// TestAssembleAdaptsTypedNilCarrier pins the typed-nil guard for the forge
// feature-carrier path of AdaptHostProfile: a Config.Profile holding a nil
// *panickyCarrier matches the carrier assertion, so without the guard
// FeatureSet() runs on a nil receiver and panics. The documented profile-less
// result (HostProfile{}, nil) must be returned instead. The full Assemble path
// is exercised to catch a re-introduction of the panic through assembly.
func TestAssembleAdaptsTypedNilCarrier(t *testing.T) {
	typedNil := reflect.Zero(reflect.TypeOf(&panickyCarrier{})).Interface()

	require.True(t, isNilCarrier(typedNil), "typed-nil carrier must read as nil")
	require.False(t, isNilCarrier(&panickyCarrier{}), "live carrier must not read as nil")

	require.NotPanics(t, func() {
		hp, err := AdaptHostProfile(typedNil)
		require.NoError(t, err, "typed-nil carrier adapts to the profile-less result")
		require.Equal(t, HostProfile{}, hp, "identical to the nil-profile result")
	})

	// The assembly path must not panic either; the typed-nil carrier is
	// treated as intentionally profile-less (like a nil *canimcp.Profile).
	require.NotPanics(t, func() {
		_, err := Assemble(Config{Catalog: testCatalog(), Profile: typedNil})
		require.NoError(t, err, "typed-nil carrier profile assembles as profile-less")
	})
}

// --- Finding (C1): upload_url advertising and registration share ONE gate ---

// noopRelayHandler is a trivial TransferDeps.Relay executor stub: the honest
// gate only checks that an executor IS wired, so a do-nothing function is
// enough to make Relay != nil true.
func noopRelayHandler(context.Context, io.Reader, int64, string, bool, string, bool) (any, error) {
	return nil, nil
}

// capabilitiesUploadToolSet returns the set of upload tools this assembled
// server's capabilities report advertises.
func capabilitiesUploadToolSet(t *testing.T, srv *Server) map[UploadToolCapability]bool {
	t.Helper()
	capDesc, ok := directTool(t, srv, "capabilities")
	require.True(t, ok, "capabilities tool must always be registered")
	res, err := capDesc.Handler(t.Context(), model.ToolRequest{})
	require.NoError(t, err)
	report, ok := res.StructuredContent.(CapabilityReport)
	require.True(t, ok, "capabilities handler must return a CapabilityReport")
	out := make(map[UploadToolCapability]bool, len(report.UploadTools))
	for _, tool := range report.UploadTools {
		out[tool] = true
	}
	return out
}

// TestUploadURLAdvertiseEqualsRegistered pins the C1 invariant: upload_url is
// in the direct tools list if AND ONLY IF the capabilities report's
// upload_tools lists it, and both sides are gated on the SAME shared gate —
// TransferDeps.RelayURLRegistered(features) = RelayURLWired && Relay != nil &&
// FeatSourceURL. The full Assemble path exercises both halves through
// DIFFERENT code (the registration branch in buildDirectTools vs
// uploadToolsFor in the capabilities descriptor), so a one-sided gate drift
// fails the advertise==registered pairing, and the per-case expected values
// keep the pairing itself from tautologically agreeing on a shared wrong
// answer.
func TestUploadURLAdvertiseEqualsRegistered(t *testing.T) {
	cases := []struct {
		name      string
		transfer  TransferDeps
		wantWired bool
	}{
		{
			// All three gate conjuncts hold: executor wired, caller wiring
			// flag set, registration-time features declare FeatSourceURL.
			name: "wired: relay executor + RelayURLWired + FeatSourceURL",
			transfer: TransferDeps{
				UploadFile:    true,
				TunnelOpenAI:  true,
				Relay:         noopRelayHandler,
				RelayURLWired: true,
				RelayFeatures: tunnelFeatures(),
			},
			wantWired: true,
		},
		{
			// FeatSourceURL missing from the registration-time set: the gate
			// fails even with the executor and the wiring flag in place.
			name: "not wired: RelayURLWired set but feature absent",
			transfer: TransferDeps{
				UploadFile:    true,
				Relay:         noopRelayHandler,
				RelayURLWired: true,
				RelayFeatures: httpFeatures(),
			},
			wantWired: false,
		},
		{
			// The executor is missing: RelayURLRegistered is false even
			// though the wiring flag and the feature both hold.
			name: "not wired: feature declared but Relay executor nil",
			transfer: TransferDeps{
				UploadFile:    true,
				TunnelOpenAI:  true,
				RelayURLWired: true,
				RelayFeatures: tunnelFeatures(),
			},
			wantWired: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantWired,
				tc.transfer.RelayURLRegistered(tc.transfer.RelayFeatures),
				"the shared gate RelayURLRegistered must equal the case's wiring intent")

			srv, err := Assemble(Config{Catalog: testCatalog(), Transfer: tc.transfer})
			require.NoError(t, err)

			_, registered := directTool(t, srv, "upload_url")
			advertised := capabilitiesUploadToolSet(t, srv)[UploadToolURL]

			require.Equal(t, tc.wantWired, registered,
				"upload_url registration must follow RelayURLRegistered && FeatSourceURL")
			require.Equal(t, tc.wantWired, advertised,
				"upload_url advertisement must follow the SAME reconciled gate")
			require.Equal(t, registered, advertised,
				"the C1 invariant: upload_url registered iff advertised — no advertise-without-register, no register-without-advertise")
		})
	}
}

// --- Finding: the copy feature set must not name unregistered relay tools ---

// TestAssembleDataURICopyFeatureFollowsRegistrationGate pins the caption
// strip contract: upload_data registers only when DataURIWired AND the
// effective feature set carries FeatSourceData, so when that combined gate
// fails the strip must remove FeatSourceData from the copy feature set —
// otherwise the capabilities chooser, the agent guide, and upload_file's
// source.mode prose would advertise a data relay tools/list does not serve.
func TestAssembleDataURICopyFeatureFollowsRegistrationGate(t *testing.T) {
	// The host declares FeatSourceData in the effective feature set, but
	// DataURIWired is false. upload_url is unwired too (RelayURLWired unset),
	// so both relay features must end up stripped.
	features := httpFeatures()
	features[FeatSourceData] = true

	srv, err := Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:    true,
			RelayFeatures: features,
			// DataURIWired deliberately false: the registration gate fails.
		},
	})
	require.NoError(t, err)

	_, present := directTool(t, srv, "upload_data")
	require.False(t, present, "DataURIWired=false -> upload_data not registered")
	_, present = directTool(t, srv, "upload_url")
	require.False(t, present, "upload_url unwired -> not registered")

	upload, ok := directTool(t, srv, "upload_file")
	require.True(t, ok, "upload_file must be registered so its source copy is observable")
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(upload.InputSchema, &schema))
	source, ok := schema.Properties["source"]
	require.True(t, ok)
	require.NotContains(t, string(source), "separate upload_data",
		"the unwired data relay must be stripped from the copy set: upload_file's source prose names upload_data only when FeatSourceData survived")
	require.NotContains(t, string(source), "separate upload_url",
		"the unwired url relay must be stripped too")

	cap, ok := directTool(t, srv, "capabilities")
	require.True(t, ok, "capabilities tool must always be registered")
	res, err := cap.Handler(t.Context(), model.ToolRequest{})
	require.NoError(t, err)
	report, ok := res.StructuredContent.(CapabilityReport)
	require.True(t, ok, "capabilities handler must return a CapabilityReport")
	require.Equal(t, []UploadToolCapability{UploadToolFile}, report.UploadTools,
		"no relay tool registered -> upload_tools must list upload_file only")

	// Positive: the same feature set with DataURIWired=true retains
	// FeatSourceData — upload_data registers and upload_file keeps the
	// source.mode=data copy.
	srv, err = Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:    true,
			DataURIWired:  true,
			RelayFeatures: features,
		},
	})
	require.NoError(t, err)
	_, present = directTool(t, srv, "upload_data")
	require.True(t, present, "DataURIWired=true + FeatSourceData -> upload_data registered")
	upload, ok = directTool(t, srv, "upload_file")
	require.True(t, ok)
	require.NoError(t, json.Unmarshal(upload.InputSchema, &schema))
	require.Contains(t, string(schema.Properties["source"]), "separate upload_data",
		"the registered data relay keeps FeatSourceData in the copy set")
}

// --- Finding: the drop sink must follow the filedrop coordinator wiring ---

// TestAssembleDropSinkFollowsFileDropWiring pins the capabilities/schema
// gate for sink=drop: the download_file tool gates the drop branch on
// FileDrop != nil ONLY (never on the raw DropWired registration flag), and
// the capabilities report must advertise sink=drop against that same single
// source of truth — a composition root that sets DropWired without
// coordinating a FileDrop must not get a drop sink advertised the registered
// tool would reject.
func TestAssembleDropSinkFollowsFileDropWiring(t *testing.T) {
	downloadSinks := func(t *testing.T, srv *Server) []FileOutputCapability {
		t.Helper()
		cap, ok := directTool(t, srv, "capabilities")
		require.True(t, ok, "capabilities tool must always be registered")
		res, err := cap.Handler(t.Context(), model.ToolRequest{})
		require.NoError(t, err)
		report, ok := res.StructuredContent.(CapabilityReport)
		require.True(t, ok, "capabilities handler must return a CapabilityReport")
		return report.DownloadSinkModes
	}

	// DropWired set but no FileDrop coordinator: nil disables the drop sink.
	srv, err := Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:   true,
			DownloadFile: true,
			DropWired:    true,
			// FileDrop deliberately nil: the tool's drop gate fails.
		},
	})
	require.NoError(t, err)
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal}, downloadSinks(t, srv),
		"DropWired without a FileDrop coordinator must not advertise sink=drop")

	// A wired FileDrop coordinator: sink=drop is advertised and download_file
	// registers with the drop branch.
	srv, err = Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:   true,
			DownloadFile: true,
			DropWired:    true,
			FileDrop:     &transfer.Download{},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal, CapabilitySinkDrop}, downloadSinks(t, srv),
		"a wired FileDrop coordinator advertises sink=drop")
	_, present := directTool(t, srv, "download_file")
	require.True(t, present, "DownloadFile wired -> download_file registered")
}

// TestAssembleDropProseFollowsFileDropWiring extends the sink-mode pin above
// to the POS: the FeatSinkDrop-bearing copy resolved through the copy feature
// set and the agent guide. With download_file registered, both the
// capabilities description and the agent guide resolve their drop prose
// against the SAME drop-availability condition the report's DownloadSinkModes
// and the download_file tool's drop gate apply (FileDrop != nil &&
// !tunnelOpenAI): a composition root registering download_file without a
// FileDrop coordinator must not serve prose advertising sink=drop the tool
// would reject at invocation time.
func TestAssembleDropProseFollowsFileDropWiring(t *testing.T) {
	guide := func(t *testing.T, srv *Server) string {
		t.Helper()
		desc, ok := directTool(t, srv, "agent_guide")
		require.True(t, ok, "agent_guide must always be registered")
		res, err := desc.Handler(t.Context(), model.ToolRequest{
			Arguments: map[string]any{},
			// An HTTP host declaring the sink-drop mechanism feature: on the
			// wire profile alone the guide would render the drop prose.
			Caps: &model.RequestCaps{Profile: modelProfileWith(FeatSinkDrop, canimcp.TransportHTTP)},
		})
		require.NoError(t, err)
		return res.Text
	}

	// DownloadFile registered, FileDrop coordinator nil: the drop sink is not
	// real, so no drop prose may render anywhere.
	srv, err := Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:   true,
			DownloadFile: true,
			DropWired:    true,
			// FileDrop deliberately nil: the tool's drop gate fails.
		},
	})
	require.NoError(t, err)

	capabilities, ok := directTool(t, srv, "capabilities")
	require.True(t, ok, "capabilities tool must always be registered")
	require.NotContains(t, capabilities.Description, "one-time filedrop link",
		"without a FileDrop coordinator the capabilities description must not advertise sink=drop")

	text := guide(t, srv)
	require.NotContains(t, text, "Prefer sink=drop",
		"without a FileDrop coordinator the agent guide must not steer to sink=drop")
	require.Contains(t, text, "sink=local is the only sink offered",
		"without a FileDrop coordinator the agent guide must state sink=local is the only sink")

	// Positive control: a wired FileDrop coordinator keeps the drop prose in
	// both surfaces (the feature carries through and the drop gate passes).
	srv, err = Assemble(Config{
		Catalog: testCatalog(),
		Transfer: TransferDeps{
			UploadFile:   true,
			DownloadFile: true,
			DropWired:    true,
			FileDrop:     &transfer.Download{},
		},
	})
	require.NoError(t, err)

	capabilities, ok = directTool(t, srv, "capabilities")
	require.True(t, ok)
	require.Contains(t, capabilities.Description, "one-time filedrop link",
		"a wired FileDrop coordinator keeps the capabilities description's drop prose")

	text = guide(t, srv)
	require.Contains(t, text, "Prefer sink=drop",
		"a wired FileDrop coordinator keeps the agent guide's drop prose")
}

// TestTransportStartupFeatures pins the effective-feature fallback per
// transport: mechanism-only for stdio/HTTP, mechanism + ChatGPT host caps for
// the embedded OpenAI tunnel.
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
