package canvas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpcanvas"
)

// fakeSource is a minimal mcpcanvas.AssetSource recording which views the
// renderer asks it to open, so tests can pin per-view bundle selection
// without shipping real UI bundles (composition roots own those). It
// reproduces the reference behavior mcpapp_test.go pins against the CLI's
// embedded assets, adapted to the injected-source seam.
type fakeSource struct {
	manifest  mcpcanvas.Manifest
	manifestE error
	opens     []string
}

func (s *fakeSource) Manifest(context.Context) (mcpcanvas.Manifest, error) {
	if s.manifestE != nil {
		return mcpcanvas.Manifest{}, s.manifestE
	}
	return s.manifest, nil
}

func (s *fakeSource) Open(_ context.Context, view string) (io.ReadSeeker, error) {
	if _, err := s.manifest.Lookup(view); err != nil {
		return nil, err
	}
	s.opens = append(s.opens, view)
	return strings.NewReader(bundleContent(view)), nil
}

// bundleContent returns deterministic fake bundle bytes for view, carrying a
// marker (like the reference pin bundle's "pins_add") so inlining into the
// document can be asserted byte-for-byte.
func bundleContent(view string) string {
	return "/* bundle:" + view + " pins_add */"
}

func manifestHash(view string) string {
	sum := sha256.Sum256([]byte(bundleContent(view)))
	return hex.EncodeToString(sum[:])
}

// newTestRenderer builds a Renderer over a fakeSource serving every
// registered view with a valid self-hash and a distinctive injected theme
// CSS. Serving the full view set keeps the batch tables (Phase 2a screens and
// the remaining screens in screens_test.go) independent of manifest holes.
func newTestRenderer(t *testing.T) (*Renderer, *fakeSource) {
	t.Helper()
	src := &fakeSource{manifest: mcpcanvas.Manifest{
		SchemaVersion: mcpcanvas.ManifestSchemaVersion,
		Compatibility: "test",
		Bundles:       []mcpcanvas.Bundle{},
	}}
	for _, view := range AllViews() {
		src.manifest.Bundles = append(src.manifest.Bundles, mcpcanvas.Bundle{
			View: string(view),
			File: "dist/" + string(view) + ".js",
			Hash: manifestHash(string(view)),
		})
	}
	r, err := NewRenderer(src, ".injected-theme{color:red}")
	require.NoError(t, err)
	return r, src
}

// TestRenderDocPinsDocumentStructure characterizes the full document shell
// against the seam, mirroring pinner-cli mcpapp_test.go TestRenderMcpAppDoc
// and TestAppModuleJSRendersIntoDoc: exact shell fragments, injected theme,
// module script, body markers the pin bundle queries, and the bundle content.
func TestRenderDocPinsDocumentStructure(t *testing.T) {
	r, src := newTestRenderer(t)
	doc, err := r.RenderDoc(context.Background(), ViewPin, "Create a Pin", PinCreateAppForm(), "v1.2.3")
	require.NoError(t, err)

	// Shell structure (reference TestRenderMcpAppDoc assertions).
	for _, want := range []string{
		"<!doctype html>",
		"<title>Create a Pin</title>",
		"<script type=\"module\">",
		"/* bundle:pin pins_add */", // the fake bundle, inlined like AppModuleJS("pin")
		"</body></html>",
	} {
		assert.Contains(t, doc, want)
	}

	// The injected theme CSS is inlined into <style>, and the body keeps its
	// markup markers (reference TestMcpAppThemeCSSEmbedded assertions, plus
	// the id/class hooks the pin bundle queries).
	for _, want := range []string{
		"<style>",
		".injected-theme{color:red}",
		"app-shell",
		"text-accent",
		"id=\"pin-form\"",
		"id=\"cid\"",
		"name=\"cid\"",
		"id=\"name\"",
		"id=\"pin-status\"",
		"id=\"out-cid\"",
		"id=\"out-status\"",
	} {
		assert.Contains(t, doc, want, "rendered doc missing %q", want)
	}

	// Bundle selection: the renderer must have opened exactly the pin view
	// through the AssetSource.
	require.Len(t, src.opens, 1)
	assert.Equal(t, "pin", src.opens[0])
}

// TestRenderDocVersionGlobal adapts reference TestAppModuleInjectsVersionGlobal:
// mcpcanvas.ModuleJS prefixes the verified bundle with the version handshake
// global carrying the injected version, quoted and semver-normalized, so apps
// advertise the host binary's version during ui/initialize.
func TestRenderDocVersionGlobal(t *testing.T) {
	r, _ := newTestRenderer(t)

	doc, err := r.RenderDoc(context.Background(), ViewPin, "T", PinCreateAppForm(), "v0.2.1")
	require.NoError(t, err)
	assert.Contains(t, doc, `window.__MCPCANVAS_VERSION__ = "0.2.1";`)

	// Non-semver values fall back to a valid semver so the host never rejects
	// the handshake (reference TestSemverNormalize {"develop", "1.0.0"}).
	doc, err = r.RenderDoc(context.Background(), ViewPin, "T", PinCreateAppForm(), "develop")
	require.NoError(t, err)
	assert.Contains(t, doc, `window.__MCPCANVAS_VERSION__ = "1.0.0";`)
}

// TestRenderDocBatchViewSelection pins that every screen in the first batch
// resolves through the asset source with its own view name, and that each
// document carries the body ids its JS bundle queries. Bundle names stay the
// reference bundleNames keys (pin, vault-create, vault-browser,
// vault-restore) — the view name is what src.Open receives and what maps to
// appsassets/dist/<view>.js in the CLI root.
func TestRenderDocBatchViewSelection(t *testing.T) {
	cases := []struct {
		view      View
		title     string
		bodyFunc  func() mcpcanvas.BodyComponent
		idMarkers []string
	}{
		{
			view:      ViewPin,
			title:     "Create a Pin",
			bodyFunc:  func() mcpcanvas.BodyComponent { return PinCreateAppForm() },
			idMarkers: []string{"pin-form", "cid", "name", "pin-status", "out-cid", "out-status"},
		},
		{
			view:      ViewVaultCreate,
			title:     "Create Vault",
			bodyFunc:  func() mcpcanvas.BodyComponent { return VaultCreateAppForm() },
			idMarkers: []string{"vault-create-start", "vault-create-url", "vault-create-status"},
		},
		{
			view:      ViewVaultBrowser,
			title:     "Vault browser",
			bodyFunc:  func() mcpcanvas.BodyComponent { return VaultBrowserAppForm() },
			idMarkers: []string{"vault-status", "vault-path", "vault-up", "vault-root", "vault-refresh", "vault-list", "vault-empty"},
		},
		{
			view:      ViewVaultRestore,
			title:     "Restore Vault",
			bodyFunc:  func() mcpcanvas.BodyComponent { return VaultRestoreAppForm() },
			idMarkers: []string{"vault-restore-start", "vault-restore-url", "vault-restore-status"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.view), func(t *testing.T) {
			r, src := newTestRenderer(t)
			doc, err := r.RenderDoc(context.Background(), tc.view, tc.title, tc.bodyFunc(), "1.0.0")
			require.NoError(t, err)

			assert.Contains(t, doc, "<title>"+tc.title+"</title>")
			assert.Contains(t, doc, "/* bundle:"+string(tc.view)+" ")
			for _, id := range tc.idMarkers {
				assert.Contains(t, doc, `id="`+id+`"`, "body missing anchor %q", id)
			}

			require.Len(t, src.opens, 1)
			assert.Equal(t, string(tc.view), src.opens[0],
				"asset source must be asked for the view's own bundle (appsassets/dist/%s.js)", src.opens[0])
		})
	}
}

// TestRenderDocUnknownView pins that an unregistered view is refused with the
// contract sentinel before any asset lookup happens.
func TestRenderDocUnknownView(t *testing.T) {
	r, src := newTestRenderer(t)

	//nolint:goconst // one-off drift simulation
	_, err := r.RenderDoc(context.Background(), View("nope"), "T", PinCreateAppForm(), "1.0.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownView)
	assert.Empty(t, src.opens, "no asset lookups may happen for an unknown view")
}

// TestNewRendererRequiresAssetSource pins the constructor guard: a nil
// AssetSource is rejected up front with the canvas sentinel.
func TestNewRendererRequiresAssetSource(t *testing.T) {
	_, err := NewRenderer(nil, ".theme{}")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAssetSource)
}

// TestRenderDocMissingBundle pins error propagation when the asset manifest
// has no bundle for the view (mcpcanvas.ErrMissingBundle), including the
// canvas prefix and view name.
func TestRenderDocMissingBundle(t *testing.T) {
	// Serve a manifest without pin-list so its lookup fails.
	r, err := NewRenderer(&fakeSource{manifest: mcpcanvas.Manifest{
		SchemaVersion: mcpcanvas.ManifestSchemaVersion,
		Compatibility: "test",
		Bundles: []mcpcanvas.Bundle{{
			View: string(ViewPin),
			File: "dist/pin.js",
			Hash: manifestHash(string(ViewPin)),
		}},
	}}, ".theme{}")
	require.NoError(t, err)

	_, err = r.RenderDoc(context.Background(), ViewPinList, "Pins", PinListStub(), "1.0.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, mcpcanvas.ErrMissingBundle)
	assert.Contains(t, err.Error(), "pin-list")
}

// TestRenderDocHashMismatch pins sha256 verification propagation: content not
// matching the manifest hash fails with mcpcanvas.ErrHashMismatch, wrapped
// with the view name.
func TestRenderDocHashMismatch(t *testing.T) {
	src := &fakeSource{manifest: mcpcanvas.Manifest{
		SchemaVersion: mcpcanvas.ManifestSchemaVersion,
		Compatibility: "test",
		Bundles: []mcpcanvas.Bundle{{
			View: string(ViewVaultCreate),
			File: "dist/vault-create.js",
			Hash: strings.Repeat("0", 64),
		}},
	}}
	r, err := NewRenderer(src, ".theme{}")
	require.NoError(t, err)

	_, err = r.RenderDoc(context.Background(), ViewVaultCreate, "Create Vault", VaultCreateAppForm(), "1.0.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, mcpcanvas.ErrHashMismatch)
	assert.Contains(t, err.Error(), "vault-create")
}

// TestRenderDocManifestError pins that a failing source manifest surfaces the
// source's own error wrapped with canvas and view context, so composition
// roots can diagnose a broken artifact store.
func TestRenderDocManifestError(t *testing.T) {
	src := &fakeSource{manifestE: errors.New("artifact store offline")}
	r, err := NewRenderer(src, ".theme{}")
	require.NoError(t, err)

	_, err = r.RenderDoc(context.Background(), ViewVaultBrowser, "Vault browser", VaultBrowserAppForm(), "1.0.0")
	require.Error(t, err)
	assert.ErrorContains(t, err, "artifact store offline")
	assert.Contains(t, err.Error(), "vault-browser")
}

// PinListStub is a tiny arbitrary BodyComponent used where a test needs a
// body component different from the tested view's real one; the renderer must
// not care that the body does not match the view.
func PinListStub() mcpcanvas.BodyComponent {
	return templStub{}
}

type templStub struct{}

func (templStub) Render(_ context.Context, _ io.Writer) error { return nil }
