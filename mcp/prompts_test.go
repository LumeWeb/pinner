package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/pinner/assembly"
)

// Characterization tests for the Pinner prompts: prompt-set gating per
// scope and
// deterministic template rendering with embedded resource references.

func TestPromptNamesFullSurface(t *testing.T) {
	prompts := PromptDescriptorsForScope(assembly.FullDomainScope, false)
	names := promptNames(prompts)
	require.Equal(t, []string{PromptWebsiteOnboarding, PromptWebsiteUpdate, PromptSetup, PromptENSPublish}, names)
}

// TestPromptGatingPerSurface pins the tool-domain gating: websites prompts on
// the websites scope, setup on the account scope, ENS on the ENS scope;
// a scope without the domain omits its prompt.
func TestPromptGatingPerSurface(t *testing.T) {
	websitesOnly := assembly.DomainScope{Websites: true}
	require.Equal(t, []string{PromptWebsiteOnboarding, PromptWebsiteUpdate},
		promptNames(PromptDescriptorsForScope(websitesOnly, false)))

	accountOnly := assembly.DomainScope{Account: true}
	require.Equal(t, []string{PromptSetup}, promptNames(PromptDescriptorsForScope(accountOnly, false)))

	ensOnly := assembly.DomainScope{ENS: true}
	require.Equal(t, []string{PromptENSPublish}, promptNames(PromptDescriptorsForScope(ensOnly, false)))

	none := assembly.DomainScope{Pins: true, DNS: true, IPNS: true, Operations: true, Admin: true, Vault: true, Upload: true}
	require.Empty(t, PromptDescriptorsForScope(none, false))
}

func promptNames(prompts []model.PromptDescriptor) []string {
	out := make([]string, 0, len(prompts))
	for _, p := range prompts {
		out = append(out, p.Name)
	}
	return out
}

// TestRenderPromptTemplateSmoke pins the render path behavior: every known
// prompt template in the package-level parsed set renders non-empty prose
// (and a data-driven template carries its data). The individual prompt
// handlers pin the exact message skeletons; this covers renderPromptTemplate
// directly across the template families.
func TestRenderPromptTemplateSmoke(t *testing.T) {
	siteData := sitePromptData{Domain: "example.com"}
	updateData := sitePromptData{WebsiteArg: "example.com", CID: "bafy"}
	ensData := sitePromptData{ENSName: "vitalik.eth"}

	cases := []struct {
		name string
		data sitePromptData
	}{
		{"website_overview", siteData},
		{"website_step_content_source_ask", siteData},
		{"website_step_dns_setup_filled", siteData},
		{"website_update_overview", updateData},
		{"ens_publish_overview", ensData},
		{"setup_overview", sitePromptData{}},
	}
	for _, tc := range cases {
		out := renderPromptTemplate(tc.name, tc.data)
		require.NotEmpty(t, strings.TrimSpace(out), "template %s must render non-empty prose", tc.name)
	}

	require.Contains(t, renderPromptTemplate("website_step_domain_filled", siteData), "example.com",
		"data-driven templates must carry their data into the rendered prose")
}

// TestWebsiteOnboardingPromptRendering renders the onboarding prompt for the
// guided (no-args) and pre-filled variants and pins the deterministic message
// skeleton: step blocks, wizard tools, and the pinner:// resource embeds.
func TestWebsiteOnboardingPromptRendering(t *testing.T) {
	prompts := PromptDescriptors()
	p := promptByName(t, prompts, PromptWebsiteOnboarding)
	require.Equal(t, "Website Onboarding Wizard", p.Title)

	res, err := p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{}})
	require.NoError(t, err)
	require.Equal(t, "Website onboarding wizard workflow with embedded resource references", res.Description)

	texts := allTexts(res)
	embedded := embeddedURIs(res)
	require.Contains(t, texts[0], "website creation wizard")
	require.Contains(t, joined(texts), "websites_wizard_start", "the wizard is started with the wizard tools")
	require.Contains(t, joined(texts), "websites_wizard_step")
	require.Contains(t, embedded, AccountStatusURI, "step 1 embeds the account status resource")
	require.Contains(t, embedded, PlatformDomainsURI, "the domain step embeds the platform-domains resource")
	require.Contains(t, embedded, ValidationStatusTmpl, "the validate step embeds the validation-status template")

	// Pre-filled arguments switch the content/source/domain steps to the
	// pre-filled variants and carry the domain through the template prose.
	prefilled, err := p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{
		ArgDomain:        "example.com",
		ArgContentSource: "upload",
		ArgTargetType:    "ipfs",
		ArgDNSMode:       "managed",
	}})
	require.NoError(t, err)
	prefTexts := joined(allTexts(prefilled))
	require.Contains(t, prefTexts, "example.com", "pre-filled variant must render filled steps")
	require.Contains(t, prefTexts, "upload")
	// The no-args variant embeds a per-domain dns-requirements URI only when a
	// domain was supplied; the guided variant does not.
	noArgsEmbedded := embeddedURIs(res)
	require.NotContains(t, noArgsEmbedded, "pinner://websites/example.com/dns-requirements")
	require.Contains(t, embeddedURIs(prefilled), "pinner://websites/example.com/dns-requirements")
}

// TestWebsiteOnboardingValidation pins the argument validation contract.
func TestWebsiteOnboardingValidation(t *testing.T) {
	p := promptByName(t, PromptDescriptorsForScope(assembly.FullDomainScope, false), PromptWebsiteOnboarding)
	for name, want := range map[string]string{
		"content_source": "invalid content_source",
		"target_type":    "invalid target_type",
		"dns_mode":       "invalid dns_mode",
	} {
		_, err := p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{name: "nonsense"}})
		require.ErrorContains(t, err, want, name)
	}
}

// TestWebsiteUpdatePrompt pins the update workflow skeleton and required args.
func TestWebsiteUpdatePrompt(t *testing.T) {
	p := promptByName(t, PromptDescriptorsForScope(assembly.FullDomainScope, false), PromptWebsiteUpdate)

	_, err := p.Handler(context.Background(), model.PromptRequest{})
	require.ErrorContains(t, err, "website is required")
	_, err = p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{ArgWebsite: "example.com"}})
	require.ErrorContains(t, err, "cid is required")

	res, err := p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{
		ArgWebsite: "example.com", ArgCID: "bafy", ArgCurrentType: "ipfs",
	}})
	require.NoError(t, err)
	require.Equal(t, "Website update workflow with target-type preservation and DNS-mode handling", res.Description)
	require.Len(t, res.Messages, 8)
	require.Contains(t, allTexts(res)[0], "update")
	// The validation-status template URI is embedded before the validate step.
	require.Equal(t, ValidationStatusTmpl, res.Messages[5].EmbeddedResource.URI)
}

// TestENSPublishPrompt pins the ENS flow skeleton: upload-when-no-CID, the
// ens_point discovery step, and the wallet-agnostic onchain guidance.
func TestENSPublishPrompt(t *testing.T) {
	p := promptByName(t, PromptDescriptorsForScope(assembly.FullDomainScope, false), PromptENSPublish)

	_, err := p.Handler(context.Background(), model.PromptRequest{})
	require.ErrorContains(t, err, "name is required")

	withCID, err := p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{
		ArgENSName: "vitalik.eth", ArgCID: "bafy",
	}})
	require.NoError(t, err)
	joined := strings.Join(allTexts(withCID), "\n")
	require.Contains(t, joined, "ens_point")
	require.Contains(t, joined, "wallet")
	require.Contains(t, joined, "app.ens.domains")
	require.NotContains(t, joined, "MetaMask required")

	withoutCID, err := p.Handler(context.Background(), model.PromptRequest{Arguments: map[string]string{ArgENSName: "vitalik.eth"}})
	require.NoError(t, err)
	require.Contains(t, strings.Join(allTexts(withoutCID), "\n"), "upload")
}

// TestSetupPrompt pins the setup wizard skeleton with the account-status
// resource embeds at the first and last steps.
func TestSetupPrompt(t *testing.T) {
	p := promptByName(t, PromptDescriptorsForScope(assembly.FullDomainScope, false), PromptSetup)
	res, err := p.Handler(context.Background(), model.PromptRequest{})
	require.NoError(t, err)
	require.Equal(t, "Setup wizard workflow with embedded resource references", res.Description)
	embedded := embeddedURIs(res)
	require.Len(t, embedded, 2, "setup embeds the account-status resource at first and last steps")
	require.Equal(t, AccountStatusURI, embedded[0])
	require.Equal(t, AccountStatusURI, embedded[len(embedded)-1])
	texts := joined(allTexts(res))
	require.Contains(t, texts, "setup_wizard_start")
	require.Contains(t, texts, "setup_wizard_step")
}

// TestPromptEmbeddedMessages pins the embedded-resource message form.
func TestPromptEmbeddedMessages(t *testing.T) {
	msg := embeddedMsg(AccountStatusURI)
	require.Equal(t, "user", msg.Role)
	require.NotNil(t, msg.EmbeddedResource)
	require.Equal(t, AccountStatusURI, msg.EmbeddedResource.URI)
	require.Equal(t, "application/json", msg.EmbeddedResource.MIMEType)
}

// --- helpers ---

func strContains(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func promptByName(t *testing.T, prompts []model.PromptDescriptor, name string) model.PromptDescriptor {
	t.Helper()
	for _, p := range prompts {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("prompt %q not found", name)
	return model.PromptDescriptor{}
}

func allTexts(res model.PromptResult) []string {
	out := make([]string, 0, len(res.Messages))
	for _, m := range res.Messages {
		out = append(out, m.Text)
	}
	return out
}

func embeddedURIs(res model.PromptResult) []string {
	var out []string
	for _, m := range res.Messages {
		if m.EmbeddedResource != nil {
			out = append(out, m.EmbeddedResource.URI)
		}
	}
	return out
}

func joined(texts []string) string { return strings.Join(texts, "\n") }

// TestHostedPromptsRenderPolicyNeutralQuotaGuidance pins the plugin-commerce
// policy gate on the prompt surface: with hosted=true, every prompt whose
// guidance touches quota/subscription renders the `*_hosted` template
// variants — the sanctioned entitlement explanation, with no web_url
// deep-link and no subscribe prompting. Hosted=false keeps the unchanged
// deep-link guidance.
func TestHostedPromptsRenderPolicyNeutralQuotaGuidance(t *testing.T) {
	for _, tc := range []struct {
		prompt string
	}{
		{PromptWebsiteOnboarding},
		{PromptSetup},
		{PromptENSPublish},
	} {
		t.Run(tc.prompt, func(t *testing.T) {
			hosted := promptByName(t, PromptDescriptorsForScope(assembly.FullDomainScope, true), tc.prompt)
			hargs := model.PromptRequest{}
			if tc.prompt == PromptENSPublish {
				hargs = model.PromptRequest{Arguments: map[string]string{ArgENSName: "vitalik.eth"}}
			}
			hres, err := hosted.Handler(context.Background(), hargs)
			require.NoError(t, err)
			htext := joined(allTexts(hres))
			require.NotContains(t, htext, "web_url",
				"hosted prompt guidance must never reference a subscription deep-link")
			require.NotContains(t, htext, "surface the returned web_url")
			require.Contains(t, htext, "unavailable on this account",
				"hosted guidance must carry the sanctioned entitlement explanation")

			local := promptByName(t, PromptDescriptorsForScope(assembly.FullDomainScope, false), tc.prompt)
			largs := model.PromptRequest{}
			if tc.prompt == PromptENSPublish {
				largs = model.PromptRequest{Arguments: map[string]string{ArgENSName: "vitalik.eth"}}
			}
			lres, err := local.Handler(context.Background(), largs)
			require.NoError(t, err)
			ltext := joined(allTexts(lres))
			require.Contains(t, ltext, "web_url deep-link",
				"non-hosted prompt guidance keeps the documented deep-link UX")
		})
	}
}
