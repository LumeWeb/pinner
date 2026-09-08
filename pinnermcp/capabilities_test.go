package pinnermcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/ieo"
)

// Characterization tests for the capabilities tool, pinned against
// pinner-cli internal/mcp/capabilities_test.go: the report must never
// advertise a mode/tool that was not registered (wiring honesty), the
// transport/source/sink enumerations travel with the transport, and the
// host_file_input gate is the conjunction of client capability and wiring.

func relayFeatures(fs ...mcpforge.Feature) mcpforge.FeatureSet {
	out := mcpforge.FeatureSet{}
	for _, f := range fs {
		out[f] = true
	}
	return out
}

func stdioFeatures() mcpforge.FeatureSet {
	return relayFeatures(FeatSourcePath, FeatSinkLocal, FeatSinkDrop, FeatCoLocated)
}

func httpFeatures() mcpforge.FeatureSet {
	return relayFeatures(FeatSourceMint, FeatSinkLocal, FeatSinkDrop, FeatRemoteAccess)
}

func tunnelFeatures() mcpforge.FeatureSet {
	return relayFeatures(FeatSourceURL, FeatSourceData, FeatSinkLocal)
}

// TestCurrentCapabilitiesStdio pins the stdio report shape.
func TestCurrentCapabilitiesStdio(t *testing.T) {
	r := CurrentCapabilities(true, false, true, false, true, false, true, false, 0)
	require.Equal(t, canimcp.TransportStdio, canimcp.TransportKind(r.Transport))
	require.Equal(t, []FileInputCapability{CapabilityLocalPath}, r.SourceModes)
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal, CapabilitySinkDrop}, r.DownloadSinkModes)
	require.True(t, r.UploadFile)
	require.True(t, r.DownloadFile)
	require.EqualValues(t, ieo.EffectiveRelayMaxBytes(0), r.RelayMaxBytes)
}

// TestCurrentCapabilitiesHTTP pins the HTTP report shape.
func TestCurrentCapabilitiesHTTP(t *testing.T) {
	r := CurrentCapabilities(false, false, true, false, true, false, true, false, 0)
	require.Equal(t, canimcp.TransportHTTP, canimcp.TransportKind(r.Transport))
	require.Equal(t, []FileInputCapability{CapabilityMint}, r.SourceModes)
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal, CapabilitySinkDrop}, r.DownloadSinkModes)
	require.Equal(t, "host_file_first", r.FileInputPolicy)
}

// TestCurrentCapabilitiesOpenAITunnel pins the tunnel report: url+data
// sources, no drop sink, draft x-mcp-file surfaced when wired.
func TestCurrentCapabilitiesOpenAITunnel(t *testing.T) {
	r := CurrentCapabilities(false, true, true, false, true, false, true, true, 1<<30)
	require.Equal(t, canimcp.TransportOpenAI, canimcp.TransportKind(r.Transport))
	require.Equal(t, []FileInputCapability{CapabilityRelayURL, CapabilityDataURI}, r.SourceModes)
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal}, r.DownloadSinkModes,
		"no reachable mux on the embedded tunnel -> sink=local only")
	require.True(t, r.DraftXFile)
	require.EqualValues(t, 1<<30, r.RelayMaxBytes)
}

// TestCurrentCapabilitiesModesNeedTools: a mode is only advertised when a
// backing tool is registered.
func TestCurrentCapabilitiesModesNeedTools(t *testing.T) {
	sinkLocalOnly := CurrentCapabilities(true, false, false, false, true, false, false, false, 0)
	require.Empty(t, sinkLocalOnly.SourceModes, "no upload tool -> no source modes")
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal}, sinkLocalOnly.DownloadSinkModes)
	require.Empty(t, sinkLocalOnly.FileInputPolicy)

	uploadOnly := CurrentCapabilities(false, true, true, false, false, false, false, false, 0)
	require.Equal(t, []FileInputCapability{CapabilityRelayURL, CapabilityDataURI}, uploadOnly.SourceModes)
	require.Empty(t, uploadOnly.DownloadSinkModes)
}

// TestUploadToolsForRegistrationHonesty pins uploadToolsFor: a tool is listed
// only when wired AND the registration-time feature set declares its feature
// — never off the per-request wire profile.
func TestUploadToolsForRegistrationHonesty(t *testing.T) {
	require.Equal(t, []UploadToolCapability{UploadToolFile}, uploadToolsFor(httpFeatures(), true, false, false))

	grok := httpFeatures().Clone()
	grok[FeatSourceURL] = true
	grok[FeatSourceData] = true
	require.Equal(t, []UploadToolCapability{UploadToolFile, UploadToolURL, UploadToolData},
		uploadToolsFor(grok, true, true, true))

	// Relay wired at registration but the feature set that decided it never
	// declared the feature -> not claimed.
	require.Equal(t, []UploadToolCapability{UploadToolFile}, uploadToolsFor(httpFeatures(), true, true, true),
		"a server that registered no relay tools never advertises them")
}

// TestCapabilitiesDescriptorBakedDescription pins the tools/list description
// resolution: the mint completion contract renders only when the tool is
// wired; the file-first clause leaves when no file-capable tool is wired.
func TestCapabilitiesDescriptorBakedDescription(t *testing.T) {
	wired := CapabilityWiring{
		CoLocated: true, UploadFile: true, DownloadFile: true,
		DropWired: true, RelayFeatures: stdioFeatures(),
	}
	desc := NewCapabilitiesDescriptor(wired)
	require.Equal(t, "capabilities", desc.Name)
	require.Contains(t, desc.Description, "source_modes lists the source.mode values")
	// The mint completion contract is tool-scoped and respects registration
	// wiring: on this stdio profile (no mint source) no mint clause renders
	// and the vault tool's non-blocking mint contract never renders.
	require.NotContains(t, desc.Description, "upload_file(source.mode=mint) is asynchronous")
	require.NotContains(t, desc.Description, "vault_put_file(source.mode=mint",
		"an unwired vault tool's mint contract never renders")

	empty := NewCapabilitiesDescriptor(CapabilityWiring{})
	require.NotContains(t, empty.Description, "`file` argument when available",
		"no file-capable tool wired -> no file-first clause")
	require.NotContains(t, empty.Description, "or drop returns a one-time filedrop")
	require.Contains(t, empty.Description, "download_file/vault_get_file take a sink")
}

// TestCapabilitiesHandlerPerRequest pins the honest handler report: modes
// travel with wiring; draft/host-file flags are re-gated on the calling
// client; upload_tools mirrors registration.
func TestCapabilitiesHandlerPerRequest(t *testing.T) {
	wiring := CapabilityWiring{
		CoLocated:     true,
		UploadFile:    true,
		DownloadFile:  true,
		DropWired:     true,
		RelayFeatures: stdioFeatures(),
	}
	desc := NewCapabilitiesDescriptor(wiring)

	// Anonymous stdio request (Caps nil): stdio modes, drop sink, no host
	// file, registration-honest upload tools.
	res, err := desc.Handler(context.Background(), model.ToolRequest{})
	require.NoError(t, err)
	r, ok := res.StructuredContent.(CapabilityReport)
	require.True(t, ok)
	require.Contains(t, res.Text, `"transport":`)
	require.Contains(t, res.Text, `"source_modes":`)
	require.Equal(t, canimcp.TransportStdio, canimcp.TransportKind(r.Transport))
	require.Equal(t, []FileInputCapability{CapabilityLocalPath}, r.SourceModes)
	require.Equal(t, []FileOutputCapability{CapabilitySinkLocal, CapabilitySinkDrop}, r.DownloadSinkModes)
	require.Equal(t, []UploadToolCapability{UploadToolFile}, r.UploadTools)
	require.False(t, r.HostFileInput)
	require.Empty(t, r.FileInputPolicy)

	// A client that can build file objects: host_file_input flips on.
	res, err = desc.Handler(context.Background(), model.ToolRequest{
		Caps: &model.RequestCaps{Profile: modelProfileWith(FeatFileHostInput, canimcp.TransportHTTP)},
	})
	require.NoError(t, err)
	r2 := res.StructuredContent.(CapabilityReport)
	require.True(t, r2.HostFileInput)
	require.True(t, r2.HostFileInputPreferred)
	require.Equal(t, "host_file_first", r2.FileInputPolicy)

	// A Grok-like client (no file objects, no x-mcp-file): the draft flag is
	// clamped false even when the wiring registered an upload_data tool
	// elsewhere; upload_tools still mirrors registration, gated on the
	// registration-time feature set.
	grokWiring := wiring
	grokWiring.DataURIWired = true
	grokWiring.DraftXFile = true
	grokWiring.RelayFeatures = stdioFeatures().Clone()
	grokWiring.RelayFeatures[FeatSourceData] = true
	grok := modelProfileWith(FeatSourceURL, canimcp.TransportHTTP)
	grok.Features[model.Feature(FeatSourceData)] = true
	grokDesc := NewCapabilitiesDescriptor(grokWiring)
	res, err = grokDesc.Handler(context.Background(), model.ToolRequest{
		Caps: &model.RequestCaps{Profile: grok},
	})
	require.NoError(t, err)
	r3 := res.StructuredContent.(CapabilityReport)
	require.False(t, r3.DraftXFile, "a host without x-mcp-file must not read a draft it cannot speak")
	require.Equal(t, []UploadToolCapability{UploadToolFile, UploadToolData}, r3.UploadTools)
}

// modelProfileWith builds an mcpplane model.Profile fixture with a single
// capability feature over the given transport — the per-request wire shape
// handlers read.
func modelProfileWith(f mcpforge.Feature, tk canimcp.TransportKind) *model.Profile {
	p := &model.Profile{HostType: model.HostGeneric, Transport: model.TransportKind(tk), Features: model.FeatureSet{}}
	feats := mcpforge.FeatureSet{f: true}
	for k, v := range feats {
		p.Features[model.Feature(string(k))] = v
	}
	return p
}
