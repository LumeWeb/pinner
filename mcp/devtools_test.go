package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/opmesh"
)

// devCaps builds a RequestCaps carrying a resolved HTTP profile plus a raw wire
// snapshot, mirroring what profile-detecting request glue produces when dev
// tools are enabled.
func devCaps() *model.RequestCaps {
	p := &model.Profile{
		HostType:    model.HostGrok,
		Transport:   model.TransportHTTP,
		AuthMethod:  model.AuthBearer,
		Remote:      true,
		Features:    model.FeatureSet{model.FeatSourceMint: true, model.FeatSinkDrop: true},
		ClientInfo:  &model.ClientInfo{Name: "grok-client", Version: "1.2.3"},
		ProtocolVer: "2025-03-26",
		UserAgent:   "grok-client/1.2.3",
		Headers:     http.Header{"User-Agent": []string{"grok-client/1.2.3"}},
		TokenInfo:   &model.TokenInfo{UserID: "u-42", Scopes: []string{"pins:read"}},
	}
	return &model.RequestCaps{
		ProtocolVersion:  "2025-03-26",
		ClientName:       "grok-client",
		Capabilities:     map[string]any{"roots": map[string]any{"listChanged": true}},
		InitializeParams: map[string]any{"protocolVersion": "2025-03-26"},
		Profile:          p,
	}
}

func devReq(name string, args map[string]any, caps *model.RequestCaps) model.ToolRequest {
	return model.ToolRequest{Name: name, Arguments: args, Caps: caps}
}

// --- Handler-level characterization (ported from pinner-cli) ---

func TestDevDescriptorsAreReadOnlyAndDirectVisible(t *testing.T) {
	descs := devToolDescriptors()
	require.Len(t, descs, 3)
	for _, d := range descs {
		require.True(t, d.ReadOnly, "%s must be read-only", d.Name)
		require.True(t, d.DirectVisible, "%s must be directly visible", d.Name)
		require.NotNil(t, d.Handler, "%s must carry a handler", d.Name)
	}
	require.ElementsMatch(t,
		[]string{"dev_host_env", "dev_profile", "dev_request"},
		[]string{descs[0].Name, descs[1].Name, descs[2].Name})
}

func TestDevDescriptorsCarryObjectSchemas(t *testing.T) {
	for _, d := range devToolDescriptors() {
		var schema map[string]any
		require.NoError(t, json.Unmarshal(d.InputSchema, &schema), d.Name)
		require.Equal(t, "object", schema["type"], d.Name)
	}
}

func TestDevHostEnvHandlerReportsHost(t *testing.T) {
	res, err := devHostEnvHandler(context.Background(), devReq("dev_host_env", nil, devCaps()))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.NotEmpty(t, res.Text, "text+structured: text-only hosts must see the data too")

	out, ok := res.StructuredContent.(*devHostEnvOutput)
	require.True(t, ok, "structured content is a typed *devHostEnvOutput, got %T", res.StructuredContent)
	require.Equal(t, "grok", out.HostType)
	require.Equal(t, "http", out.Transport)
	require.Equal(t, "bearer", out.AuthMethod)
	require.True(t, out.Remote)
	require.Equal(t, "grok-client", out.ClientInfo.Name)
	require.Equal(t, "1.2.3", out.ClientInfo.Version)
	require.Equal(t, "2025-03-26", out.ProtocolVersion)
	require.Equal(t, "grok-client/1.2.3", out.UserAgent.Raw)
	require.Equal(t, []string{"grok-client/1.2.3"}, out.UserAgent.Values)
	// Raw wire snapshot present under dev tools.
	require.NotEmpty(t, out.ClientCapabilities)
	require.NotEmpty(t, out.InitializeParams)
	// Features are listed, sorted, and enabled-only.
	require.Equal(t, []string{"sink-drop", "source-mint"}, out.Features)
	// Redacted auth state: token presence only — the token material itself
	// (user ID, scopes, claims) must never appear in the dump.
	require.True(t, out.TokenPresent)
	require.NotContains(t, res.Text, "u-42")
	require.NotContains(t, res.Text, "pins:read")
	require.NotContains(t, res.Text, "oauth_token")
}

// TestDevHostEnvOmitsTokenMaterial pins the security contract: the auth dump
// is a redacted state (auth method + token presence), never the OAuth token
// info itself — echoing token claims into the result puts authentication
// material into the host's persisted conversation logs.
func TestDevHostEnvOmitsTokenMaterial(t *testing.T) {
	caps := devCaps()
	caps.Profile.TokenInfo = &model.TokenInfo{
		UserID: "u-42",
		Scopes: []string{"pins:read", "pins:write"},
		Extra:  map[string]any{"email": "secret@example.com"},
	}
	res, err := devHostEnvHandler(context.Background(), devReq("dev_host_env", nil, caps))
	require.NoError(t, err)

	out := res.StructuredContent.(*devHostEnvOutput)
	require.True(t, out.TokenPresent, "token presence is the redacted auth state")
	require.Equal(t, "bearer", out.AuthMethod)
	for _, secret := range []string{"u-42", "pins:read", "pins:write", "secret@example.com"} {
		require.NotContains(t, res.Text, secret)
	}
}

func TestDevHostEnvHandlerNilsAreSafe(t *testing.T) {
	// A request with no caps must not panic and must still classify via the
	// stdio-generic fallback profile.
	res, err := devHostEnvHandler(context.Background(), devReq("dev_host_env", nil, nil))
	require.NoError(t, err)
	require.False(t, res.IsError)
	out, ok := res.StructuredContent.(*devHostEnvOutput)
	require.True(t, ok)
	require.Equal(t, "stdio", out.Transport)
	require.Equal(t, "generic", out.HostType)
	require.Nil(t, out.UserAgent)
	require.Nil(t, out.ClientCapabilities)
	require.Nil(t, out.InitializeParams)
}

// TestDevHostEnvRedactsCredentialHeaders pins the security contract: the
// header dump masks every header value not explicitly allowlisted (an
// allowlist, not a credential deny-list, so newly invented credential header
// names are masked by default) — a dev tool whose plain-text result echoes an
// Authorization header lands the bearer token in the host's persisted
// conversation logs. Header names keep their presence so the debug signal
// survives; only values are masked.
func TestDevHostEnvRedactsCredentialHeaders(t *testing.T) {
	secret := "secret-bearer-token-do-not-leak"
	caps := devCaps()
	caps.Profile.Headers = http.Header{
		"User-Agent":           []string{"grok-client/1.2.3"},
		"Authorization":        []string{"Bearer " + secret},
		"Proxy-Authorization":  []string{"Bearer " + secret},
		"Cookie":               []string{"session=" + secret},
		"X-Api-Key":            []string{secret},
		"X-Auth-Token":         []string{secret},
		"X-Goog-Api-Key":       []string{secret},
		"X-Amz-Security-Token": []string{secret},
		"Accept":               []string{"application/json"},
	}
	res, err := devHostEnvHandler(context.Background(), devReq("dev_host_env", nil, caps))
	require.NoError(t, err)

	out := res.StructuredContent.(*devHostEnvOutput)
	for _, h := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-Api-Key", "X-Auth-Token", "X-Goog-Api-Key", "X-Amz-Security-Token"} {
		require.Equal(t, []string{"[redacted]"}, out.HTTPHeaders[h], h)
	}
	// Non-sensitive headers keep their values; the UA multi-value detail that
	// dev_user_agent_from relies on is untouched.
	require.Equal(t, []string{"grok-client/1.2.3"}, out.HTTPHeaders["User-Agent"])
	require.Equal(t, []string{"application/json"}, out.HTTPHeaders["Accept"])

	// The plain-text form must be just as safe: no credential value anywhere.
	require.NotContains(t, res.Text, secret)
	require.Contains(t, res.Text, "[redacted]")
}

func TestDevProfileHandlerReportsClassification(t *testing.T) {
	res, err := devProfileHandler(context.Background(), devReq("dev_profile", nil, devCaps()))
	require.NoError(t, err)
	out, ok := res.StructuredContent.(*devProfileOutput)
	require.True(t, ok, "structured content is a typed *devProfileOutput, got %T", res.StructuredContent)
	require.Equal(t, "grok", out.HostType)
	require.Equal(t, "http", out.Transport)
	require.True(t, out.Remote)
	require.Equal(t, "grok-client", out.ClientInfo.Name)
	require.Equal(t, "grok-client/1.2.3", out.UserAgent.Raw)
	require.True(t, out.TokenPresent)
	require.Contains(t, out.Features, "source-mint")
}

// TestDevProfileProtocolVersionSingleSource pins the sibling-agreement
// contract: dev_profile reports the negotiated protocol version from the same
// caps source dev_host_env uses, falling back to the profile only when the
// caps source is empty — the two tools cannot disagree for the same call.
func TestDevProfileProtocolVersionSingleSource(t *testing.T) {
	// Caps protocol version wins when present.
	res, err := devProfileHandler(context.Background(), devReq("dev_profile", nil, devCaps()))
	require.NoError(t, err)
	out := res.StructuredContent.(*devProfileOutput)
	require.Equal(t, "2025-03-26", out.ProtocolVersion)

	// Empty caps version falls back to the profile's wire value.
	caps := devCaps()
	caps.ProtocolVersion = ""
	res, err = devProfileHandler(context.Background(), devReq("dev_profile", nil, caps))
	require.NoError(t, err)
	out = res.StructuredContent.(*devProfileOutput)
	require.Equal(t, "2025-03-26", out.ProtocolVersion)
}

func TestDevRequestHandlerEchoesInvocation(t *testing.T) {
	args := map[string]any{"page": 1}
	res, err := devRequestHandler(context.Background(), devReq("dev_request", args, devCaps()))
	require.NoError(t, err)
	out, ok := res.StructuredContent.(*devRequestOutput)
	require.True(t, ok, "structured content is a typed *devRequestOutput, got %T", res.StructuredContent)
	require.Equal(t, "dev_request", out.Tool)
	require.Equal(t, args, out.Arguments)
	require.Equal(t, "2025-03-26", out.ProtocolVersion)
	require.False(t, out.InputResponses)
}

func TestDevRequestHandlerNilCapsSafe(t *testing.T) {
	res, err := devRequestHandler(context.Background(), devReq("dev_request", nil, nil))
	require.NoError(t, err)
	require.False(t, res.IsError)
}

// --- Assembly-level contract (the port's raison d'être) ---

// TestAssembleDevToolsGate pins the Config.DevTools contract: the dev_*
// introspection tools must NEVER be on the production surface (default off),
// and when enabled they land on Direct as read-only, directly-visible
// descriptors that can be registered on tools/list as-is.
func TestAssembleDevToolsGate(t *testing.T) {
	// Default: absent.
	srv, err := Assemble(Config{Catalog: testCatalog()})
	require.NoError(t, err)
	_, present := directTool(t, srv, "dev_host_env")
	require.False(t, present, "production surface must never carry dev tools")
	require.False(t, srv.DevEnabled())

	// Enabled: all three on Direct, direct-only descriptors.
	srv, err = Assemble(Config{Catalog: testCatalog(), DevTools: true})
	require.NoError(t, err)
	require.True(t, srv.DevEnabled())
	for _, name := range []string{"dev_host_env", "dev_profile", "dev_request"} {
		d, ok := directTool(t, srv, name)
		require.True(t, ok, "%s must be registered when DevTools is enabled", name)
		require.True(t, d.DirectVisible)
		require.True(t, d.ReadOnly)
		require.Equal(t, model.CategoryCore, d.Category)
	}
	// The dev tools must be additive: agent_guide and capabilities stay.
	_, ok := directTool(t, srv, "agent_guide")
	require.True(t, ok)
}

// TestDevToolsAlsoOnEmptyCatalogScope asserts dev tools assemble even for a
// bare-Hosted hosted composition root whose catalog is empty — the debug
// surface must not depend on any domain scope.
func TestDevToolsOnHostedAssembly(t *testing.T) {
	srv, err := Assemble(Config{Catalog: opmesh.NewCatalog(), Hosted: true, DevTools: true})
	require.NoError(t, err)
	_, ok := directTool(t, srv, "dev_profile")
	require.True(t, ok)
}

// TestDevHandlersDispatchThroughAssembledDirect end-to-end: invoke a dev tool
// through its assembled descriptor handler with a caps-bearing request, the
// way a composition root registering srv.Direct on the wire would.
func TestDevHandlersDispatchThroughAssembledDirect(t *testing.T) {
	srv, err := Assemble(Config{Catalog: testCatalog(), DevTools: true})
	require.NoError(t, err)
	d, ok := directTool(t, srv, "dev_request")
	require.True(t, ok)

	res, err := d.Handler(context.Background(), model.ToolRequest{Caps: nil})
	require.NoError(t, err)
	require.False(t, res.IsError)
	out, ok := res.StructuredContent.(*devRequestOutput)
	require.True(t, ok)
	require.Empty(t, out.Tool)
	require.Empty(t, out.ProtocolVersion)
}
