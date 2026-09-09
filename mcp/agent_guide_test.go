package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/pinner/assembly"
)

// Characterization tests for the agent guide. The guide is host-aware and
// re-resolved per request; the surface and hosted flags (Config fields) are
// overlaid onto the request profile.

func guideFlowByName(t *testing.T, guide AgentGuide, name string) GuideFlow {
	t.Helper()
	for _, f := range guide.Flows {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("guide has no flow named %q", name)
	return GuideFlow{}
}

// TestAgentGuideDescriptorFullSurface pins the full-surface guide:
// the full surface carries all 13 flows, sane structure, clean serialization.
func TestAgentGuideDescriptorFullSurface(t *testing.T) {
	desc := AgentGuideDescriptor(assembly.FullSurface, false, true)
	require.Equal(t, "agent_guide", desc.Name)
	require.EqualValues(t, model.CategoryCore, desc.Category)

	res, err := desc.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{}})
	require.NoError(t, err)
	require.NotNil(t, res.StructuredContent)

	guid, ok := res.StructuredContent.(AgentGuide)
	require.True(t, ok, "StructuredContent must be an AgentGuide")
	require.NotEmpty(t, guid.Summary)
	require.Len(t, guid.Flows, 13, "the full surface must cover all primary flows")

	names := make([]string, 0, len(guid.Flows))
	for _, f := range guid.Flows {
		names = append(names, f.Name)
		require.NotEmpty(t, f.Title)
		if f.Decision != nil {
			require.NotEmpty(t, f.Decision.Question)
			require.NotEmpty(t, f.Decision.Branches)
			for _, b := range f.Decision.Branches {
				require.NotEmpty(t, b.When)
				require.NotEmpty(t, b.Steps, "each decision branch must list at least one step: %s/%s", f.Name, b.When)
			}
		} else {
			require.GreaterOrEqual(t, len(f.Steps), 2, "each flat flow must list an ordered tool chain: %s", f.Name)
		}
	}
	for _, want := range []string{"auth", "vault_create", "vault_restore", "upload", "vault_upload", "download", "vault_download", "vault_share", "vault_sync", "pins", "publish_website", "update_website", "ens_publish"} {
		require.Contains(t, names, want)
	}

	// Serializes cleanly (structured content reaches the wire as JSON).
	_, err = json.Marshal(guid)
	require.NoError(t, err)
}

// TestAgentGuideHostedSurfaceDropsVaultFlows pins the surface gating.
func TestAgentGuideHostedSurfaceDropsVaultFlows(t *testing.T) {
	guid := BuildAgentGuide(profileForTransport(canimcp.TransportHTTP), assembly.HostedSurface, true)
	for _, f := range guid.Flows {
		require.NotContains(t, []string{"vault_create", "vault_restore", "vault_upload", "vault_download", "vault_share", "vault_sync"}, f.Name,
			"the hosted surface must never advertise a vault flow")
	}
	// Hosted notice renders.
	require.Contains(t, strings.Join(guid.Rules, "\n"), "Hosted instance notice")
	// Synthetic restricted surface keeps non-vault flows.
	for _, want := range []string{"auth", "upload", "download", "pins", "publish_website", "update_website", "ens_publish"} {
		require.Contains(t, flowNames(guid), want)
	}
}

func flowNames(g AgentGuide) []string {
	out := make([]string, 0, len(g.Flows))
	for _, f := range g.Flows {
		out = append(out, f.Name)
	}
	return out
}

// TestAgentGuideModesMatchProfile guards against advertising source modes the
// resolved profile's transport cannot serve (the Kody regression the source
// pinned).
func TestAgentGuideModesMatchProfile(t *testing.T) {
	cases := []struct {
		name     string
		profile  HostProfile
		mustHave []string
		mustNot  []string
	}{
		{name: "stdio generic advertises only path",
			profile:  profileForTransport(canimcp.TransportStdio),
			mustHave: []string{"source.mode=path"},
			mustNot:  []string{"source.mode=mint", "source.mode=url/data"}},
		{name: "http generic advertises only mint",
			profile:  profileForTransport(canimcp.TransportHTTP),
			mustHave: []string{"source.mode=mint"},
			mustNot:  []string{"source.mode=path", "source.mode=url/data"}},
		{name: "openai tunnel advertises url/data fallback",
			profile:  openAITunnelProfile(),
			mustHave: []string{"source.mode=url/data"},
			mustNot:  []string{"source.mode=path", "source.mode=mint"}},
		{name: "grok http advertises only mint",
			profile:  grokHTTPProfile(),
			mustHave: []string{"source.mode=mint"},
			mustNot:  []string{"source.mode=path", "source.mode=url/data"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guide := BuildAgentGuide(tc.profile, assembly.FullSurface, false)
			for _, name := range []string{"upload", "vault_upload"} {
				flow := guideFlowByName(t, guide, name)
				for _, want := range tc.mustHave {
					require.Contains(t, flow.Detail, want, "%s detail must advertise %s", name, want)
				}
				for _, not := range tc.mustNot {
					require.NotContains(t, flow.Detail, not, "%s detail must NOT advertise %s", name, not)
				}
			}
			pub := guideFlowByName(t, guide, "publish_website")
			require.NotNil(t, pub.Decision, "publish_website must be a decision flow")
		})
	}
}

// TestAgentGuideClaudeWebNoticeScoped mirrors the hosted-scoping pin of the
// Claude Web no-curl notice.
func TestAgentGuideClaudeWebNoticeScoped(t *testing.T) {
	web := BuildAgentGuide(claudeHTTPProfile(), assembly.FullSurface, false)
	rules := strings.Join(web.Rules, "\n")
	require.Contains(t, rules, "Host capability notice (Claude Web)")
	require.Contains(t, rules, "upload_data")

	desktop := BuildAgentGuide(stdioAppsProfile(), assembly.FullSurface, false)
	require.NotContains(t, strings.Join(desktop.Rules, "\n"), "Host capability notice (Claude Web)")

	generic := BuildAgentGuide(profileForTransport(canimcp.TransportHTTP), assembly.FullSurface, false)
	require.NotContains(t, strings.Join(generic.Rules, "\n"), "Host capability notice (Claude Web)")

	// Hosted Claude Web is NOT special-cased: no notice, generic mint guidance.
	hostedWeb := BuildAgentGuide(claudeHTTPProfile(), assembly.FullSurface, true)
	require.NotContains(t, strings.Join(hostedWeb.Rules, "\n"), "Host capability notice (Claude Web)")
	upload := guideFlowByName(t, hostedWeb, "upload")
	require.Contains(t, upload.Detail, "curl -sS -T")
	require.Contains(t, upload.Detail, "poll upload_status")
	download := guideFlowByName(t, hostedWeb, "download")
	require.Contains(t, download.Detail, "Prefer sink=drop")
	require.Contains(t, download.Detail, "curl -o")
}

// TestAgentGuidePublishChainContainsRealTools pins the publish/ens decision
// chains: every branch ends at real tools, byte route first.
func TestAgentGuidePublishChainContainsRealTools(t *testing.T) {
	guid := BuildAgentGuide(profileForTransport(canimcp.TransportHTTP), assembly.FullSurface, false)
	pub := guideFlowByName(t, guid, "publish_website")
	require.NotNil(t, pub.Decision)
	require.Equal(t, "Where are the bytes?", pub.Decision.Question,
		"publish_website's top decision is the byte route")
	for _, b := range pub.Decision.Branches {
		require.Equal(t, "Does the user have a domain or subdomain label preference?", b.Next.Question,
			"every byte-route branch chains the domain/deployment decision")
	}
	ens := guideFlowByName(t, guid, "ens_publish")
	require.NotNil(t, ens.Decision)
	for _, b := range pub.Decision.Branches {
		require.Contains(t, flowSteps(t, b.Next), "websites_create",
			"each publish_website branch must reach websites_create after the byte route")
	}
	// The ENS flow's pointed branch resolves to ens_point (progressive
	// disclosure; the guide names it so the agent searches for it).
	require.Contains(t, flowSteps(t, ens.Decision), "ens_point")
}

// flowSteps collects every step in a decision tree: the branches' steps plus
// their nested decisions.
func flowSteps(t *testing.T, d *GuideDecision) []string {
	t.Helper()
	if d == nil {
		return nil
	}
	var out []string
	var walk func(*GuideDecision)
	walk = func(dd *GuideDecision) {
		if dd == nil {
			return
		}
		for _, b := range dd.Branches {
			out = append(out, b.Steps...)
			walk(b.Next)
		}
	}
	walk(d)
	return out
}

// TestAgentGuideFileHandoffMutuallyExclusive mirrors the segment-level
// mutual-exclusion guarantee: for file-host-input profiles exactly one of the
// host-file / convert-source clauses is active, per profile.
func TestAgentGuideFileHandoffMutuallyExclusive(t *testing.T) {
	handoffMarker := "host file argument"
	convertMarker := "with a convert source"

	for _, p := range []HostProfile{openAITunnelProfile(), profileForTransport(canimcp.TransportHTTP).SetFeature(FeatFileHostInput)} {
		segs := uploadDetailDesc.Clone().ResolveSegments(p)
		require.True(t, segHasToken(segs, handoffMarker), "file-host-input profile must activate the host-file clause")
		require.False(t, segHasToken(segs, convertMarker), "file-host-input profile must not also activate the convert-source clause")
	}
	for _, p := range []HostProfile{profileForTransport(canimcp.TransportStdio), grokHTTPProfile()} {
		segs := uploadDetailDesc.Clone().ResolveSegments(p)
		require.True(t, segHasToken(segs, convertMarker), "non-file-host-input profile must activate the convert-source clause")
		require.False(t, segHasToken(segs, handoffMarker), "non-file-host-input profile must not activate the host-file clause")
	}
}

func segHasToken(segs []string, tok string) bool {
	for _, s := range segs {
		if strings.Contains(s, tok) {
			return true
		}
	}
	return false
}

// TestAgentGuideSubstitution pins the {{SOURCES}} interpolation: the profile's
// transport modes are substituted, never a generic enumeration.
func TestAgentGuideSubstitution(t *testing.T) {
	guid := BuildAgentGuide(profileForTransport(canimcp.TransportHTTP), assembly.FullSurface, false)
	require.Contains(t, guid.Summary, "source.mode=mint")
	require.NotContains(t, guid.Summary, "{{SOURCES}}")
	require.NotContains(t, guid.Summary, "source.mode=path/mint/url/data")
}

// ---
// Host-profile fixtures with the platform profile's static declarations
// (mechanism features composed from the transport; per-host capability caps
// declared on top). These exist so the hosted profiles' behavior is
// characterized with the exact feature sets the transports serve.
// ---

func openAITunnelProfile() HostProfile {
	p := profileForTransport(canimcp.TransportOpenAI)
	p.Host = canimcp.HostChatGPT
	p.Features[FeatFileHostInput] = true
	p.Features[FeatXMcpFile] = true
	p.Features[FeatMCPApps] = true
	p.Features[FeatElicitation] = true
	return p
}

func grokHTTPProfile() HostProfile {
	p := profileForTransport(canimcp.TransportHTTP)
	p.Host = canimcp.HostGrok
	p.Features[FeatSourceData] = true
	p.Features[FeatSourceURL] = true
	return p
}

func claudeHTTPProfile() HostProfile {
	p := profileForTransport(canimcp.TransportHTTP)
	p.Host = canimcp.HostClaude
	p.Features[FeatSourceData] = true
	p.Features[FeatMCPApps] = true
	return p
}

func stdioAppsProfile() HostProfile {
	p := profileForTransport(canimcp.TransportStdio)
	p.Host = canimcp.HostStdioApps
	p.Features[FeatMCPApps] = true
	return p
}
