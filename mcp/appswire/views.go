package appswire

import (
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/pinner/canvas"
)

// Capability is a deployment-provided facility a view's install depends on.
// Capabilities are WIRING facts, not deployment-kind facts: a hosted
// (Portal-embedded) assembly can wire the OOB browser-handoff coordinators as
// well as a local CLI can, and a custom hosted DomainScope can enable the Sia
// vault. Nothing here encodes "hosted servers may not have OOB" — a view
// installs on any deployment whose capabilities cover its requirements and
// whose domain surface registers the view's underlying tools.
type Capability uint32

const (
	// CapOOB is the out-of-band browser-handoff coordinators (SSO sign-in /
	// account credential one-time pages). CLI assemblies wire them from the
	// local auth stack; a hosted assembly may wire its own browser-form
	// coordinators when its identity adoption supports those flows.
	CapOOB Capability = 1 << iota
	// CapVault is the local Sia vault surface (the handoff-driven vault
	// create/restore/browse/upload views drive vault-profile operations on
	// the machine's local vault state).
	CapVault
	// CapHosted is the hosted (Portal-embedded) deployment kind. It only
	// affects identity/attribution decisions (deployment origin), not
	// installability.
	CapHosted
)

// AppLauncherToolPrefix is the open_* launcher tool-name prefix; a view's
// screen name (the InstalledAppView Screen equivalent) is the launcher name
// minus this prefix.
const AppLauncherToolPrefix = "open_"

// The launcher tool-name vocabulary. Composition roots key their installers
// and gating on these constants instead of re-typing the strings, so a table
// rename cannot silently pass an unavailable launcher to Selectable.
const (
	LauncherPinCreator       = "open_pin_creator"
	LauncherSSOSignin        = "open_sso_signin"
	LauncherAccountPassword  = "open_account_password"
	LauncherAccountEmail     = "open_account_email"
	LauncherAccount          = "open_account"
	LauncherVaultCreate      = "open_vault_create"
	LauncherVaultRestore     = "open_vault_restore"
	LauncherVaultBrowser     = "open_vault_browser"
	LauncherPinList          = "open_pin_list"
	LauncherUploadManager    = "open_upload_manager"
	LauncherVaultManager     = "open_vault_manager"
	LauncherDownloadManager  = "open_download_manager"
	LauncherVaultDownloadMgr = "open_vault_download_manager"
)

// ViewSpec is one row of the shared app-view table: the wire identity of the
// ui:// view, its model-facing open_* launcher, and which deployments may
// carry it. Composition roots own everything this table deliberately does
// not: the view's HTML (injected renderer), the app-only helper tools, and
// the service dependencies behind custom installers.
type ViewSpec struct {
	// Launcher is the model-facing launcher tool name ("open_pin_creator").
	// It is also the ONLY tool that advertises the view's ui.resourceUri;
	// the operational primitives the view drives stay headless.
	Launcher string

	// Title is the launcher tool title (e.g. "Create a Pin").
	Title string

	// AppPhrase and HumanPurpose feed the shared launcher description
	// skeleton: AppPhrase follows "Open the interactive " and HumanPurpose
	// completes "it renders an iframe for a human to ...".
	AppPhrase    string
	HumanPurpose string

	// Headless is the full clause after "the headless equivalent is " in the
	// launcher description.
	Headless string

	// Category is the launcher's discovery category.
	Category model.ToolCategory

	// URI is the ui:// resource the launcher renders.
	URI string

	// ResourceName is the stable ui:// resource slug (resources/list).
	ResourceName string

	// ResourceTitle and ResourceDescription are the resource metadata
	// surfaced in resources/list and the widget description layers.
	ResourceTitle       string
	ResourceDescription string

	// View is the canvas screen the document renders.
	View canvas.View

	// PrefersBorder hints hosts to render the iframe with a border.
	PrefersBorder bool
	// Requires is the capability set the view needs beyond its
	// dependency-bound installer: a deployment's capability mask must cover
	// Requires for the view to be selectable. Missing capabilities express
	// "not wired on this deployment", never "not allowed": a future hosted
	// OOB wiring flips CapOOB on and the sign-in card becomes installable
	// there with no table change.
	Requires Capability

	// CustomDescriptor marks views whose launcher descriptor is
	// dependency-bound rather than the generic NewLauncherDescriptorFor
	// skeleton (currently the two upload managers: they carry input schemas and
	// live coordinators). The table still declares their launch identity so
	// inventory and gating stay centralized; a deployment builds those
	// descriptors via the shared seam (UploadManagerDescriptor /
	// UploadManagerInstaller) instead of from scratch.
	CustomDescriptor bool
}

// Screen returns the bare app screen name the InstalledAppView-style records
// and guide inventory use: the launcher name minus the open_ prefix.
func (v ViewSpec) Screen() string {
	return v.Launcher[len(AppLauncherToolPrefix):]
}

// all is the single source of truth for Pinner's app views. Registration
// order is load-direction order (launcher revision: pin, auth, account,
// vault, transfer) and MUST remain stable — the launcher list order shows up
// in open_app's enumerated description and guide copy.
var all = []ViewSpec{
	{
		Launcher: "open_pin_creator", Title: "Create a Pin",
		AppPhrase: "Create a Pin app", HumanPurpose: "enter a CID and pin it",
		Headless: "pins_add for autonomous pin creation without a rendered form",
		Category: model.CategoryCore,
		URI:      "ui://pins/create.html", ResourceName: "create-pin",
		ResourceTitle:       "Create a Pin",
		ResourceDescription: "Create a pin for an existing CID via the Pinner.xyz API.",
		View:                canvas.ViewPin,
		PrefersBorder:       true,
		Requires:            0,
	},
	{
		Launcher: "open_sso_signin", Title: "Sign In (App)",
		AppPhrase: "Sign In app", HumanPurpose: "complete SSO approval",
		Headless: "auth_sso, which returns the approval URL + resume handle without rendering a card",
		Category: model.CategoryAccount,
		URI:      "ui://auth/sso.html", ResourceName: "auth-sso",
		ResourceTitle:       "Sign In",
		ResourceDescription: "Complete an out-of-band sign-in (SSO approval).",
		View:                canvas.ViewAuthSSO,
		PrefersBorder:       true,
		Requires:            CapOOB,
	},
	{
		Launcher: "open_account_password", Title: "Change Password (App)",
		AppPhrase: "Change Password app", HumanPurpose: "change their password",
		Headless: "account_password_update",
		Category: model.CategoryAccount,
		URI:      "ui://account/password.html", ResourceName: "account-password",
		ResourceTitle:       "Change Password",
		ResourceDescription: "Change your Pinner password via a one-time page opened in your browser.",
		View:                canvas.ViewAccountPassword,
		PrefersBorder:       true,
		Requires:            CapOOB,
	},
	{
		Launcher: "open_account_email", Title: "Change Email (App)",
		AppPhrase: "Change Email app", HumanPurpose: "change their email",
		Headless: "account_email_change",
		Category: model.CategoryAccount,
		URI:      "ui://account/email.html", ResourceName: "account-email",
		ResourceTitle:       "Change Email",
		ResourceDescription: "Change your Pinner email via a one-time page opened in your browser.",
		View:                canvas.ViewAccountEmail,
		PrefersBorder:       true,
		Requires:            CapOOB,
	},
	{
		Launcher: "open_account", Title: "Account (App)",
		AppPhrase: "Account app", HumanPurpose: "view authentication status",
		Headless: "auth_status for autonomous access",
		Category: model.CategoryAccount,
		URI:      "ui://auth/status.html", ResourceName: "auth-status",
		ResourceTitle:       "Account",
		ResourceDescription: "Read-only authentication/account status strip.",
		View:                canvas.ViewAuthStatus,
		PrefersBorder:       true,
		Requires:            0,
	},
	{
		Launcher: "open_vault_create", Title: "Create Vault (App)",
		AppPhrase: "Create Vault app", HumanPurpose: "create a vault (Sia approval + recovery seed)",
		Headless: "vault_create, which returns the create URL + resume handle without rendering a card",
		Category: model.CategoryStorage,
		URI:      "ui://vault/create.html", ResourceName: "vault-create",
		ResourceTitle:       "Create Vault",
		ResourceDescription: "Create a vault: approve the Sia device and save the recovery seed.",
		View:                canvas.ViewVaultCreate,
		PrefersBorder:       true,
		Requires:            CapVault,
	},
	{
		Launcher: "open_vault_restore", Title: "Restore Vault (App)",
		AppPhrase: "Restore Vault app", HumanPurpose: "restore a vault from its recovery seed",
		Headless: "vault_restore, which returns the restore URL + resume handle without rendering a card",
		Category: model.CategoryStorage,
		URI:      "ui://vault/restore.html", ResourceName: "vault-restore",
		ResourceTitle:       "Restore Vault",
		ResourceDescription: "Restore a vault from its recovery seed.",
		View:                canvas.ViewVaultRestore,
		PrefersBorder:       true,
		Requires:            CapVault,
	},
	{
		Launcher: "open_vault_browser", Title: "Vault Browser (App)",
		AppPhrase: "Vault browser app", HumanPurpose: "browse the vault",
		Headless: "vault_status / vault_ls for autonomous access",
		Category: model.CategoryStorage,
		URI:      "ui://vault/browser.html", ResourceName: "vault-browser",
		ResourceTitle:       "Vault browser",
		ResourceDescription: "Read-only vault status and file browser.",
		View:                canvas.ViewVaultBrowser,
		PrefersBorder:       true,
		Requires:            CapVault,
	},
	{
		Launcher: "open_pin_list", Title: "Pin List (App)",
		AppPhrase: "Pin list app", HumanPurpose: "browse pins",
		Headless: "pins_list for autonomous access",
		Category: model.CategoryCore,
		URI:      "ui://pins/list.html", ResourceName: "pin-list",
		ResourceTitle:       "Pins",
		ResourceDescription: "Read-only list of your pins and their status.",
		View:                canvas.ViewPinList,
		PrefersBorder:       true,
		Requires:            0,
	},
	{
		Launcher: "open_upload_manager", Title: "Open Upload to IPFS App",
		AppPhrase: "Upload to IPFS app", HumanPurpose: "pick a file and upload it to Pinner",
		Headless: "upload_file for autonomous uploads without a rendered form",
		Category: model.CategoryCore,
		URI:      "ui://uploads/ipfs.html", ResourceName: "ipfs-upload",
		ResourceTitle:       "Upload to IPFS",
		ResourceDescription: "Pick a file and upload it to Pinner over IPFS.",
		View:                canvas.ViewIPFSUpload,
		PrefersBorder:       true,
		// Presigned-PUT input schema and a live coordinator: dependency-bound
		// via the shared UploadManagerDescriptor / UploadManagerInstaller seam.
		CustomDescriptor: true,
		Requires:         0,
	},
	{
		Launcher: "open_vault_manager", Title: "Open Upload to Vault App",
		AppPhrase: "Upload to Vault app", HumanPurpose: "pick a file and store it in the vault",
		Headless: "vault_put_file for autonomous vault uploads without a rendered form",
		Category: model.CategoryStorage,
		URI:      "ui://uploads/vault.html", ResourceName: "vault-upload",
		ResourceTitle:       "Upload to Vault",
		ResourceDescription: "Pick a file and store it in your encrypted Pinner vault.",
		View:                canvas.ViewVaultUpload,
		PrefersBorder:       true,
		CustomDescriptor:    true,
		Requires:            CapVault,
	},
	{
		Launcher: "open_download_manager", Title: "Download from IPFS",
		AppPhrase: "Download from IPFS app", HumanPurpose: "initiate a download",
		Headless: "download_file for autonomous downloads without a rendered form",
		Category: model.CategoryCore,
		URI:      "ui://downloads/ipfs.html", ResourceName: "ipfs-download",
		ResourceTitle:       "Download from IPFS",
		ResourceDescription: "Download IPFS content (CID or CID/path) to a file.",
		View:                canvas.ViewIPFSDownload,
		PrefersBorder:       true,
		Requires:            0,
	},
	{
		Launcher: "open_vault_download_manager", Title: "Download from Vault",
		AppPhrase: "Download from Vault app", HumanPurpose: "initiate a vault download",
		Headless: "vault_get_file for autonomous vault downloads without a rendered form",
		Category: model.CategoryStorage,
		URI:      "ui://downloads/vault.html", ResourceName: "vault-download",
		ResourceTitle:       "Download from Vault",
		ResourceDescription: "Download a file from your encrypted Pinner vault.",
		View:                canvas.ViewVaultDownload,
		PrefersBorder:       true,
		Requires:            CapVault,
	},
}

// All returns the shared app-view table (read-only; callers must not mutate).
func All() []ViewSpec { return all }

// SpecForLauncher returns the table row for a launcher tool name.
func SpecForLauncher(launcher string) (ViewSpec, bool) {
	for _, v := range all {
		if v.Launcher == launcher {
			return v, true
		}
	}
	return ViewSpec{}, false
}

// Selectable returns the table rows installable on a deployment with the
// capability mask caps: each row's Requires must be covered by caps AND
// available must report the row's dependencies wired. available is consulted
// only for capability-covered rows; a nil available takes every covered
// row's dependencies as present.
func Selectable(caps Capability, available func(ViewSpec) bool) []ViewSpec {
	var out []ViewSpec
	for _, v := range all {
		if v.Requires&^caps != 0 {
			continue
		}
		if available != nil && !available(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}
