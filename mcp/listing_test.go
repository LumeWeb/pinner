package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/opmesh"
)

// The shared listing policy is a Pinner-owned product decision keyed on the
// canimcp host vocabulary: the browser-side web hosts (Claude Web, Grok Web,
// ChatGPT/OpenAI web) bypass progressive discovery with a flat tools/list
// policy; every other host — including each web host's co-located stdio
// sibling and every co-located agent — keeps progressive discovery. Canimcp
// carries none of this: no disclosure enum, no flat feature — it stays
// host-only.

func TestStrategyForHostWebHostsGoFlat(t *testing.T) {
	cases := []struct {
		name      string
		host      canimcp.HostType
		transport canimcp.TransportKind
	}{
		{"Claude Web over HTTP", canimcp.HostClaude, canimcp.TransportHTTP},
		{"Grok web connector over HTTP", canimcp.HostGrok, canimcp.TransportHTTP},
		{"OpenAI web (openai-mcp) over HTTP", canimcp.HostOpenAI, canimcp.TransportHTTP},
		{"ChatGPT over the embedded OpenAI tunnel", canimcp.HostChatGPT, canimcp.TransportOpenAI},
		{"Manufact Cloud dashboard over HTTP", canimcp.HostManufact, canimcp.TransportHTTP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, ListingFlat, StrategyForHost(tc.host, tc.transport),
				"web host must bypass progressive discovery with a flat listing")
			require.True(t, IsFlatListingHost(tc.host, tc.transport))
			require.Equal(t, ListingFlat, PolicyForHost(tc.host, tc.transport).Strategy)
		})
	}
}

func TestStrategyForHostOtherHostsStayProgressive(t *testing.T) {
	cases := []struct {
		name      string
		host      canimcp.HostType
		transport canimcp.TransportKind
	}{
		// Co-located agents keep progressive discovery regardless of family:
		// a host type must not leak its web connector's policy to its stdio
		// client, and vice versa.
		{"Grok Shell over stdio", canimcp.HostGrok, canimcp.TransportStdio},
		{"Claude over stdio (not the web client)", canimcp.HostClaude, canimcp.TransportStdio},
		{"Claude Code", canimcp.HostClaudeCode, canimcp.TransportStdio},
		{"Claude Desktop", canimcp.HostClaudeDesktop, canimcp.TransportStdio},
		{"Codex", canimcp.HostCodex, canimcp.TransportStdio},
		{"Generic stdio client", canimcp.HostGeneric, canimcp.TransportStdio},
		// Unidentified HTTP hosts keep progressive discovery: only the
		// DETECTED web hosts go flat.
		{"Generic HTTP client", canimcp.HostGeneric, canimcp.TransportHTTP},
		{"Unknown host over HTTP", canimcp.HostUnknown, canimcp.TransportHTTP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, ListingProgressive, StrategyForHost(tc.host, tc.transport),
				"non-web host must keep progressive discovery")
			require.False(t, IsFlatListingHost(tc.host, tc.transport))
			require.Equal(t, ListingProgressive, PolicyForHost(tc.host, tc.transport).Strategy)
		})
	}
}

func TestPolicyForHostKeepsSafeMetaDefault(t *testing.T) {
	// The host selector only decides the strategy; it never layers an
	// onboarding override, and the meta-on-flat switch stays at the safe
	// default (nil resolves true) so flat always keeps the discovery
	// meta-tools unless a consumer explicitly opts out.
	flat := PolicyForHost(canimcp.HostClaude, canimcp.TransportHTTP)
	require.Equal(t, ListingFlat, flat.Strategy)
	require.Nil(t, flat.IncludeMetaOnFlat)
	require.True(t, flat.ResolveIncludeMetaOnFlat(),
		"flat host policy keeps the discovery meta-tools (safe default)")
	require.Empty(t, flat.Onboarding,
		"the host selector must not set an onboarding override")

	prog := PolicyForHost(canimcp.HostCodex, canimcp.TransportStdio)
	require.Equal(t, ListingProgressive, prog.Strategy)
	require.Nil(t, prog.IncludeMetaOnFlat)
	require.True(t, prog.ResolveIncludeMetaOnFlat())
}

func TestDefaultPolicyIsProgressiveWithSafeMeta(t *testing.T) {
	p := DefaultPolicy()
	require.Equal(t, ListingProgressive, p.Strategy)
	require.Nil(t, p.IncludeMetaOnFlat)
	require.True(t, p.ResolveIncludeMetaOnFlat())
	require.NoError(t, p.Validate())
}

func TestListingPolicyValidateRejectsUnknownStrategy(t *testing.T) {
	bad := DefaultPolicy()
	bad.Strategy = ToolListingStrategy(99)
	require.Error(t, bad.Validate())
	require.Contains(t, bad.Validate().Error(), "unsupported listing strategy")
	require.Contains(t, bad.Strategy.String(), "<strategy 99>")
}

func TestAssembleResolvesListingPolicy(t *testing.T) {
	// nil (default): progressive.
	srv, err := Assemble(Config{Catalog: testCatalog()})
	require.NoError(t, err)
	require.Equal(t, ListingProgressive, srv.ListingPolicy().Strategy)
	require.True(t, srv.ListingPolicy().ResolveIncludeMetaOnFlat())

	// Explicit flat policy is adopted verbatim.
	flat := ListingPolicy{Strategy: ListingFlat}
	srv, err = Assemble(Config{Catalog: testCatalog(), Listing: &flat})
	require.NoError(t, err)
	require.Equal(t, ListingFlat, srv.ListingPolicy().Strategy)

	// An unsupported strategy fails assembly loudly.
	bad := ListingPolicy{Strategy: ToolListingStrategy(99)}
	_, err = Assemble(Config{Catalog: testCatalog(), Listing: &bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported listing strategy")
}

func TestFlatListingPromotesOnlyAgentSafeOperations(t *testing.T) {
	flat := ListingPolicy{Strategy: ListingFlat}
	cases := []struct {
		name        string
		category    string
		interaction opmesh.Interaction
		wantDirect  bool
	}{
		{name: "agent-safe core", category: string(model.CategoryCore), interaction: opmesh.InteractionAgentSafe, wantDirect: true},
		{name: "admin", category: string(model.CategoryAdmin), interaction: opmesh.InteractionAgentSafe},
		{name: "wizard", category: string(model.CategoryWizard), interaction: opmesh.InteractionAgentSafe},
		{name: "human-only", category: string(model.CategoryCore), interaction: opmesh.InteractionHumanOnly},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := catalogDescriptorToPresentation(opmesh.ToolDescriptor{
				Name:        tc.name,
				Category:    tc.category,
				Interaction: tc.interaction,
			}, flat)
			require.Equal(t, tc.wantDirect, got.DirectVisible)
		})
	}

	progressive := catalogDescriptorToPresentation(opmesh.ToolDescriptor{
		Name:        "progressive",
		Category:    string(model.CategoryCore),
		Interaction: opmesh.InteractionAgentSafe,
	}, DefaultPolicy())
	require.False(t, progressive.DirectVisible)
}
