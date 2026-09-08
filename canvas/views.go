package canvas

import (
	"fmt"
	"regexp"
)

// ContractVersion is the version of the Pinner view contracts defined in
// this package, in the same spirit as mcpcanvas's ManifestSchemaVersion. It
// covers the entire surface here: the view ID set, each view's ui:// address
// suffix, the payload field sets, and the action/state vocabulary.
//
// Bump it (and the contract tests) on any breaking change — renaming or
// removing a view ID, changing a payload field's name or type, or removing
// an action/state value. Additive changes (a new view, payload field,
// action, or state) do not require a bump because older peers ignore what
// they do not know. All registered views share one version; there is no
// per-view versioning.
const ContractVersion = 1

// View identifies one Pinner MCP App screen: a self-contained HTML document
// served as a ui:// resource and rendered by a UI-capable host in an
// iframe.
//
// The canonical name string is the key both composition roots resolve the
// view by: the CLI root uses it as the module bundle name (pinner-cli
// internal/mcpapp apps_embed.go bundleNames), and a hosted root uses it as
// the manifest view key handed to mcpcanvas asset lookup (mcpcanvas
// Bundle.View). Each constant's godoc records its canonical ui:// address
// as registered by pinner-cli internal/mcp/*.
//
// View IDs are a versioned compatibility surface: the name format is stable
// lowercase kebab-case ASCII ([a-z][a-z0-9]*(-[a-z0-9]+)*), names are never
// empty, and clients (host manifests, open_app launchers, published UI
// artifacts) refer to them blindly — so a rename or removal is a breaking
// change and requires a ContractVersion bump.
type View string

const (
	// ViewPin is the "Create a Pin" form view (ui://pins/create.html).
	ViewPin View = "pin"
	// ViewPinList is the "Pins" read-only listing view (ui://pins/list.html).
	ViewPinList View = "pin-list"
	// ViewVaultBrowser is the "Vault" browser view (ui://vault/browser.html).
	ViewVaultBrowser View = "vault-browser"
	// ViewVaultCreate is the "Create Vault" flow view (ui://vault/create.html).
	ViewVaultCreate View = "vault-create"
	// ViewVaultRestore is the "Restore Vault" flow view
	// (ui://vault/restore.html).
	ViewVaultRestore View = "vault-restore"
	// ViewVaultUpload is the "Upload to Vault" form view
	// (ui://uploads/vault.html).
	ViewVaultUpload View = "vault-upload"
	// ViewVaultDownload is the "Download from Vault" form view
	// (ui://downloads/vault.html).
	ViewVaultDownload View = "vault-download"
	// ViewIPFSUpload is the "Upload to IPFS" form view
	// (ui://uploads/ipfs.html).
	ViewIPFSUpload View = "ipfs-upload"
	// ViewIPFSDownload is the "Download from IPFS" form view
	// (ui://downloads/ipfs.html).
	ViewIPFSDownload View = "ipfs-download"
	// ViewAuthSSO is the "Sign In" SSO flow view (ui://auth/sso.html).
	ViewAuthSSO View = "auth-sso"
	// ViewAuthStatus is the signed-in "Account" status view
	// (ui://auth/status.html).
	ViewAuthStatus View = "auth-status"
	// ViewAccountPassword is the "Change Password" link view
	// (ui://account/password.html).
	ViewAccountPassword View = "account-password"
	// ViewAccountEmail is the "Change Email" link view
	// (ui://account/email.html).
	ViewAccountEmail View = "account-email"
)

// viewNameRe matches the stable, documented view ID format: lowercase
// kebab-case ASCII, starting with a letter, never empty.
var viewNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// allViews is the exhaustive view table for ContractVersion 1, in sorted
// order. It mirrors the reference screen set one-to-one (pinner-cli
// internal/mcpapp bundleNames keys and the ui:// resources internal/mcp
// registers); the contract tests pin the exact membership so a screen
// served without a contract, or a contract without a screen, fails.
var allViews = []View{
	ViewAccountEmail,
	ViewAccountPassword,
	ViewAuthSSO,
	ViewAuthStatus,
	ViewIPFSDownload,
	ViewIPFSUpload,
	ViewPin,
	ViewPinList,
	ViewVaultBrowser,
	ViewVaultCreate,
	ViewVaultDownload,
	ViewVaultRestore,
	ViewVaultUpload,
}

// AllViews returns every view registered under ContractVersion, in sorted
// order. Callers must treat the result as read-only; it is a defensive
// copy.
func AllViews() []View {
	out := make([]View, len(allViews))
	copy(out, allViews)
	return out
}

// IsValid reports whether v is a registered view under ContractVersion.
func (v View) IsValid() bool {
	for _, known := range allViews {
		if v == known {
			return true
		}
	}
	return false
}

// Validate checks v against the stable view ID format. It returns an error
// wrapping ErrViewName for a name that is empty or not lowercase
// kebab-case; a registered view can never fail it. Callers use it to
// refuse unvalidated IDs at registration boundaries (open_app arguments,
// hand-written manifests).
func (v View) Validate() error {
	if !viewNameRe.MatchString(string(v)) {
		return fmt.Errorf("%w: %q is not a valid view ID (want lowercase kebab-case, e.g. %q)",
			ErrViewName, string(v), ViewPin)
	}
	return nil
}

// viewURIs maps each contract view to its canonical ui:// resource address,
// as registered by pinner-cli internal/mcp (the const URI next to each
// AppView registration). The address suffix is part of the compatibility
// surface: hosts, agent guides, and published artifacts reference it the
// same way they reference the view ID.
var viewURIs = map[View]string{
	ViewPin:             "ui://pins/create.html",
	ViewPinList:         "ui://pins/list.html",
	ViewVaultBrowser:    "ui://vault/browser.html",
	ViewVaultCreate:     "ui://vault/create.html",
	ViewVaultRestore:    "ui://vault/restore.html",
	ViewVaultUpload:     "ui://uploads/vault.html",
	ViewVaultDownload:   "ui://downloads/vault.html",
	ViewIPFSUpload:      "ui://uploads/ipfs.html",
	ViewIPFSDownload:    "ui://downloads/ipfs.html",
	ViewAuthSSO:         "ui://auth/sso.html",
	ViewAuthStatus:      "ui://auth/status.html",
	ViewAccountPassword: "ui://account/password.html",
	ViewAccountEmail:    "ui://account/email.html",
}

// URI returns the canonical ui:// resource address for v. It returns an
// error wrapping ErrUnknownView for an unregistered view.
func (v View) URI() (string, error) {
	if !v.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownView, string(v))
	}
	return viewURIs[v], nil
}
