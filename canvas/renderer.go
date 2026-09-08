package canvas

import (
	"context"
	"fmt"

	"go.lumeweb.com/mcpcanvas"
)

// Renderer renders complete, self-contained ui:// MCP App view documents for
// the registered Pinner views (AllViews): doctype, <head> with the inline
// theme stylesheet, the view's <body> markup (authored in templ, ported from
// the reference implementation's screens), and the view's ESM module <script>
// resolved through the injected asset source.
//
// The Renderer depends on go.lumeweb.com/mcpcanvas for exactly two things:
// the document shell (mcpcanvas.RenderAppDoc) and the verified-bundle module
// script (mcpcanvas.ModuleJS, which resolves the view through the asset
// source manifest, verifies the sha256 recorded for it, and prefixes the
// version handshake global). Everything Pinner-specific — the view set, the
// bodies, the payload contracts — stays in this package.
//
// Asset delivery is fully injected: this module ships no UI bundles and no
// stylesheet of its own. Composition roots supply an mcpcanvas.AssetSource —
// the CLI wraps its embedded dist bundles (the same files today served by
// internal/mcpapp's bundleNames table, so those asset names are the
// compatibility surface here), and a hosted root wraps its published
// artifact. The theme CSS is likewise injected: the CLI passes its compiled
// Tailwind theme (today inlined as McpAppThemeCSS), a hosted root passes its
// own product stylesheet. An empty stylesheet is permitted (a headless or
// style-less embedding may want it), but a nil AssetSource is a constructor
// error (ErrAssetSource): rendering requires some source of bundle bytes.
type Renderer struct {
	src      mcpcanvas.AssetSource
	themeCSS string
}

// NewRenderer returns a Renderer that serves view documents with bundles
// resolved through src and the stylesheet themeCSS inlined into every
// document's <head>. It returns an error wrapping ErrAssetSource if src is
// nil; themeCSS may be empty (see the Renderer type documentation).
func NewRenderer(src mcpcanvas.AssetSource, themeCSS string) (*Renderer, error) {
	if src == nil {
		return nil, fmt.Errorf("%w: renderer requires an mcpcanvas.AssetSource", ErrAssetSource)
	}
	return &Renderer{src: src, themeCSS: themeCSS}, nil
}

// RenderDoc renders the complete ui:// document for view: title becomes the
// <title>, body renders the <body> markup (pass the templ components in this
// package — they satisfy mcpcanvas.BodyComponent, whose Render method
// signature matches templ.Component exactly), and version is stamped into the
// module script's version handshake global (non-semver values such as
// "develop" are normalized to "1.0.0" by mcpcanvas). Returns the document as
// an inline-safe string, exactly as mcpcanvas.RenderAppDoc does.
//
// An unregistered view wraps ErrUnknownView. Bundle resolution failures
// surface mcpcanvas's sentinels — ErrMissingBundle (view absent from the
// asset manifest), ErrHashMismatch (bundle content fails sha256
// verification), ErrManifestSchema — wrapped with the canvas prefix and the
// view name so a composition root can tell which screen's assets broke.
func (r *Renderer) RenderDoc(ctx context.Context, view View, title string, body mcpcanvas.BodyComponent, version string) (string, error) {
	if !view.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownView, string(view))
	}
	moduleJS, err := mcpcanvas.ModuleJS(ctx, r.src, string(view), version)
	if err != nil {
		return "", fmt.Errorf("canvas: build module script for view %q: %w", string(view), err)
	}
	doc, err := mcpcanvas.RenderAppDoc(title, r.themeCSS, body, moduleJS)
	if err != nil {
		return "", fmt.Errorf("canvas: render document for view %q: %w", string(view), err)
	}
	return doc, nil
}
