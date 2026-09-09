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
var websitesCreateDesc = mcpforge.Static[mcpforge.FeatureCarrier]("Create a website that serves an IPFS CID.").
	// CID structure invariant — the tool validates and rejects violations.
	Static("The CID is a directory whose root contains index.html — gateways serve /index.html at the directory root.").
	Static("This tool rejects a CID whose root has no index.html or is wrapped in a single parent directory (e.g. site.zip/mysite/index.html is rejected; correct: site.zip/index.html).").
	Static("A multi-file website is published as its component files (index.html, CSS, JS, images) rather than flattened into a single HTML file.").
	Static("For a site bundle (ZIP with index.html, CSS, JS, images), ZIP the directory contents (not the directory itself) and upload with archive_mode=convert.").
	Static("For a single HTML file, use upload_file with wrap=true and no explicit name — the tool auto-names wrapped HTML to index.html so the site resolves at root.").
	Static("An explicit name like 'starter-site' is honored as-is and the page will only be reachable at /starter-site, not /.").
	Static("If the user has no domain, call websites_create with only {\"cid\":\"<cid>\"} and a platform subdomain is auto-minted; a domain or label is not invented for a generic request.").
	Static("For a custom domain, pass {\"cid\":\"<cid>\",\"website\":\"<domain>\"} (target-type and dns-hosting are optional).").
	Static("Custom domains default to namespace icann (traditional DNS). For a Handshake (alt-root) name like acme/, pass {\"namespace\":\"hns\"}.").
	Static("After creating a Handshake site, read pinner://websites/<domain>/dns-requirements — it renders the records to publish on-chain in the HNS wallet (parent NS/DS/GLUE) plus the authoritative side; managed DNS handles the authoritative side for you.").
	Static("For a platform subdomain with an explicit label, pass {\"cid\":\"<cid>\",\"platform\":true,\"label\":\"<label>\"} or {\"cid\":\"<cid>\",\"platform\":true,\"generate\":true}.").
	Static("See agent_guide's publish_website flow for the full custom-domain vs platform-label decision tree.").
	Static("A generic request to create or publish a website implies no custom naming; default to no domain unless the user explicitly supplies or requests a specific label or domain.").
	Static("For newly uploaded content, use the CID returned by the upload tool directly — the upload already pinned it, so pins_add after upload is unnecessary.").
	Static("pins_add is only needed when the CID originated outside Pinner and requires import from IPFS.").
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
