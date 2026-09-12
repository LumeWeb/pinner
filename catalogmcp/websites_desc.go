package catalogmcp

import (
	"go.lumeweb.com/mcpforge"
)

// A file host is one that can build {download_url, file_id} file references
// (OpenAI/ChatGPT runtime); such hosts enable the top-level `file` parameter
// on upload/vault tools.
// This boundary does not detect hosts; consumers pass a MCPProfile (or any
// carrier reporting a mcpforge.FeatureSet) into catalogmcp.NewCompilerForProfile
// to resolve profile-gated segments.
// MCPProfile is the feature-carrier consumers hand to DescFunc resolvers via
// catalogmcp.NewCompilerForProfile. See profile.go for the explicit
// adapter contract: consumers with a richer host profile (e.g. a
// Has/IsHost-style one) adapt it via
// ProfileFromHas, or implement ForgeFeatureCarrier directly. An unadapted
// non-carrier profile is a reported adapter gap (per-compiler
// (*mcpCompiler).AdapterGap), never a silently empty feature set.
type MCPProfile struct {
	Features mcpforge.FeatureSet
}

// FeatFileHostInput gates the description DSL segment for hosts that can
// build {download_url, file_id} file references (e.g. OpenAI/ChatGPT-style
// runtimes); it enables the top-level `file` parameter clause on description
// resolution.
const FeatFileHostInput = mcpforge.Feature("file-host-input")

// FeatureSet reports the carrier's features (mcpforge.FeatureCarrier).
func (p MCPProfile) FeatureSet() mcpforge.FeatureSet {
	if p.Features == nil {
		return mcpforge.FeatureSet{}
	}
	return p.Features
}

// ForgeFeatureCarrier is anything that exposes a mcpforge.FeatureSet. Remote
// consumers' own profile types may implement this directly so their existing
// profiles resolve MCP descriptions here without conversion (the alternative
// Has-style adaptation path is ProfileFromHas in profile.go).
type ForgeFeatureCarrier interface {
	FeatureSet() mcpforge.FeatureSet
}

// websitesCreateDesc is the per-profile MCP description for websites_create,
// composed from discrete DSL sentences rather than a monolithic string. Each
// sentence is an independently gateable segment so feature-specific guidance
// (file-parameter hosts vs mint-only hosts) never leaks across profiles.
//
// The description retains the operational invariants the tool enforces and
// the common mistake-prevention guidance that was in the original Fallback
// string. The full custom-domain vs platform-label decision tree lives in
// agent_guide's publish_website flow — the tool description points at it
// rather than restating it.
// The composition is sized for host metadata limits: the resolved
// description must stay at or below Claude Code's 2 KiB description
// truncation point in BOTH feature states (with and without
// FeatFileHostInput), so keep invariant guidance first and prose lean.
var websitesCreateDesc = mcpforge.Static[mcpforge.FeatureCarrier]("Create a website that serves an IPFS CID.").
	// CID structure invariant — the tool validates and rejects violations.
	Static("The CID is a directory whose root must contain index.html (gateways serve /index.html at the root); a root without index.html, or wrapped in a single parent directory (site.zip/mysite/index.html), is rejected — correct: site.zip/index.html.").
	Static("A multi-file site is published as its component files (index.html, CSS, JS, images), not a single flattened HTML page.").
	Static("For a ZIP bundle (index.html, CSS, JS, images), zip the directory CONTENTS, not the directory, and upload with archive_mode=convert.").
	Static("For a single HTML file, upload_file with wrap=true and no name: wrapped HTML auto-renames to index.html so the site resolves at root.").
	Static("An explicit name (e.g. 'starter-site') is honored as-is: the page is reachable only at /starter-site, not /.").
	Static("A generic create/publish request implies no custom naming: pass only {\"cid\":\"<cid>\"} and a platform subdomain is auto-minted; never invent a domain or label.").
	Static("Custom domain: pass {\"cid\":\"<cid>\",\"website\":\"<domain>\"} (target-type and dns-hosting optional); namespaces default to icann; for a Handshake (alt-root) name like acme/ pass {\"namespace\":\"hns\"}.").
	Static("After creating an HNS site, read pinner://websites/<domain>/dns-requirements for the records to publish in the HNS wallet; managed DNS covers the authoritative side.").
	Static("Platform subdomain with explicit label: {\"cid\":\"<cid>\",\"platform\":true,\"label\":\"<label>\"} or {\"cid\":\"<cid>\",\"platform\":true,\"generate\":true}.").
	Static("Use the CID an upload tool returned directly — it is already pinned; pins_add is only for CIDs that originated outside Pinner.").
	Static("Full custom-domain vs platform-label decision tree: agent_guide's publish_website flow.").
	When(FeatFileHostInput, "The upload tool's file parameter is the preferred byte path on this host.").
	Static("Returns the created website (numeric ID, validation TXT token, DNS records to publish).")

// websitesCreateTargets is the MCPTargets slice for websites_create. The
// FallbackFunc target resolves the DescBuilder per-request so the description
// is profile-aware without a static string.
var websitesCreateTargets = MCPTargets(
	FallbackFunc(func(p any) string {
		return websitesCreateDesc.Resolve(forgeProfileOf(p))
	}),
)
