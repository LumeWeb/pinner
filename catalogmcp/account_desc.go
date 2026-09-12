package catalogmcp

import (
	"go.lumeweb.com/mcpforge"
)

// A hosted assembly is a Portal-embedded MCP deployment (mcp.Config.Hosted)
// that serves plugin hosts (e.g. the ChatGPT plugin). Platform commerce
// policy forbids such agent surfaces from displaying subscription plans,
// initiating subscriptions, promoting upgrades, or linking to any
// checkout/transaction flow. Explanations of entitlement state are the ONLY
// permitted communication. Non-hosted assemblies (CLI, local MCP) are free to
// surface the full subscription deep-link UX.
//
// The deployment fact travels through the description boundary as a feature
// (not a profile field): catalogmcp resolves descriptions exclusively against
// mcpforge.FeatureSets, so the hosting layer overlays FeatHosted on the
// compiled profile when (and only when) it assembles with hosted=true. A
// profile without the feature — including the intentional nil/profile-less
// configuration — resolves as non-hosted, which is the safe default: the
// full deep-link wording is legal on self-hosted surfaces.

// FeatHosted marks profiles compiled from a hosted (plugin) assembly. It is
// part of this package's description vocabulary (see profileFeatures) and is
// overlaid by the hosting layer at assembly time.
const FeatHosted = mcpforge.Feature("hosted")

// accountQuotaDesc is the per-profile MCP description for account_quota. The
// subscription deep-link clause is feature-gated: hosted plugin surfaces
// resolve the neutral entitlement explanation only.
var accountQuotaDesc = mcpforge.Static[mcpforge.FeatureCarrier](
	"Call account_quota to read the account's quota status (upload, download, storage usage/limits/remaining) and whether it is covered by granted usage (has_quota). Quota trumps a subscription: when has_quota is true the user needs no subscription. When has_quota is false the result relates to account_subscription",
).
	// Non-hosted surfaces may hand the human the subscription deep-link.
	Unless(FeatHosted, "; when that reports not-subscribed the response carries a web_url deep-link that the human opens in the web app to subscribe — the model acting alone cannot subscribe on their behalf").
	// Hosted plugin surfaces get a factual statement of the returned result
	// only: descriptions must describe what the tool does, not direct the
	// model's response.
	When(FeatHosted, ". When has_quota is false, the response reports the exhausted entitlement only — it carries no subscription state, no plan information, and no links; subscription state is read separately with account_subscription")

// accountSubscriptionDesc is the per-profile MCP description for
// account_subscription. The subscription-management deep-link clause is
// feature-gated: hosted plugin surfaces resolve the purely-informational
// wording only.
var accountSubscriptionDesc = mcpforge.Static[mcpforge.FeatureCarrier](
	"Call account_subscription to read the user's active subscription status (is_subscribed, plan period, gateway, cancellation/pause state)",
).
	Unless(FeatHosted, " and the response's web_url field: the HTTPS deep-link to the deployment's account console subscription management page. The URL is returned as data only; a human must open it in a browser to subscribe or change plans — the model acting alone cannot subscribe on the user's behalf").
	When(FeatHosted, ". Purely informational: the response carries no subscription link and no way to start or change a subscription")

// accountQuotaTargets is the MCPTargets slice for account_quota. The
// FallbackFunc target resolves the DescBuilder per-request so the description
// is deployment-aware without a static string.
var accountQuotaTargets = MCPTargets(
	FallbackFunc(func(p any) string {
		return accountQuotaDesc.Resolve(forgeProfileOf(p))
	}),
)

// accountSubscriptionTargets is the MCPTargets slice for
// account_subscription, resolved per-request like accountQuotaTargets.
var accountSubscriptionTargets = MCPTargets(
	FallbackFunc(func(p any) string {
		return accountSubscriptionDesc.Resolve(forgeProfileOf(p))
	}),
)
