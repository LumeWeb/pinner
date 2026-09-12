package catalogmcp

// Characterization tests for the hosted-plugin subscription-promotion gate on
// the account descriptions. Platform commerce policy forbids hosted agent
// surfaces from displaying subscription plans, initiating subscriptions, or
// promoting upgrades: only an entitlement explanation is permitted there. The
// non-hosted wording (web_url deep-link guidance) is the documented,
// unchanged UX for CLI/local surfaces.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpforge"
)

// hostedAccountProfile returns the MCPProfile equivalents a hosted (plugin)
// assembly compiles against: the hosting layer overlays FeatHosted on the
// profile feature set when it assembles with hosted=true.
func accountProfile(hosted bool) MCPProfile {
	fs := mcpforge.FeatureSet{}
	if hosted {
		fs[FeatHosted] = true
	}
	return MCPProfile{Features: fs}
}

// TestAccountQuotaDescriptionGatesSubscriptionPromotion pins the per-profile
// resolution of the account_quota description: hosted resolves with no
// subscription deep-link or subscribe prompting; the profile-less default
// (and every non-hosted profile) resolves the unchanged deep-link guidance.
func TestAccountQuotaDescriptionGatesSubscriptionPromotion(t *testing.T) {
	hosted := accountQuotaDesc.Resolve(accountProfile(true))
	require.NotContains(t, hosted, "web_url", "hosted description must not promise a subscription deep-link")
	require.NotContains(t, hosted, "deep-link")
	require.NotContains(t, hosted, "to subscribe", "hosted description must not promote subscribing")
	require.Contains(t, hosted, "account_subscription",
		"the entitlement explanation may still point at the informational status tool")
	require.NotContains(t, hosted, "never offer",
		"descriptions describe the tool; response-policy directives do not belong here")
	require.Contains(t, hosted, "carries no subscription state",
		"hosted wording states the factual limitation of the returned result")

	local := accountQuotaDesc.Resolve(accountProfile(false))
	require.Contains(t, local, "web_url deep-link",
		"non-hosted description keeps the documented deep-link guidance")

	profileLess := accountQuotaDesc.Resolve(MCPProfile{})
	require.Equal(t, local, profileLess,
		"a nil/featureless profile resolves as non-hosted (the safe default)")
}

// TestAccountSubscriptionDescriptionGatesSubscriptionPromotion pins the
// per-profile resolution of the account_subscription description with the
// same hosted/non-hosted split.
func TestAccountSubscriptionDescriptionGatesSubscriptionPromotion(t *testing.T) {
	hosted := accountSubscriptionDesc.Resolve(accountProfile(true))
	require.NotContains(t, hosted, "web_url", "hosted description must not promise a deep-link")
	require.NotContains(t, hosted, "deep-link")
	require.NotContains(t, hosted, "to subscribe", "hosted description must not promote subscribing")
	require.Contains(t, hosted, "Purely informational")
	require.NotContains(t, hosted, "may be surfaced",
		"hosted wording stays factual about the response, not directive about the model")

	local := accountSubscriptionDesc.Resolve(accountProfile(false))
	require.Contains(t, local, "web_url field",
		"non-hosted description keeps the documented deep-link guidance")
	require.Contains(t, local, "cannot subscribe on the user's behalf")
}

// TestAccountTargetsResolveViaFallbackFunc pins that the opTargets wiring for
// the two subscription-related account operations routes through the
// description DSL (FallbackFunc), so the compiled MCP surface resolves the
// hosted variant against the compiled profile and never serves the
// non-hosted deep-link wording on a hosted assembly.
func TestAccountTargetsResolveViaFallbackFunc(t *testing.T) {
	for _, tc := range []struct {
		opID     string
		targets  []Target
		hosted   bool
		contains string
		notWant  string
	}{
		{"account_quota", TargetsOf("account_quota"), true, "account_subscription", "web_url"},
		{"account_quota", TargetsOf("account_quota"), false, "web_url deep-link", ""},
		{"account_subscription", TargetsOf("account_subscription"), true, "Purely informational", "web_url"},
		{"account_subscription", TargetsOf("account_subscription"), false, "web_url field", ""},
	} {
		t.Run(tc.opID, func(t *testing.T) {
			var fallback *Target
			for i := range tc.targets {
				if len(tc.targets[i].Require) == 0 && tc.targets[i].Visible {
					fallback = &tc.targets[i]
					break
				}
			}
			require.NotNil(t, fallback, "targets must carry a visible Require-less fallback")

			require.NotNil(t, fallback.DescFunc, "%s must resolve via the description DSL", tc.opID)
			desc := fallback.DescFunc(accountProfile(tc.hosted))

			require.True(t, strings.Contains(desc, tc.contains),
				"hosted=%v: resolved description %q must contain %q", tc.hosted, desc, tc.contains)
			if tc.notWant != "" {
				require.NotContains(t, desc, tc.notWant,
					"hosted=%v: resolved description must not contain %q", tc.hosted, tc.notWant)
			}
		})
	}
}
