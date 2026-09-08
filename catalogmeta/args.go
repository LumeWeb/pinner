// Package catalogmeta carries the frontend (CLI/MCP) presentation metadata
// that was split out of the catalogops operation definitions during the
// opmesh migration, keyed by the stable operation ID.
//
// The core operation model is now go.lumeweb.com/opmesh, which deliberately
// owns only the frontend-neutral vocabulary (IDs, typed args, codecs,
// normalization/defaults/validation, effect classification, actor +
// confirmation policy, registry execution) and must never grow
// audience-specific fields. Everything that describes how a specific
// frontend presents an operation lives here instead:
//
//   - EnvironmentOf: which runtime surfaces (CLI, local MCP, hosted MCP) an
//     operation is valid on (EnvCLIOnly / EnvLocalOnly / EnvHostedOnly flags).
//   - ArgFrontendFor: per-argument audience metadata — AgentHelp (agent-surface
//     arg prose), AgentOnly (agent-surface-only arg), PositionalOnly (arg fed by
//     the CLI positional rather than a --flag), Sources (CLI env-var sources).
//
// The MCP per-operation tool targets/descriptions (MCPTargets, FallbackFunc,
// the feature-gated description DSL and the profile adapter) live in the
// sibling catalogmcp package, which also owns the mcpforge dependency; this
// package is stdlib-only so assembly layers can do surface gating without
// pulling in any MCP machinery.
//
// A consumer (a CLI compiler or MCP compiler in pinner-cli, for example)
// looks metadata up by operation ID: absent entries mean "no frontend
// specialization", which is the overwhelmingly common case.
//
// The data in this file preserves, verbatim, the metadata that previously
// lived inline on the catalogops operation definitions; it was extracted
// mechanically during the opmesh migration and is not interpreted here.
package catalogmeta

// ArgFrontend is the frontend presentation metadata for one operation
// argument. Field meanings mirror the former pinner.OperationArg fields of
// the same names:
//
//   - AgentHelp: agent-oriented help prose (audience separation from the
//     generic Help carried by the core model).
//   - AgentOnly: the arg is exposed on the agent/MCP surface only; a CLI
//     invocation never supplies it and the handler must tolerate absence.
//   - PositionalOnly: the arg value is supplied by the command's positional
//     argument rather than a --flag (only meaningful for arguments of
//     operations that declare a Positional binding).
//   - Sources: env vars that also source the arg's value on the CLI surface.
type ArgFrontend struct {
	Name           string
	AgentHelp      string
	AgentOnly      bool
	PositionalOnly bool
	Sources        []string
}

// argFrontend maps an operation ID to the frontend metadata of its arguments,
// in declared order. Operations absent from the map declare no frontend
// argument specialization.
var argFrontend = map[string][]ArgFrontend{
	"account_otp_disable": {
		{Name: "password", AgentHelp: "The user's current account password, used to verify disabling two-factor authentication."},
	},
	"account_update_email": {
		{Name: "email", AgentHelp: "The new email address for the account."},
		{Name: "password", AgentHelp: "The user's current account password, used to verify the change."},
	},
	"account_update_password": {
		{Name: "current_password", AgentHelp: "The user's current account password."},
		{Name: "new_password", AgentHelp: "The new password to set for the account."},
	},
	"admin_billing_credits_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_credits_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_credits_restore": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_credits_user_balance": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_credits_user_deleted_credits": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_price_lines_add_plan": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_price_lines_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_price_lines_delete_plan": {
		{Name: "price-line-id", PositionalOnly: true},
		{Name: "plan-id", PositionalOnly: true},
	},
	"admin_billing_price_lines_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_price_lines_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_price_lines_update_plan_position": {
		{Name: "price-line-id", PositionalOnly: true},
		{Name: "plan-id", PositionalOnly: true},
	},
	"admin_billing_pricing_plan_periods_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_pricing_plan_periods_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_pricing_plan_periods_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_pricing_plans_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_pricing_plans_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_pricing_plans_sync": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_pricing_plans_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_subscribers_abort_cancel": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_subscribers_cancel": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_subscribers_change_plan": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_subscribers_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_billing_subscribers_list_gateway": {
		{Name: "gateway-id", PositionalOnly: true},
	},
	"admin_billing_subscribers_list_user": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_subscribers_pause": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_billing_subscribers_resume": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_platform_domains_bind": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_platform_domains_delete": {
		{Name: "id", PositionalOnly: true},
		{Name: "confirm", AgentHelp: "Must be true to delete the platform domain; this is destructive and cannot be undone. Only a human sets this on confirmation; a model alone cannot confirm a destructive delete."},
	},
	"admin_platform_domains_register": {
		{Name: "domain", PositionalOnly: true},
		{Name: "namespace", PositionalOnly: true},
	},
	"admin_platform_domains_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_allowances_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_allowances_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_plans_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_plans_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_plans_set_default": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_plans_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_quota_user_configs_reset": {
		{Name: "user-id", PositionalOnly: true},
	},
	"admin_social_providers_delete": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_social_providers_disable": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_social_providers_enable": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_social_providers_get": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_social_providers_update": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_websites_block": {
		{Name: "id", PositionalOnly: true},
	},
	"admin_websites_unblock": {
		{Name: "id", PositionalOnly: true},
	},
	"api_keys_delete": {
		{Name: "id", AgentHelp: "The name or UUID of the API key to delete."},
		{Name: "confirm", AgentHelp: "Set true to delete the API key even if it is the one currently used for authentication."},
	},
	"auth_login": {
		{Name: "token", AgentHelp: "The Pinner.xyz auth token (JWT) to store as the active credential. Sensitive value."},
	},
	"dns_records_create": {
		{Name: "zone", PositionalOnly: true},
	},
	"dns_records_delete": {
		{Name: "zone", PositionalOnly: true},
		{Name: "confirm", AgentHelp: "Must be true to delete the record; this is destructive and cannot be undone."},
	},
	"dns_records_get": {
		{Name: "zone", PositionalOnly: true},
	},
	"dns_records_list": {
		{Name: "zone", PositionalOnly: true},
	},
	"dns_records_update": {
		{Name: "zone", PositionalOnly: true},
	},
	"dns_zones_delete": {
		{Name: "zone", PositionalOnly: true},
		{Name: "confirm", AgentHelp: "Must be true to delete the zone; this is destructive and cannot be undone."},
	},
	"dns_zones_get": {
		{Name: "zone", PositionalOnly: true},
	},
	"dns_zones_validate": {
		{Name: "zone", PositionalOnly: true},
	},
	"ens_point": {
		{Name: "name", AgentHelp: "The onchain/ENS domain to point, e.g. vitalik.eth. Do not invent one; use the name the user provided."},
	},
	"ens_unpoint": {
		{Name: "confirm", AgentHelp: "Must be true to delete the key; this is destructive and cannot be undone. Only a human sets this on confirmation; a model alone cannot confirm a destructive delete."},
	},
	"ipns_keys_delete": {
		{Name: "confirm", AgentHelp: "Must be true to delete the key; this is destructive and cannot be undone. Only a human sets this on confirmation; a model alone cannot confirm a destructive delete."},
	},
	"operations_list": {
		{Name: "search", AgentHelp: "Full-text search term evaluated server-side against operation type, status, protocol, or CID. Composes (AND) with the structured filters."},
		{Name: "status", AgentHelp: "One or more statuses to filter by. Valid values: pending, processing, completed, failed, duplicate. When omitted, only active operations (pending, processing) are returned unless all=true."},
		{Name: "all", AgentHelp: "When true, return operations in any status, overriding the default that shows only pending and processing. Ignored when status is explicitly provided."},
		{Name: "sort", AgentHelp: "Sort field and direction, e.g. \"id:desc\" or \"started:asc\". Defaults to id:desc."},
	},
	"pins_add": {
		{Name: "cids", AgentHelp: "One or more concrete CIDs to pin. This field is required; supply the values here."},
	},
	"pins_rm": {
		{Name: "cids", AgentHelp: "Concrete CIDs to unpin. Omitted only when removing all pins (all=true). The field takes concrete values; CLI positional/file/stdin syntax is not used here."},
		{Name: "confirm", AgentHelp: "Must be true to remove pins; this is destructive and cannot be undone."},
	},
	"pins_status": {
		{Name: "cid", AgentHelp: "The concrete CID whose pin status to return."},
	},
	"pins_update": {
		{Name: "cid", AgentHelp: "The concrete CID of the pin to update."},
	},
	"vault_flush": {
		{Name: "path", AgentHelp: "An optional vault:/ path restricting the flush to a single file; when omitted, every staged file is flushed."},
	},
	"vault_flush_status": {
		{Name: "job_id", AgentHelp: "The job_id returned by vault_flush."},
	},
	"vault_forget": {
		{Name: "profile", AgentHelp: "The name of the vault profile to remove. Always required; this tool never auto-resolves a default profile."},
		{Name: "confirm", AgentHelp: "Must be true to forget the profile; this permanently deletes local vault data and cannot be undone."},
	},
	"vault_ls": {
		{Name: "path", AgentHelp: "The vault path to list. Append a trailing slash (vault:/a/b/) to list a subdirectory; without it, a non-root path is assumed to be a file path and lists the parent. If the directory is empty, the tool auto-retries as a directory."},
	},
	"vault_profile_use": {
		{Name: "name", AgentHelp: "The vault profile name to set as the default for subsequent vault operations."},
	},
	"vault_rm": {
		{Name: "path", AgentHelp: "The vault:/ path of the file to delete."},
		{Name: "confirm", AgentHelp: "Must be true to delete the file; this is destructive and cannot be undone."},
	},
	"vault_search": {
		{Name: "query", AgentHelp: "A substring of the file name to match (case-insensitive)."},
		{Name: "tag", AgentHelp: "One or more tags; a file must have all of them. Repeat for multiple."},
		{Name: "tag_any", AgentHelp: "A file must have at least one of these tags."},
		{Name: "dir", AgentHelp: "A vault directory to restrict results to (inclusive)."},
		{Name: "where", AgentHelp: "A list of predicates to filter by. Items are ANDed. Each object has exactly one field (or not): tag/status/source/host/agent/dir accept a string OR a list (a list means match ANY of them); since/before are scalar times (RFC3339 or YYYY-MM-DD)."},
	},
	"vault_send": {
		{Name: "path", AgentHelp: "The vault:/ source path whose durable object to hand off."},
		{Name: "dest_path", AgentHelp: "The vault:/ destination path where the source object should be pinned in to_profile."},
		{Name: "from_profile", AgentHelp: "The unlocked profile that owns the source object (required)."},
		{Name: "to_profile", AgentHelp: "The unlocked profile that will own the pinned destination copy (required)."},
		{Name: "tags", AgentHelp: "Optional durable tags applied at accept time on the destination row."},
	},
	"vault_share": {
		{Name: "path", AgentHelp: "The vault:/ path to the file to share."},
	},
	"vault_share_accept": {
		{Name: "share_url", AgentHelp: "The https:// share URL you received from vault_share. It is time-limited and carries the encryption key in its fragment (#encryption_key=…). Pass it through unchanged."},
		{Name: "path", AgentHelp: "The vault:/ destination path where the accepted copy should be pinned."},
		{Name: "tags", AgentHelp: "Tags applied atomically at write time — durable on the sealed object and local tag index. Eliminates the need for a separate vault_tag_add call."},
	},
	"vault_stat": {
		{Name: "path", AgentHelp: "The vault:/ path to report on."},
	},
	"vault_tag_add": {
		{Name: "path", AgentHelp: "The vault:/ path of the file to tag."},
		{Name: "tags", AgentHelp: "One or more tags to add to the file."},
	},
	"vault_tag_rm": {
		{Name: "path", AgentHelp: "The vault:/ path of the file."},
		{Name: "tags", AgentHelp: "One or more tags to remove from the file."},
	},
	"vault_tag_set": {
		{Name: "path", AgentHelp: "The vault:/ path of the file."},
		{Name: "tags", AgentHelp: "The exact tag set; omit to clear all tags."},
	},
	"vault_verify": {
		{Name: "path", AgentHelp: "The vault:/ path to verify."},
	},
	"vault_version_get": {
		{Name: "path", AgentHelp: "The vault:/ path of the file."},
		{Name: "version_id", AgentHelp: "The version_id to inspect (from vault_version_ls)."},
	},
	"vault_version_ls": {
		{Name: "path", AgentHelp: "The vault:/ path whose version history to list."},
	},
	"vault_version_restore": {
		{Name: "path", AgentHelp: "The vault:/ path of the file to restore into."},
		{Name: "version_id", AgentHelp: "The version_id to restore (from vault_version_ls)."},
		{Name: "confirm", AgentHelp: "Set to true to confirm the destructive restore. An agent may self-confirm this restore headlessly with confirm=true."},
	},
	"websites_create": {
		{Name: "website", AgentHelp: "The destination domain. A subdomain of a platform root (e.g. myapp.pinned.site) is treated as a platform claim (label/root parsed); any other domain is a custom domain. Omit to default to a minted platform subdomain."},
		{Name: "cid", AgentHelp: "The IPFS CID to serve. A CID returned by a Pinner upload tool is already pinned and can be used directly. A CID external to Pinner requires pinning first via pins_add."},
		{Name: "dns-hosting", AgentHelp: "true lets Pinner manage DNS; false leaves DNS self-managed. Omit to use the default. Ignored for platform subdomains (always managed)."},
		{Name: "namespace", AgentHelp: "The namespace of the custom domain: \"icann\" (traditional, default) or \"hns\" for a Handshake (alt-root) name like acme/. Only applies to the custom-domain path; platform subdomains derive their namespace from the platform root."},
		{Name: "platform", AgentHelp: "Set true to force a platform (free) subdomain claim. Pair with label or generate; omit the domain positional. The type is otherwise derived automatically.", AgentOnly: true},
		{Name: "platform-domain", AgentHelp: "The platform root to claim a free subdomain under (e.g. pinned.site). Optional; when set, restricts the claim to this root. Use with platform plus label or generate.", AgentOnly: true},
		{Name: "platform-namespace", AgentOnly: true},
		{Name: "generate", AgentHelp: "Set true to let the platform auto-generate the subdomain label. Mutually exclusive with label.", AgentOnly: true},
		{Name: "label", AgentHelp: "Explicit subdomain label to claim under a platform domain. Mutually exclusive with generate.", AgentOnly: true},
	},
	"websites_delete": {
		{Name: "confirm", AgentHelp: "Must be true to delete the website; this is destructive and cannot be undone."},
	},
	"websites_domains_add": {
		{Name: "website", AgentHelp: "The website (ID or domain) to bind the domain to. Omit to auto-select when the account has exactly one website."},
		{Name: "namespace", Sources: []string{"PINNER_DOMAIN_NAMESPACE"}},
	},
	"websites_domains_convert_onchain": {
		{Name: "confirm", AgentHelp: "Must be true to convert the domain to on-chain managed; this drops Pinner's managed zone/DNSSEC and is one-way. Only a human sets this on confirmation; a model alone cannot confirm a destructive operation."},
	},
	"websites_update": {
		{Name: "cid", AgentHelp: "The IPFS CID to serve. A CID produced by a Pinner upload tool is already pinned and used directly. A CID that is an external IPFS CID requires pins_add(cids=[\"<cid>\"], wait=true) first; an unpinned CID fails with CID_NOT_PINNED. With a bare cid (no target-type), the site's current targeting is preserved automatically."},
		{Name: "dns-hosting", AgentHelp: "true enables Pinner-managed DNS; false disables it (self-managed). Omit to leave the current DNS hosting state unchanged."},
		{Name: "namespace", AgentHelp: "The DNS namespace of the custom domain: \"icann\" (traditional) or \"hns\" for a Handshake (alt-root) name. Omit to leave the current namespace unchanged. When renaming to an HNS name with rename-to, set namespace to \"hns\"."},
	},
}

// ArgFrontendFor returns the frontend metadata for the arguments of the
// operation named by opID. It returns nil for operations with no frontend
// argument specialization; callers treat a nil result as "every arg is a
// plain generic arg" (the default).
func ArgFrontendFor(opID string) []ArgFrontend {
	meta := argFrontend[opID]
	if meta == nil {
		return nil
	}
	out := make([]ArgFrontend, len(meta))
	copy(out, meta)
	return out
}

// ArgFrontendIDs returns every operation ID that declares frontend argument
// metadata. It exists so integrity/drift guards (and boundary consumers) can
// enumerate the keyed table without reaching into package state.
func ArgFrontendIDs() []string {
	ids := make([]string, 0, len(argFrontend))
	for id := range argFrontend {
		ids = append(ids, id)
	}
	return ids
}

// ArgFrontendForArg returns the metadata for a single argument by operation ID
// and arg name, or nil when none is declared.
func ArgFrontendForArg(opID, argName string) *ArgFrontend {
	for _, a := range ArgFrontendFor(opID) {
		if a.Name == argName {
			a := a
			return &a
		}
	}
	return nil
}
