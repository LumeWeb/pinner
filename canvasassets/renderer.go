package canvasassets

import (
	"context"
	"fmt"
	"io/fs"
	"sync"

	"go.lumeweb.com/mcpcanvas"
	"go.lumeweb.com/pinner/canvas"
)

// DefaultVersion is the version stamped into a document's version handshake by
// Render. mcpcanvas normalizes non-semver values to "1.0.0"; a composition
// root that has a real build version should render through the *canvas.Renderer
// (NewRenderer) and pass its own version to RenderDoc.
const DefaultVersion = "develop"

// rendererOnce lazily builds the single shared canvas.Renderer over the
// embedded source + theme. sync.OnceValues guarantees the constructor runs
// exactly once and every caller observes the same result; a broken tree
// (an error here) is a build/release problem and is re-raised on every call.
var rendererOnce = sync.OnceValues(func() (*canvas.Renderer, error) {
	root, err := fs.Sub(Assets, "appsassets")
	if err != nil {
		return nil, fmt.Errorf("canvasassets: sub appsassets from embedded fs: %w", err)
	}
	src, err := mcpcanvas.NewEmbedSource(root)
	if err != nil {
		return nil, fmt.Errorf("canvasassets: build mcpcanvas source from embedded manifest (run `make assets`): %w", err)
	}
	r, err := canvas.NewRenderer(src, ThemeCSS)
	if err != nil {
		return nil, fmt.Errorf("canvasassets: build canvas renderer: %w", err)
	}
	return r, nil
})

// NewRenderer returns a canvas.Renderer over the embedded asset source and the
// compiled theme, ready to render any canvas.View document (manifest-verified,
// version handshake). It is the ready-made constructor for composition roots;
// roots that need a custom asset source/theme use canvas.NewRenderer directly.
func NewRenderer() (*canvas.Renderer, error) {
	return rendererOnce()
}

// bodyFor maps a canvas view to the canvas templ body component for that
// view's screen. The bodies are pure static markup, so the mapping is total
// over every view in canvas.AllViews and needs no per-view arguments.
func bodyFor(view canvas.View) mcpcanvas.BodyComponent {
	switch view {
	case canvas.ViewPin:
		return canvas.PinCreateAppForm()
	case canvas.ViewPinList:
		return canvas.PinListAppForm()
	case canvas.ViewVaultBrowser:
		return canvas.VaultBrowserAppForm()
	case canvas.ViewVaultCreate:
		return canvas.VaultCreateAppForm()
	case canvas.ViewVaultRestore:
		return canvas.VaultRestoreAppForm()
	case canvas.ViewVaultUpload:
		return canvas.VaultUploadAppForm()
	case canvas.ViewVaultDownload:
		return canvas.VaultDownloadAppForm()
	case canvas.ViewIPFSUpload:
		return canvas.IPFSUploadAppForm()
	case canvas.ViewIPFSDownload:
		return canvas.IPFSDownloadAppForm()
	case canvas.ViewAuthSSO:
		return canvas.AuthSSOAppForm()
	case canvas.ViewAuthStatus:
		return canvas.AuthStatusAppForm()
	case canvas.ViewAccountPassword:
		return canvas.AccountPasswordAppForm()
	case canvas.ViewAccountEmail:
		return canvas.AccountEmailAppForm()
	}
	return nil
}

// titleFor is the canonical document <title> for each view, matching the
// titles the reference implementation serves so documents are interchangeable.
func titleFor(view canvas.View) string {
	switch view {
	case canvas.ViewPin:
		return "Create a Pin"
	case canvas.ViewPinList:
		return "Pins"
	case canvas.ViewVaultBrowser:
		return "Vault browser"
	case canvas.ViewVaultCreate:
		return "Create Vault"
	case canvas.ViewVaultRestore:
		return "Restore Vault"
	case canvas.ViewVaultUpload:
		return "Upload to Vault"
	case canvas.ViewVaultDownload:
		return "Download from Vault"
	case canvas.ViewIPFSUpload:
		return "Upload to IPFS"
	case canvas.ViewIPFSDownload:
		return "Download from IPFS"
	case canvas.ViewAuthSSO:
		return "Sign In"
	case canvas.ViewAuthStatus:
		return "Account"
	case canvas.ViewAccountPassword:
		return "Change Password"
	case canvas.ViewAccountEmail:
		return "Change Email"
	}
	return string(view)
}

// Render renders view's complete, self-contained ui:// MCP App document through
// the embedded source + theme, using the canonical title + body for view and
// stamping DefaultVersion into the version handshake. Unknown views error
// wrapping canvas.ErrUnknownView; bundle/manifest problems wrap the mcpcanvas
// sentinels. This is the one-liner for a hosted composition root's
// appswire.RenderFunc.
func Render(view canvas.View) (string, error) {
	return RenderVersioned(view, DefaultVersion)
}

// RenderVersioned is Render with an explicit version for the handshake.
func RenderVersioned(view canvas.View, version string) (string, error) {
	return RenderDoc(view, titleFor(view), version)
}

// RenderDoc renders view's complete document through the embedded source +
// theme with an explicit title and version for the handshake (canonical body).
// It is the seam for roots — such as the reference CLI — that render a view
// with their own title and their own build version while reusing the shared
// embedded assets and theme.
func RenderDoc(view canvas.View, title, version string) (string, error) {
	r, err := NewRenderer()
	if err != nil {
		return "", err
	}
	body := bodyFor(view)
	if body == nil {
		return "", fmt.Errorf("%w: %q", canvas.ErrUnknownView, string(view))
	}
	doc, err := r.RenderDoc(context.Background(), view, title, body, version)
	if err != nil {
		return "", err
	}
	return doc, nil
}
