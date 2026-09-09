package catalogmcp

// opTargets maps an operation ID to its MCP per-profile targets.
// Every entry carries a Fallback (universal, feature-independent) target so
// description resolution always succeeds; websites_create additionally
// routes through the feature-gated description DSL (websitesCreateDesc) via
// FallbackFunc.
var opTargets = map[string][]Target{
	"account_info": {
		Fallback("Call account_info to read the authenticated user's profile (email, name, user id, verified, otp_enabled). Read-only."),
	},
	"account_otp_disable": {
		Fallback("Call account_otp_disable to turn off the account's two-factor authentication. Requires the user's current account password."),
	},
	"account_quota": {
		Fallback("Call account_quota to read the account's quota status and whether it is covered by granted usage (has_quota). Quota trumps a subscription: when has_quota is true the user needs no subscription. When has_quota is false the result relates to account_subscription; when that reports not-subscribed the response carries a web_url deep-link that the human opens in the web app to subscribe — the model acting alone cannot subscribe on their behalf."),
	},
	"account_subscription": {
		Fallback("Call account_subscription to read the user's active subscription status and obtain the web_url deep-link to https://account.<portal>/account/subscription where they sign in and manage/subscribe. The URL is returned as data; a human must open it in a browser to actually subscribe or change their plan."),
	},
	"account_update_email": {
		Fallback("Call account_update_email to change the account's email address. Requires the current password for verification. On success the user must confirm via the verification email sent to the new address."),
	},
	"account_update_password": {
		Fallback("Call account_update_password to change the account's password. Requires the current password and a new password."),
	},
	"auth_login": {
		Fallback("Call auth_login with a pre-issued auth token (JWT) to store it as the active Pinner.xyz credential. The token argument is sensitive. Returns {status, user_id?, message}. This channel accepts an auth token only, not a password or OTP; interactive sign-in goes through auth_sso."),
	},
	"auth_logout": {
		Fallback("Call auth_logout to clear the locally stored Pinner.xyz credential. Returns {status: logged_out | not_authenticated, config_path?, message}. Note this only clears the local token; it does not revoke server-side API keys."),
	},
	"auth_status": {
		Fallback("Call auth_status to verify the stored Pinner.xyz credential is present and valid before running authenticated operations. Returns {authenticated: bool, email?, user_id?, message?}. When authenticated is false, steer the human to the out-of-band sign-in flow (auth_sso -> auth_resume) rather than asking for a password or OTP on this channel."),
	},
	"vault_create": {
		Fallback("Provision a new vault under a profile and hand the host off to a human. The create is completed out-of-band: the human opens the returned create_url, approves the Sia device connection in a browser, and retrieves the one-time recovery seed. Poll the returned vault_create_resume handle until the vault is active and the seed has been retrieved. The plaintext mnemonic never appears on this channel."),
	},
	"vault_flush": {
		Fallback("Make staged vault files durable on Sia. vault_put_file returns before bytes are on Sia (status: staged); call this to kick off upload + pin. Flush all staged files by default, or pass path to flush a single file. This tool is non-blocking and runs on a per-profile flush worker (one worker goroutine per profile, never a shared global queue). It returns a job { job_id, profile, path? } with status accepted — poll vault_flush_status(job_id) or vault_stat until status is durable, then vault_share. A full host-set upload takes time, so the immediate accepted response is not completion. If a file stays non-durable across polls, vault_stat's flush_started_at, flush_attempts and flush_error describe its progress: a flushing file shows a flush_started_at and a rising flush_attempts with no error, a failed file shows flush_attempts plus a non-empty flush_error, and a staged file that never started shows zero attempts/no error and an empty flush_started_at. Comparing now against flush_started_at distinguishes a long-but-progressing host upload from a hung pin."),
	},
	"vault_flush_status": {
		Fallback("Check the status of a flush job accepted by vault_flush, given its job_id. Returns { job_id, profile, path?, status (queued|running|done|failed), flushed?, error?, started_at? }. A done job means the targeted staged file(s) are now durable; a failed job carries an error whose cause is visible in vault_stat's flush_error, after which vault_flush can be re-run. started_at is the RFC3339 time the worker began the job, so a job stuck in 'running' longer than your hang threshold (compare against vault_stat's flush_started_at on the file) is a hung pin, not just a slow upload. Requires a wired flush manager; without one this errors with code no_flush_manager."),
	},
	"vault_profiles": {
		Fallback("List every provisioned/unlocked vault profile name the server can access: { profiles: [<names>] }. On a multi-profile server (more than one unlocked profile), vault ops without an explicit profile= argument return code profile_required instead of silently hitting the active vault; pass the name from this list. Read-only."),
	},
	"vault_restore": {
		Fallback("Start an out-of-band vault restore for a profile. An out-of-band restore_url is returned for the human to open in a browser and enter the recovery seed to complete the restore; poll the returned vault_restore_resume handle until done. The seed never appears on this channel."),
	},
	"vault_send": {
		Fallback("Send a durable vault file from one profile to another in the same process (swarm handoff). Requires path (vault:/ source), from_profile, to_profile (two distinct unlocked profiles), and dest_path (vault:/ destination); tags is an optional durable tag list. The server flushes-if-needed — it does NOT block on a long Sia upload in the request: if the source is not yet durable it returns {code:'not_durable', path, status, message}, and the source is made durable by running vault_flush, then polling vault_flush_status(job_id) or vault_stat until durable, before calling vault_send again. Once durable it mints a 24h share from the source, accepts it into the destination profile (metadata-only pin of the same object key — no full decrypt, no long client-side sleep), and returns once the destination row exists: {from_profile, to_profile, dest_path, object_key, size, accept_state:'pinned'}. Accept-state pinned — NOT a digest failure — is the success signal; the destination will deep-verify on first get/decrypt."),
	},
	"vault_share": {
		Fallback("Generate a share link for a durable vault file. Returns a time-limited pre-signed https:// URL whose fragment carries the object's encryption key (#encryption_key=…). Pass this URL unchanged to vault_share_accept on another profile to pin slab references — a metadata-only operation that transfers no content. Donor sealed metadata is NOT put into the share URL; the fragment still only carries the encryption key. A link works only for a durable file: if the file is staged/flushing/failed this returns a structured {code:\"not_durable\", path, status, message} result (failed means the durability flush failed and sharing will not work until the file is re-uploaded; a staged/flushing file needs its flush completed via vault_flush, then vault_flush_status or vault_stat until durable, or vault_send). Bound how long the link works with the expiry field (e.g. 7d, 30d, 1h, or 0 for never)."),
	},
	"vault_share_accept": {
		Fallback("Accept a share URL issued by another agent/profile and pin its slab references into this profile's vault. Metadata-only — no content is downloaded from Sia hosts, so it completes quickly regardless of file size. Accept creates an independent pin of the same object key (a metadata-only slab-reference copy), NOT a rewritten object, so accept_state is 'pinned' and digest_verified is 'not_applicable' until the acceptor first gets/decrypts or deep-verifies the content. The accepting profile owns an independent object referencing the same sectors, so the content survives even if the sharer deletes theirs. The share URL's scheme and host are rewritten to this profile's indexer origin before any request is made, so the agent never needs to validate or transform the URL. Accepting the same share at different paths creates multiple local rows referencing the same indexer object (like hard-links); PinObject is idempotent, so duplicate accepts of the same slabs are a no-op on the indexer. path is the vault:/ destination for the pinned copy."),
	},
	"vault_stat": {
		Fallback("Show metadata for a single vault path: type, size, media type, content digest, object ID, and current status (staged | flushing | durable | failed). Returns metadata only, never the content. While a file is not yet durable (staged/flushing/failed) the result carries flush_attempts, flush_error and flush_started_at: a flushing file shows a flush_started_at and a rising flush_attempts with no error, a failed file shows flush_attempts plus a non-empty flush_error, and a staged file that has never started shows zero attempts/no error and an empty flush_started_at. Compare now against flush_started_at to tell a long-but-progressing host upload from a hung pin. Once durable, these fields are omitted."),
	},
	"vault_verify": {
		Fallback("Verify a vault file's integrity: checks that the object exists on the Sia indexer and compares the recorded SHA-256 digest. Returns digest_verified (verified/unverified/mismatch/not_applicable), digest_match, object_exists, and the recorded digest. An accepted share or vault_send has no digest until first decrypt/get/deep verify; in that state digest_verified is 'not_applicable' (a neutral no-verdict-yet — NOT a failure), so treat the pin as successful and resolve the digest on first get or a deep=true verify. Use deep=true to download the full content, recompute the hash, and backfill the digest if missing. Does NOT stream or return file content."),
	},
	"websites_create": websitesCreateTargets,
	"websites_platform_domain_availability": {
		Fallback("Check whether a candidate subdomain label is claimable on each enabled platform (free-subdomain) root. label is required. Returns one availability result per platform-owned root. The check applies when a concrete subdomain label has already been supplied by the user or is required by an explicit user request for custom naming. A label is not generated solely for this check; when no label preference exists, websites_create with no domain auto-generates a platform subdomain."),
	},
	"websites_platform_domains_list": {
		Fallback("List the platform-owned root domains that are enabled and available for users to claim free subdomains under. Relevant when the user explicitly requests a specific subdomain label — discover roots here before checking availability with websites_platform_domain_availability. When the user has no label preference, websites_create with no domain auto-generates a platform subdomain."),
	},
	"websites_update": {
		Fallback("Update an existing website: change its cid, target-type (ipfs|ipns), rename its domain (rename-to), set the domain namespace (namespace: icann or hns for a Handshake/alt-root name), or set dns-hosting (true = Pinner-managed, false = self-managed, omit = unchanged). Select the site by website; set at least one optional field. A CID produced by an upload tool is already pinned and usable directly. A CID that is an EXTERNAL IPFS CID needs pins_add first; a bare update with an unpinned CID fails with CID_NOT_PINNED. With only cid set (no target-type), the site's current target type is preserved automatically. For the guided flow, the website-update prompt (prompts/get website-update) provides the decision tree."),
	},
	"websites_validate": {
		Fallback("Validates that a website's DNS records are correctly configured (TXT validation token + _dnslink). Call this after websites_create to confirm DNS propagation. For managed-DNS platform subdomains, validation typically passes within 30-60s of creation; if it fails, wait and retry rather than treating it as a creation failure. For self-managed DNS, ensure the _dnslink TXT and validation TXT are published before calling."),
	},
}
