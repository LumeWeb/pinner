package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/pinner/assembly"
)

// The open_app prose is inventory-gated, not capability-gated: a host that
// signals FeatMCPApps but whose composition root registered no app views
// (Config.InstalledApps empty — e.g. a hosted deployment without an apps
// registry) must get no open_app claim anywhere in the guide, and a host
// with a partial inventory must name only the launchers that registered.

// guideAllText joins every text surface the resolved guide carries (summary,
// rules, flow details, decision-branch details) so the open_app claims can be
// scanned in full, un-escaped.
func guideAllText(t *testing.T, g AgentGuide) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(g.Summary)
	for _, r := range g.Rules {
		b.WriteString(r)
	}
	for _, f := range g.Flows {
		b.WriteString(f.Detail)
		if f.Decision != nil {
			writeDecisionText(&b, *f.Decision)
		}
	}
	return b.String()
}

func writeDecisionText(b *strings.Builder, d GuideDecision) {
	for _, br := range d.Branches {
		b.WriteString(br.Detail)
		if br.Next != nil {
			writeDecisionText(b, *br.Next)
		}
	}
}

// TestAgentGuideOpenAppAbsentWithoutInventory pins the drift fix: the
// capability signal alone (FeatMCPApps on every GUI profile) is no longer
// sufficient to make the guide mention open_app — the apps-registry-less
// composition root (nil inventory) yields a guide with zero open_app claims.
func TestAgentGuideOpenAppAbsentWithoutInventory(t *testing.T) {
	for name, profile := range map[string]HostProfile{
		"openai_tunnel": openAITunnelProfile(),
		"claude_http":   claudeHTTPProfile(),
		"stdio_apps":    stdioAppsProfile(),
	} {
		t.Run(name, func(t *testing.T) {
			guide := BuildAgentGuide(profile, assembly.FullDomainScope, false, nil)
			require.NotContains(t, guideAllText(t, guide), "open_app",
				"guide must not claim open_app when no app views are registered")
		})
	}
}

// TestAgentGuideOpenAppEnumeratesInventoryOnly pins the enumeration: the
// open_app summary/rule copy lists exactly the registered launchers, a
// per-app flow detail appears only for a registered launcher, and an
// unregistered app name never appears.
func TestAgentGuideOpenAppEnumeratesInventoryOnly(t *testing.T) {
	profile := claudeHTTPProfile()
	inventory := []string{"sso_signin", "vault_create"}
	guide := BuildAgentGuide(profile, assembly.FullDomainScope, false, inventory)
	text := guideAllText(t, guide)

	require.Contains(t, text, "open_app",
		"registered inventory must restore the open_app guidance")
	require.Contains(t, text, "sso_signin, vault_create",
		"{{APPS}} must enumerate exactly the registered launchers")
	require.Contains(t, text, `app="sso_signin"`,
		"the auth flow detail appears when its launcher registered")
	require.Contains(t, text, `app="vault_create"`,
		"the vault_create flow detail appears when its launcher registered")
	require.NotContains(t, text, `app="vault_restore"`,
		"the vault_restore flow detail stays absent when its launcher did not register")
	require.NotContains(t, text, "pin_creator",
		"an unregistered app name must never appear in the guide")

	// Also reachable through the descriptor's per-request path: the handler
	// re-applies the construction-time inventory over whatever wire profile
	// arrives (here a Claude-Web-style profile that only signals the
	// capability, no inventory of its own).
	wireProfile := &model.Profile{
		HostType:  model.HostType(canimcp.HostClaude),
		Transport: model.TransportKind(canimcp.TransportHTTP),
		Features:  model.FeatureSet{model.Feature(string(FeatMCPApps)): true, model.Feature(string(FeatSourceData)): true},
	}
	desc := AgentGuideDescriptor(assembly.FullDomainScope, false, true, inventory)
	result, err := desc.Handler(nil, model.ToolRequest{Caps: &model.RequestCaps{Profile: wireProfile}})
	require.NoError(t, err)
	require.Contains(t, result.Text, "sso_signin, vault_create")
}

// TestAgentGuideOpenAppWithoutCapabilitySignal pins the host-capability half
// of the gate: an inventory without the host's FeatMCPApps signal (a
// launcher registered on an agent-only host) also yields no open_app prose —
// the guide composes capability AND installed, matching the wire surface.
func TestAgentGuideOpenAppWithoutCapabilitySignal(t *testing.T) {
	profile := profileForTransport(canimcp.TransportHTTP)
	guide := BuildAgentGuide(profile, assembly.FullDomainScope, false, []string{"pin_list"})
	require.NotContains(t, guideAllText(t, guide), "open_app",
		"an agent-only host gets no open_app prose even with an inventory")
}

// TestAgentGuideOpenAppRulePrefixAddedOnlyWithApps verifies the rule list
// carries the guide rule (not just summary prose) exactly when a host both
// signals the capability and holds an inventory.
func TestAgentGuideOpenAppRulePrefixAddedOnlyWithApps(t *testing.T) {
	rules := func(profile HostProfile, apps []string) string {
		return strings.Join(BuildAgentGuide(profile, assembly.FullDomainScope, false, apps).Rules, "\n")
	}
	require.Contains(t, rules(claudeHTTPProfile(), []string{"pin_list"}), "MCP Apps rule",
		"capability + inventory adds the open_app rule")
	require.NotContains(t, rules(claudeHTTPProfile(), nil), "MCP Apps",
		"capability without inventory adds no open_app rule")
	require.NotContains(t, rules(profileForTransport(canimcp.TransportHTTP), []string{"pin_list"}), "MCP Apps",
		"inventory without capability adds no open_app rule")
}
