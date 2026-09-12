package mcp

import (
	"context"
	"strings"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/toolargs"
	"go.lumeweb.com/mcpplane/transfer"
	"go.lumeweb.com/pinner/assembly"
	"go.lumeweb.com/pinner/mcp/appswire"
)

// The guide wire-model types (AgentGuide, GuideFlow, GuideDecision,
// GuideBranch) live in mcpforge — the platform-DSL package — so the guide is
// composed by the same DSL that builds schemas and descriptions. These type
// aliases expose them under the package's own names so call-sites and tests
// need not import mcpforge directly.
type (
	AgentGuide    = mcpforge.AgentGuide
	GuideFlow     = mcpforge.GuideFlow
	GuideDecision = mcpforge.GuideDecision
	GuideBranch   = mcpforge.GuideBranch
)

// sourceModePrefix prefixes a transfer.FileSourceMode value into the
// "source.mode=X" label the guide surfaces. The mode names themselves come
// from the shared transfer enum so this guide can never drift from what
// capabilities() and the upload_file schema advertise.
const sourceModePrefix = "source.mode="

// guideSourceModes returns the transport-scoped source modes the resolved
// profile actually supports. It derives from the transport (matching
// capabilities().source_modes and the upload_file schema enum), never from the
// profile's feature flags: the separate upload_data / upload_url TOOLS are
// gated on FeatSourceData/FeatSourceURL, but upload_file's `source` enum is a
// pure function of the transport (mint on HTTP, path on stdio, url/data on the
// OpenAI tunnel). Deriving from features here would advertise a source mode
// the upload_file schema on that transport cannot accept.
func guideSourceModes(profile HostProfile) []string {
	switch profile.Transport {
	case canimcp.TransportStdio:
		return []string{sourceModePrefix + string(transfer.SourcePath)}
	case canimcp.TransportOpenAI:
		// The tunnel's url + data pair is a single relay label in the guide.
		return []string{sourceModePrefix + string(transfer.SourceURL) + "/" + string(transfer.SourceData)}
	default: // TransportHTTP
		return []string{sourceModePrefix + string(transfer.SourceMint)}
	}
}

// sourceModesText joins guideSourceModes with "or" for inline use in
// description segments.
func sourceModesText(profile HostProfile) string {
	return strings.Join(guideSourceModes(profile), " or ")
}

// uploadDetailDesc composes the upload flow detail string from feature-gated
// segments, replacing the previous string concatenation. The returned CID is
// already pinned, so it must never steer an agent to pins_add.
var uploadDetailDesc = mcpforge.Static[HostProfile](
	"Check capabilities to pick the byte source THIS client is told to use.",
).
	When(FeatFileHostInput,
		"If capabilities' file_input_policy is host_file_first (only when your client can hand Pinner a {download_url, file_id} file object), pass a host-provided file reference directly. Otherwise use a transport-scoped source: {{SOURCES}}.",
	).
	Unless(FeatFileHostInput,
		"Use a transport-scoped source: {{SOURCES}}.",
	).
	Static("The returned CID is already pinned — use it directly in websites_create/update; do NOT call pins_add after an upload").
	WhenSentence(FeatSourceMint,
		"Mint (source.mode=mint) has NOT stored bytes when upload_file returns — it only mints url + upload_handle.",
	).
	WhenSentence(FeatSourceMint,
		"1) PUT your agent-local file to the returned url (curl -sS -T <file> \"<url>\")",
	).
	WhenSentence(FeatSourceMint,
		"2) "+appswire.UploadMintPoll,
	).
	WhenSentence(FeatSourceMint,
		"3) the completed CID is already pinned — use it directly; do NOT call pins_add. Treat the mint response as the START of the upload, not the end.",
	).
	ListWhenAny([]mcpforge.Feature{FeatSourceURL, FeatSourceData},
		mcpforge.List[HostProfile](mcpforge.ListNumbered).
			Intro("Pick the byte route in this order:").
			ItemWhen(FeatSourceMint, "a file you can read locally → upload_file mint + host PUT + upload_status").
			ItemWhen(FeatSourceURL, "bytes already at a public HTTPS URL → upload_url (server fetch; do not download then re-upload)").
			ItemWhen(FeatSourceData, "only raw bytes, no file, no URL → upload_data (RFC 2397 data: URI) — last resort; never base64-encode a real file"),
	).
	StaticSentence("Static site bundle rule: a ZIP containing index.html, CSS, JS, images, or nested pages is a single directory DAG — call upload_file").
	When(FeatFileHostInput,
		"with the host file argument (or a convert source) and archive_mode=convert",
	).
	Unless(FeatFileHostInput,
		"with a convert source ({{SOURCES}}) and archive_mode=convert",
	).
	StaticList("not individual assets.")

// vaultUploadDetailDesc composes the vault upload flow detail string.
var vaultUploadDetailDesc = mcpforge.Static[HostProfile](
	"Check capabilities to pick the byte source THIS client is told to use.",
).
	When(FeatFileHostInput,
		"If capabilities' file_input_policy is host_file_first (only when your client can hand Pinner a {download_url, file_id} file object), pass a host-provided file reference directly. Otherwise use a transport-scoped source ({{SOURCES}}) plus the destination vault_path.",
	).
	Unless(FeatFileHostInput,
		"Use a transport-scoped source ({{SOURCES}}) plus the destination vault_path.",
	).
	When(FeatSourceMint,
		"When using source.mode=mint + vault_path, mint returns a one-time presigned PUT url bound to vault_path (it has NOT stored bytes yet). PUT the agent-local file to the returned url; the PUT returns quickly after staging the bytes locally (status: staged). The file is immediately readable from this instance; durability on Sia happens in the background or via the vault_flush tool — vault_flush is non-blocking and returns an accepted job { job_id, profile, path? }, so poll vault_flush_status(job_id) or vault_stat until status: durable when durability is needed before sharing; there is no upload_status to poll.",
	).
	WhenSentence(FeatSourceMint,
		"On this mint-only transport there is no direct vault path for a public URL or inline bytes: materialize them to an agent-local file first, then vault_put_file(source.mode=mint) + PUT.",
	).
	When(FeatSourceURL,
		"The separate upload_url tool is IPFS-only, not a vault write — do not invent a 'vault a CID' step.",
	).
	When(FeatSourceData,
		"The separate upload_data tool is IPFS-only, not a vault write — do not invent a 'vault a CID' step.",
	).
	WhenPredSep(mcpforge.SepSentence, TransportIs(canimcp.TransportOpenAI),
		"The separate upload_url / upload_data tools pin to IPFS and are NOT vault writes: over this tunnel transport vault_put_file takes public-URL or raw-inline bytes via its own url/data source plus the destination vault_path. Do not invent a 'vault a CID' step.",
	)

// downloadDetailDesc composes the download flow detail string. sink=local is
// always available but writes to the MCP server's own disk; for a remote agent
// (not co-located) that path is invisible, so drop is the preferred sink.
var downloadDetailDesc = mcpforge.Static[HostProfile](
	"Read capabilities' download_sink_modes; call download_file with ipfs_path (CID or CID/path) using a supported sink.",
).
	WhenSentence(FeatSinkDrop,
		"Prefer sink=drop: it returns a one-time HTTP GET filedrop link to pull into your sandbox with curl -o or a browser.",
	).
	UnlessSep(mcpforge.SepSentence, FeatCoLocated,
		"sink=local writes to a path on the MCP server's own disk and is NOT visible to a remote agent like this one — do not look for the downloaded file in your sandbox.",
	).
	UnlessSep(mcpforge.SepSentence, FeatSinkDrop,
		"On this transport, sink=local is the only sink offered.",
	)

// vaultDownloadDetailDesc composes the vault download flow detail string.
var vaultDownloadDetailDesc = mcpforge.Static[HostProfile](
	"Read capabilities' download_sink_modes and ensure the vault is unlocked; call vault_get_file with vault_path using a supported sink.",
).
	WhenSentence(FeatSinkDrop,
		"Prefer sink=drop: it returns a one-time HTTP GET filedrop link to pull into your sandbox.",
	).
	UnlessSep(mcpforge.SepSentence, FeatCoLocated,
		"sink=local writes the decrypted bytes to the MCP server's own disk and is NOT visible to a remote agent like this one.",
	).
	UnlessSep(mcpforge.SepSentence, FeatSinkDrop,
		"On this transport, sink=local is the only sink offered.",
	)

// guideSummary is the guide's opening orientation: start here, check state,
// follow flows, treat a static website ZIP as a single directory DAG, and take
// the byte path capabilities actually reports for THIS host. The byte-path and
// wizard-orientation tails are feature-gated so a host without a `file`
// parameter (e.g. Grok) never sees a "prefer the file parameter" clause, and a
// host without elicitation is not steered into a wizard. The {{SOURCES}} token
// is substituted per profile.
var guideSummary = mcpforge.Static[HostProfile](
	"Start here. Drive Pinner through these primary flows; each step is a tool. Check the current state first, then follow the matching flow. A static website ZIP (index.html, CSS, JS, images, nested pages) is always a single directory DAG: call upload_file",
).
	When(FeatFileHostInput,
		"with a host file argument IF capabilities' file_input_policy is host_file_first (your client can hand Pinner a {download_url, file_id} object), otherwise a convert-capable transport source",
	).
	Unless(FeatFileHostInput,
		"with a convert-capable transport source ({{SOURCES}})",
	).
	StaticList("then publish the resulting directory CID.").
	When(FeatFileHostInput,
		"Follow the byte path capabilities reports: when file_input_policy is host_file_first, prefer the `file` parameter (user attachments AND assistant-generated sandbox files) over a transport source; otherwise use a transport-scoped source ({{SOURCES}}). Do NOT invent an OpenAI download_url/file_id or base64-encode a file as a data URI.",
	).
	Unless(FeatFileHostInput,
		"This host has no `file` parameter it can fill: use a transport-scoped source ({{SOURCES}}). Do NOT invent a file_id or OpenAI download_url, and do NOT base64-encode a file as upload_data.",
	).
	When(FeatSourceMint,
		"For source.mode=mint, completion differs by tool: upload_file is asynchronous — PUT the agent-local file to the returned url, then poll upload_status; vault_put_file is non-blocking — PUT the file and it returns after staging locally (status: staged), with durability on Sia happening in the background or via the vault_flush tool (which is itself non-blocking and returns an accepted job { job_id, profile, path? }), so poll vault_flush_status(job_id) or vault_stat until status: durable when durability is needed before sharing; there is no upload_status poll (see the upload and vault_upload flows).",
	).
	When(FeatSourcePath,
		"For source.mode=path, point the source at the host-side file/directory/archive path — the server reads it directly, so there is no PUT.",
	).
	WhenAll([]mcpforge.Feature{FeatSourceMint, FeatSourceURL, FeatSourceData},
		"Byte route order is in the upload flow: a local file → mint + PUT, a public HTTPS URL → upload_url, raw bytes → upload_data.",
	).
	StaticSentence("For autonomous website publishing after an upload, run the publish_website flow directly. For explicitly requested guided website onboarding (human-in-the-loop, step-by-step DNS setup), use the website-onboarding prompt and the websites_wizard tools (websites_wizard_start → websites_wizard_step) instead. Once a wizard session is active, stay in it: always call the returned next_step_schema via the wizard step tool — do not abandon the wizard to rediscover low-level tools.").
	// Inventory-gated, not FeatMCPApps-gated: a host may signal the MCP Apps
	// capability while its composition root registered no app views (no apps
	// registry wired). {{APPS}} enumerates the actual installed launchers, so
	// the summary can never name a view tools/list does not carry.
	WhenPred(appsGate(AppsInstalled()),
		"This host renders MCP Apps: interactive app views are available via open_app for human-facing interactions ({{APPS}}). Prefer headless primitives for autonomous workflows; call open_app only when a human-facing screen is needed.")

// guideArchiveInvariant and guideCIDStructure are the two operational website
// rules every agent must honor. Kept as named fragments so branch guidance can
// cite the same wrapper rule without duplicating the prose.
var (
	guideArchiveInvariant = "Website archive invariant: before publishing any generated static-site archive, verify that index.html is at the archive root. Never publish an archive where the entire site is wrapped in a single parent directory (e.g. site.zip/mysite/index.html). The correct layout is site.zip/index.html. If the first path component wraps the entire site, rebuild the archive from the directory's contents, not the directory itself."
	guideCIDStructure     = "Website CID structure: a website CID must be a directory whose root contains index.html. Gateways serve /index.html at the directory path. Uploading an archive with archive_mode=convert produces a directory CID whose structure mirrors the archive — if the archive has a wrapper directory, the CID will too, and the site will not resolve at /. The tool will reject a CID that has no root index.html or is wrapped in a single parent directory."
)

// byteRouteDecision composes the "where are the bytes?" chooser as a guide
// Decision so the flow's steps (not just its detail) can produce a CID from any
// source the host actually registers. Each branch is feature-gated and ends with
// real upload tools — every step resolves to a genuine tool, so the guide's
// "steps are real tools" invariant holds.
//
// The branches are intentionally the union of every profile's route; each host
// resolves to only the branches its features enable, so uploaded bytes always
// have a matching, non-invented chain. next, when non-nil, is attached to every
// branch as a nested decision (used by publish_website to chain the byte route
// to the domain/websites_create choice).
func byteRouteDecision(next *mcpforge.GuideDecisionBuilder[HostProfile]) *mcpforge.GuideDecisionBuilder[HostProfile] {
	return mcpforge.Decision("Where are the bytes?",
		mcpforge.Branch[HostProfile]("a file on the host — a user attachment, OR a file the host runtime itself created (assistant-generated sandbox file)").
			WhenFeature(FeatFileHostInput).
			Steps("upload_file").
			Detail(mcpforge.Static[HostProfile]("Pass the host file reference via the file argument; the host runtime fetches and uploads it. Do not base64-encode, mint a presigned URL, or build a download_url/file_id yourself.")).
			Next(next),
		mcpforge.Branch[HostProfile]("a local file/directory path on a co-located host").
			WhenFeature(FeatSourcePath).
			Steps("upload_file").
			Detail(mcpforge.Static[HostProfile]("Use source.mode=path with the host-side file/directory/archive path; the server reads it directly.")).
			Next(next),
		mcpforge.Branch[HostProfile]("agent-local bytes the host runtime cannot provide through `file` (not a host/user/assistant-generated file)").
			WhenFeature(FeatSourceMint).
			Steps("upload_file").
			StepWhen(FeatSourceMint, "<host PUT>", "upload_status").
			Detail(mcpforge.Static[HostProfile]("Mint has NOT stored bytes when upload_file returns: PUT the agent-local file to the returned url (curl -sS -T <file> \"<url>\"), then poll upload_status until completed — the completed CID is already pinned; do not call pins_add.")).
			Next(next),
		mcpforge.Branch[HostProfile]("bytes already at a public HTTPS URL (user handed a URL)").
			WhenFeature(FeatSourceURL).
			Steps("upload_url").
			Detail(mcpforge.Static[HostProfile]("upload_url server-fetches the public HTTPS URL and pins it; do not download then re-upload.")).
			Next(next),
		mcpforge.Branch[HostProfile]("only raw inline bytes, with no file and no URL").
			WhenFeature(FeatSourceData).
			Steps("upload_data").
			Detail(mcpforge.Static[HostProfile]("upload_data is a last resort (RFC 2397 data: URI); never base64-encode a real or host-provided file into it.")).
			Next(next),
	)
}

// vaultByteRouteDecision is the upload-chooser twin for the vault flow. Every
// branch still ends at vault_put_file — the ONLY vault write (there is no
// "vault a CID" tool) — but the decision makes the steps represent the byte
// source (host file, local path, mint, public URL, raw bytes) so a steps-first
// model does not assume vault storage must be mint, nor invent a path through
// the IPFS-only upload_url / upload_data tools.
func vaultByteRouteDecision() *mcpforge.GuideDecisionBuilder[HostProfile] {
	return mcpforge.Decision("Where are the bytes for the vault?",
		mcpforge.Branch[HostProfile]("a file on the host — a user attachment, OR a file the host runtime itself created (assistant-generated sandbox file)").
			WhenFeature(FeatFileHostInput).
			Steps("vault_put_file").
			Detail(mcpforge.Static[HostProfile]("Pass the host file reference via the file argument; the vault stores its bytes at vault_path.")),
		mcpforge.Branch[HostProfile]("a local file/directory path on a co-located host").
			WhenFeature(FeatSourcePath).
			Steps("vault_put_file").
			Detail(mcpforge.Static[HostProfile]("Use source.mode=path with the host-side path and the destination vault_path; the server reads it directly.")),
		mcpforge.Branch[HostProfile]("agent-local bytes the host runtime cannot provide through `file` (not a host/user/assistant-generated file)").
			WhenFeature(FeatSourceMint).
			Steps("vault_put_file").
			StepWhen(FeatSourceMint, "<host PUT>").
			Detail(mcpforge.Static[HostProfile]("vault_put_file with source.mode=mint + vault_path mints a one-time presigned PUT url bound to vault_path; it has not stored bytes yet.").
				StaticSentence("PUT the agent-local file to the returned url.").
				StaticSentence("The vault write is non-blocking: the PUT returns after staging the bytes locally (status: staged); the file is immediately readable from this instance, and durability on Sia happens in the background or via the vault_flush tool (non-blocking, returns an accepted job { job_id, profile, path? }) — poll vault_flush_status(job_id) or vault_stat until status: durable when durability is needed before sharing. There is no upload_status to poll.")),
		mcpforge.Branch[HostProfile]("bytes already at a public HTTPS URL").
			// vault_put_file's url source exists ONLY on the OpenAI tunnel
			// transport. Gate on the transport, not FeatSourceURL: Grok declares
			// FeatSourceURL to register upload_url, but its vault_put_file is
			// mint-only — there is no "vault a URL" branch on Grok.
			WhenPred(TransportIs(canimcp.TransportOpenAI)).
			Steps("vault_put_file").
			Detail(mcpforge.Static[HostProfile]("vault_put_file takes the URL via its own url source on the tunnel transport; the separate upload_url tool is IPFS-only, not a vault write.")),
		mcpforge.Branch[HostProfile]("only raw inline bytes, no file and no URL").
			WhenPred(TransportIs(canimcp.TransportOpenAI)).
			Steps("vault_put_file").
			Detail(mcpforge.Static[HostProfile]("vault_put_file takes raw inline bytes via its own data source as a last resort; never base64-encode a real or host-provided file.")),
	)
}

// publishDomainDecision is the publish_website choice of how to deploy the
// already-uploaded site: a generic platform subdomain, an explicit label, or a
// custom domain. It is nested under the byte-route decision (byteRouteDecision)
// so a model first obtains a CID via real upload tools, then chooses the
// deployment shape. Every step here is a real tool.
func publishDomainDecision() *mcpforge.GuideDecisionBuilder[HostProfile] {
	return mcpforge.Decision("Does the user have a domain or subdomain label preference?",
		mcpforge.Branch[HostProfile]("No — generic request (e.g. \"create me a website\", \"publish this\", \"host this\")").
			Steps("websites_create", "websites_validate").
			Detail(publishCidLead.Then(htmlRootClause).
				Static("Call websites_create with only {\"cid\": \"<cid>\"} — no domain, no label, no platform. The platform auto-generates a subdomain and manages DNS. Do NOT invent a label or call websites_platform_domain_availability. Do not infer a desire for custom naming from a generic request to create or publish a website.").
				Then(validateAfterCreateClause).
				Then(cdnDeployNoticeClause).
				Then(reconcileNoSleep).
				Then(siteBundleUpload())),
		mcpforge.Branch[HostProfile]("Yes — user explicitly supplied or requested a specific label (e.g. \"call it acme\", \"use myapp\")").
			Steps("websites_platform_domains_list", "websites_platform_domain_availability", "websites_create", "websites_validate").
			Detail(publishCidLead.Then(htmlRootClause).
				Static("List platform roots with websites_platform_domains_list, then check the label is claimable with websites_platform_domain_availability <label>, then call websites_create with {\"cid\": \"<cid>\", \"platform\": true, \"label\": \"<label>\"}. Only use this branch when the user explicitly named a label — never invent one to perform the availability step.").
				Then(validateAfterCreateClause).
				Then(cdnDeployNoticeClause).
				Then(reconcilePlain)),
		mcpforge.Branch[HostProfile]("Yes — user owns a custom domain (e.g. example.com)").
			Steps("websites_create", "websites_validate").
			Detail(publishCidLead.Then(htmlRootClause).
				Static("Call websites_create with {\"cid\": \"<cid>\", \"website\": \"<domain>\"}. The domain is used directly as a custom domain (not a platform subdomain). Read pinner://websites/<domain>/dns-requirements for DNS records to publish. If dns_hosting=true (managed), DNS is reconciled asynchronously — validation may report the old CID right after the update; that is reconciliation lag, not failure, so re-call websites_validate without starting a new flow. If self-managed, publish the _dnslink TXT and validation TXT before calling websites_validate.").
				Then(cdnDeployNoticeClause).
				Then(hnsNamespaceClause)),
	)
}

// BuildAgentGuide constructs the AgentGuide declaratively with the platform
// DSL, then resolves it against the given profile overlaid with the assembled
// server's domain scope and hosted deployment — both construction-time
// properties owned by the caller (Config), applied here explicitly instead of
// via package globals. Every flow, step, branch and sentence is feature-gated
// and per-host resolved through the same mcpforge DSL the tool schemas use, so
// the guide can never advertise a tool or source mode the resolved scope
// rejects
// (e.g. upload_status only appears on mint transports).
//
// installedApps is the composition root's registered app-view inventory
// (Config.InstalledApps): the guide's open_app-bearing clauses gate on it,
// so an inventory of zero (no apps registry wired) yields no open_app prose
// regardless of the profile's FeatMCPApps capability signal.
func BuildAgentGuide(profile HostProfile, scope assembly.DomainScope, hosted bool, installedApps []string) AgentGuide {
	p := profile.CloneFeatures()
	// The deployment mode is a construction-time property (owned by Config);
	// overlay it so the guide reflects the actual deployment, which the
	// request profile carries only as a wire signal.
	p.Hosted = hosted
	// The app inventory is construction-time state like hosted: overlay the
	// registered launcher names so the inventory predicates (AppIs,
	// AppsInstalled) resolve against what THIS server actually registered.
	p.KnownApps = installedApps
	substitute := func(s string) string {
		s = strings.ReplaceAll(s, "{{SOURCES}}", sourceModesText(p))
		return strings.ReplaceAll(s, "{{APPS}}", strings.Join(p.KnownApps, ", "))
	}

	spec := mcpforge.Guide[HostProfile]().
		Substitute(substitute).
		Summary(guideSummary).
		Rule(guideArchiveInvariant).
		Rule(guideCIDStructure).
		// Inventory-gated like the summary: the rule lists only the launchers
		// the assembled server actually registered ({{APPS}}), so an apps-less
		// composition root never advertises open_app at all.
		RuleWhenPred(appsGate(AppsInstalled()),
			"MCP Apps rule: this host renders interactive app views. When a user explicitly requests a visual interface, call open_app with the app name ({{APPS}}). open_app returns a ui:// view the host renders as an iframe. Prefer headless primitives (vault_status, vault_put_file, pins_list, auth_sso, ...) for autonomous workflows — call open_app only when a human-facing screen is needed.").
		// Claude Web (host "claude") on a self-hosted (non-hosted) deployment
		// cannot exercise the transport-derived mint/sink endpoints, so the
		// only working upload is the base64 upload_data relay and downloads
		// cannot be delivered to the user. Scoped to the Web host AND
		// non-hosted deployment only (RuleWhenPred) — Claude Desktop (a
		// different HostType) is co-located with full local file access and
		// must NOT get this notice, and a hosted (Portal-embedded) deployment
		// lets Claude Web use the mint/drop endpoints like any other HTTP
		// host, so it is not treated as special.
		RuleWhenPred(mcpforge.And[HostProfile](HostIs(canimcp.HostClaude), mcpforge.Not[HostProfile](HostedIs(true))),
			"Host capability notice (Claude Web): this agent has no network egress (no curl) and no file references, so the ONLY working upload is upload_data (RFC 2397 base64 data: URI passed in the tool args). upload_file's source.mode=mint and the sink=drop download link both require the agent to curl or fetch out of band, which this host cannot do, and sink=local writes to the MCP server's own unreachable disk — so warn the user before offering a download that the content cannot be delivered to them.").
		// Hosted (Portal-embedded) deployments establish the caller's identity
		// via Portal OAuth before the request reaches the MCP server. State that
		// explicitly so the agent does not attempt a config-mutating
		// auth_login/auth_logout, which are CLI/local-only scopes absent here.
		RuleWhenPred(HostedIs(true),
			"Hosted instance notice: a Portal OAuth identity is already established for the current request and authenticated operations run as that user. Do NOT call auth_login or auth_logout (they are unavailable on this hosted surface); identity cannot be switched mid-session.").
		// The subscription deep-link guidance is legal on self-hosted surfaces
		// but load-bearing for plugin policy on hosted ones: platform commerce
		// rules forbid promoted upgrades and subscription-flow links, so the
		// hosted clause permits only the entitlement explanation.
		RuleWhenPred(HostedIs(false),
			"Access policy (quota trumps a subscription): before a paid/metered action, check the user's access via account_quota (discover it with search_tools query \"quota\"). Its has_quota flag is authoritative — if true, granted quota covers the user and they need NO subscription, so proceed without asking about one. Only when has_quota is false, check account_subscription (search_tools query \"subscription\"): if subscribed, proceed; if not subscribed, surface the returned web_url deep-link so the human opens the web app to subscribe — you can neither subscribe on their behalf nor treat a subscription as a substitute when quota is available.").
		RuleWhenPred(HostedIs(true),
			"Access policy (quota trumps a subscription): before a paid/metered action, check the user's access via account_quota (discover it with search_tools query \"quota\"). Its has_quota flag is authoritative — if true, granted quota covers the user and they need NO subscription, so proceed without asking about one. Only when has_quota is false, check account_subscription (search_tools query \"subscription\"): if subscribed, proceed; if not subscribed, explain only that the feature requires an active subscription and is unavailable on this account. Never offer, link to, or point toward any subscription, plan, pricing, or upgrade page, and do not suggest upgrading — subscription changes are handled entirely outside this server. Never treat a subscription as a substitute when quota is available.").
		Flow(mcpforge.Flow[HostProfile]("auth", "Authenticate").
			Steps("auth_status", "auth_sso", "auth_resume", "auth_status").
			Detail(mcpforge.Static[HostProfile]("Run auth_status; if unauthenticated, call auth_sso and poll auth_resume with the returned handle until the human completes the browser sign-in.").
				WhenPred(appsGate(AppIs("sso_signin")),
					"On this host you can also call open_app with app=\"sso_signin\" to render an interactive sign-in card for the human."))).
		Flow(mcpforge.Flow[HostProfile]("vault_create", "Create a vault").
			Steps("vault_create", "vault_create_resume", "vault_status").
			Detail(mcpforge.Static[HostProfile]("Call vault_create with a profile name; poll vault_create_resume with the returned handle; confirm with vault_status until unlocked.").
				WhenPred(appsGate(AppIs("vault_create")),
					"On this host you can also call open_app with app=\"vault_create\" to render the interactive vault creation wizard."))).
		Flow(mcpforge.Flow[HostProfile]("vault_restore", "Restore a vault").
			Steps("vault_restore", "vault_restore_resume", "vault_status").
			Detail(mcpforge.Static[HostProfile]("Call vault_restore; poll vault_restore_resume with the returned handle; confirm with vault_status until unlocked.").
				WhenPred(appsGate(AppIs("vault_restore")),
					"On this host you can also call open_app with app=\"vault_restore\" to render the interactive restore wizard."))).
		Flow(mcpforge.Flow[HostProfile]("upload", "Upload new content (creates + pins)").
			Steps("capabilities").
			// The byte route is a decision, not a fixed upload_file: a model
			// that reads steps first still sees upload_url / upload_data as the
			// route for a public URL / raw inline bytes. The mint tail (<host
			// PUT> + upload_status) lives inside the mint branch; <host PUT> is
			// an out-of-band action, not an MCP tool, but naming it keeps the
			// chain from looking complete at the mint response.
			Decision(byteRouteDecision(nil)).
			Detail(uploadDetailDesc)).
		Flow(mcpforge.Flow[HostProfile]("vault_upload", "Store a file in a vault").
			Steps("capabilities").
			// Vault storage is always vault_put_file (no vault-from-CID tool);
			// the decision surfaces the byte source so steps-first models do not
			// route vault bytes through the IPFS-only upload_url / upload_data.
			Decision(vaultByteRouteDecision()).
			Detail(vaultUploadDetailDesc)).
		Flow(mcpforge.Flow[HostProfile]("download", "Download IPFS content to a file").
			Steps("capabilities", "download_file").
			Detail(downloadDetailDesc)).
		Flow(mcpforge.Flow[HostProfile]("vault_download", "Download a file from a vault").
			Steps("capabilities", "vault_get_file").
			Detail(vaultDownloadDetailDesc)).
		Flow(mcpforge.Flow[HostProfile]("vault_share", "Share from a vault").
			Steps("vault_status", "vault_share", "vault_verify").
			Detail(mcpforge.Static[HostProfile]("Ensure the vault is unlocked (vault_status), then call vault_share with the vault_path to generate a shareable link (control its lifetime with expiry). Local reads (vault_get_file / vault cat / vault_stats) work any time after a staged PUT; only share/send require durability across profiles. Only durable (status: durable) files can be shared: if vault_share or vault_send returns {code:'not_durable', ...}, run vault_flush (non-blocking, returns an accepted job { job_id, profile, path? }), poll vault_flush_status(job_id) or vault_stat until status: durable, then share/send again. If a file stays non-durable across polls, read vault_stat's flush_started_at, flush_attempts and flush_error: a flushing file shows a flush_started_at and a rising flush_attempts with no error, a failed file shows flush_attempts plus a non-empty flush_error, and a staged file that never started shows zero attempts/no error and an empty flush_started_at — compare now against flush_started_at to tell a long host upload from a hung pin. The recipient accepts the share with vault_share_accept (accept_state 'pinned' — an independent pin of the same object key, NOT a digest failure), which is directly visible on tools/list; vault_verify on a freshly pinned object reports digest_verified 'not_applicable' until first get/decrypt/deep verify — treat accept_state 'pinned' (not a digest signal) as the success indicator. For multi-profile swarms, list profiles with vault_profiles and hand off a file with vault_send (or pass profile=<name> when more than one profile is unlocked — vault ops return profile_required otherwise)."))).
		Flow(mcpforge.Flow[HostProfile]("vault_sync", "Sync and verify vault state").
			Steps("vault_status", "vault_sync", "vault_verify").
			Detail(mcpforge.Static[HostProfile]("vault_sync reconciles the local vault cache from the indexer; vault_verify checks file integrity. Run both after creating or restoring on a new device, or when share state may have changed. Related utilities are discoverable via search_tools(category=vault): vault_ls, vault_stat, vault_tag_add, vault_tag_rm, vault_version_restore."))).
		Flow(mcpforge.Flow[HostProfile]("pins", "Manage pins").
			Steps("pins_add", "pins_list", "pins_status", "pins_rm").
			Detail(mcpforge.Static[HostProfile]("pins_add imports content already on IPFS by external CID; it is NOT for use after an upload tool (which already pins). pins_status takes one cid; pins_rm requires confirm and exactly one of cids or all."))).
		Flow(mcpforge.Flow[HostProfile]("publish_website", "Publish a website").
			// The byte route comes first (real upload tools produce the CID),
			// then the domain/websites_create choice is nested under each branch.
			// Every step here is a real tool, so the guide's "steps resolve to
			// real tools" invariant holds on every host.
			Decision(byteRouteDecision(publishDomainDecision()))).
		Flow(mcpforge.Flow[HostProfile]("ens_publish", "Point an ENS/onchain domain at IPFS content").
			// ENS domains do not use the website system — they resolve via an
			// IPNS-based contenthash set onchain in the ENS resolver. The byte
			// route reuses the existing upload chooser to produce a CID, then
			// ens_point publishes it under the domain's IPNS key and returns
			// the contenthash + wallet guidance. ens_point/ens_unpoint are
			// behind progressive disclosure (never curated), so name them here
			// and steer the agent to search for them rather than expecting
			// them on tools/list. The final contenthash is set by the USER's
			// wallet/ENS manager — the agent surfaces the value and options,
			// never assumes a specific wallet.
			Decision(byteRouteDecision(
				mcpforge.Decision[HostProfile]("Point the ENS name at the CID?",
					mcpforge.Branch[HostProfile]("Yes — point the ENS/onchain domain at the content").
						Steps("ens_point").
						Detail(mcpforge.Static[HostProfile]("Search for ens_point (search_tools query \"ens\"), then call it with the onchain domain (e.g. vitalik.eth) and the cid from the upload. It creates or reuses the domain's IPNS key, publishes the CID, and returns the contenthash (ipns://<ipns-name>) plus a verify URL (eth.limo for .eth). The returned next_steps are onchain: the user sets the ENS resolver's contenthash field to the returned value from their own wallet or the ENS manager (app.ens.domains), the ENS SDK (ethers.js), or a wallet with ENS support. Do NOT assume a specific wallet. After the onchain transaction confirms, verify at the returned verify URL.")),
					mcpforge.Branch[HostProfile]("No — only publish the content to IPFS/IPNS, no onchain pointing").
						Steps("websites_create").
						Detail(mcpforge.Static[HostProfile]("Treat it as a normal website publish: websites_create with the cid. ENS pointing is only applied when the user explicitly wants their ENS name to resolve to the content.")),
				),
			))).
		Flow(mcpforge.Flow[HostProfile]("update_website", "Update an existing website").
			Steps("websites_get", "websites_update", "websites_validate").
			Detail(mcpforge.Static[HostProfile]("Update a deployed website's content without recreating it. 1) websites_get <domain> first to capture the current target_type and dns_hosting_enabled — never guess them. 2) If the new CID was never uploaded through Pinner, pins_add it first; if CID_NOT_PINNED appears right after an upload the gateway is still propagating the pin — wait a few seconds and retry the update. 3) websites_update <domain> with the new cid (target-type is inherited when omitted; change it only when intentionally switching IPFS<->IPNS). 4) websites_validate. If DNS hosting is managed, validation may report the old CID right after the update — that is reconciliation lag, not failure; re-call websites_validate without starting a new flow.").
				Then(cdnDeployNoticeClause))).
		Resolve(p)

	// The resolved guide is filtered to the server's domain scope: flows whose
	// underlying tools are not registered on this scope (e.g. the Sia vault
	// flows on a hosted server) are dropped so the guide never advertises an
	// unregisterable action.
	return filterGuideFlows(spec, scope)
}

// flowScope maps each agent_guide flow name to the domain-scope flag that
// gates it. Flows not listed are gated by no flag (always kept).
var flowScope = map[string]func(assembly.DomainScope) bool{
	"auth":            assembly.DomainScope.AccountOn,
	"vault_create":    assembly.DomainScope.VaultOn,
	"vault_restore":   assembly.DomainScope.VaultOn,
	"vault_upload":    assembly.DomainScope.VaultOn,
	"vault_download":  assembly.DomainScope.VaultOn,
	"vault_share":     assembly.DomainScope.VaultOn,
	"vault_sync":      assembly.DomainScope.VaultOn,
	"upload":          assembly.DomainScope.UploadOn,
	"download":        assembly.DomainScope.UploadOn,
	"pins":            assembly.DomainScope.PinsOn,
	"publish_website": assembly.DomainScope.WebsitesOn,
	"update_website":  assembly.DomainScope.WebsitesOn,
	"ens_publish":     assembly.DomainScope.ENSOn,
}

// filterGuideFlows drops resolved flows whose scope flag is disabled.
func filterGuideFlows(guide AgentGuide, scope assembly.DomainScope) AgentGuide {
	if scope.IsZero() {
		return guide
	}
	kept := guide.Flows[:0]
	for _, f := range guide.Flows {
		if gate, ok := flowScope[f.Name]; ok {
			if !gate(scope) {
				continue
			}
		}
		kept = append(kept, f)
	}
	guide.Flows = kept
	return guide
}

// agentGuideDescription is shared between the static Description (tools/list)
// and the direct-only presentation surface: it is a direct-only tool outside
// the operation catalog and never enters the compiled surface.
// agentGuideDescription positions the guide as OPTIONAL orientation: the
// description must not recommend broad triggering (a blanket "call this
// first" directive can override an explicit request already served by a
// specific tool), so it defers to directly relevant tools for clear intents.
const agentGuideDescription = "Orientation material for agents: the primary Pinner flows (auth, vault_create, vault_restore, upload, vault_upload, download, vault_download, vault_share, vault_sync, pins, publish_website, ens_publish) as ordered tool chains or decision trees, plus operational rules. When the server has app views registered, the guide includes open_app as the single launcher for the human-facing interactive views actually installed. Optional orientation when unfamiliar with Pinner or driving a multi-step flow; for an explicit, already-clear request, prefer the directly relevant tool instead of consulting the guide."

// AgentGuideDescriptor returns a static, no-input tool that orients an agent
// to the primary Pinner flows and how to chain them. It is deterministic
// structured guidance, so a model does not have to discover the flows by
// probing tool descriptions. The guide content is composed via the platform
// DSL and adapted based on the calling client's platform profile so
// file-input and download-sink guidance match the transport's capabilities;
// because it is host-aware it is re-resolved per request rather than at
// assembly. scope and hosted are the assembled server's construction-time
// properties (Config fields) — the de-globalized replacements for the source
// package's activeSurface()/activeHosted() read.
//
// dropSinkAvailable is the registration-true drop-sink eligibility derived
// from the transfer wiring (FileDrop != nil && !tunnelOpenAI) — the exact
// condition the capabilities report's sinkModesFor and the download_file
// tool's drop gate apply. When false, the per-request profile is stripped of
// FeatSinkDrop before resolution, mirroring assemble's strip closure: with no
// FileDrop coordinator wired no download tool accepts sink=drop, so the
// guide's drop-bearing prose must not render either.
//
// installedApps is the assembly's registered app-view inventory;
// Config.InstalledApps supplies it (empty by default). Every open_app-bearing
// clause gates on it, so the guide claims the open_app launcher only when the
// launching surface actually exists on the assembled server.
func AgentGuideDescriptor(scope assembly.DomainScope, hosted bool, dropSinkAvailable bool, installedApps []string) model.ToolDescriptor {
	return model.ToolDescriptor{
		Name:          "agent_guide",
		Title:         "Pinner agent guide",
		Description:   agentGuideDescription,
		Category:      model.CategoryCore,
		OpenWorldHint: false, // static local guidance payload; changes no state
		InputSchema:   toolargs.ToolSchemaFor[noInput](),
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			profile := profileFromRequest(request)
			profile.Hosted = hosted
			if !dropSinkAvailable {
				profile = profile.CloneFeatures()
				// The startup copy set (assemble.go) strips the feature for
				// the same wiring fact; per-request wire profiles may still
				// declare FeatSinkDrop on hosts, so re-apply the gate here.
				delete(profile.Features, FeatSinkDrop)
			}
			guide := BuildAgentGuide(profile, scope, hosted, installedApps)
			return model.ToolResult{StructuredContent: guide, Text: toolargs.ResultJSONText(guide)}, nil
		},
	}
}

// noInput is the typed argument shape for tools that take no input. It is the
// shared empty-input convention: the reflected schema is an empty object.
type noInput struct{}
