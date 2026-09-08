package canvas

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpcanvas"
)

// TestRenderDocRemainingViewsScrollsStructure characterizes the nine screens
// added after the Phase 2a batch (pin-list, the four transfer forms, auth-sso,
// auth-status, and the two account link screens), mirroring
// pinner-cli mcpapp_test.go TestRenderMcpAppDoc / TestAppModuleJSRendersIntoDoc
// against the canvas seam. For each view it pins: the exact document shell
// (doctype, module script, version handshake global), the injected theme
// stylesheet, the right bundle selected through the AssetSource (the view name
// is the reference bundleNames key, so the CLI root resolves
// appsassets/dist/<view>.js — no view deviates from its slug), and every
// element id the view's JS entry in pinner-cli packages/apps/src/entries
// actually wires (ids are quoted verbatim from those entries' ids tables, so
// renaming an id here breaks the real bundle's DOM queries in tests).
func TestRenderDocRemainingViewsStructure(t *testing.T) {
	cases := []struct {
		view     View
		title    string
		bodyFunc func() mcpcanvas.BodyComponent
		ids      []string
		classes  []string // style hooks the entry JS toggles or the shell relies on
	}{
		{
			view:     ViewPinList,
			title:    "Pins",
			bodyFunc: func() mcpcanvas.BodyComponent { return PinListAppForm() },
			ids:      []string{"pinlist-status", "pinlist-count", "pinlist-refresh", "pinlist-table", "pinlist-empty"},
			classes:  []string{"browser-bar", "browser-path", "browser-empty", "table-head"},
		},
		{
			view:     ViewVaultUpload,
			title:    "Upload to Vault",
			bodyFunc: func() mcpcanvas.BodyComponent { return VaultUploadAppForm() },
			ids:      []string{"vault-upload-form", "vfile", "vfile-name", "vault-path", "vault-upload-status", "vstart", "out-path"},
			classes:  []string{"file-field", "file-picker", "file-btn", "file-name"},
		},
		{
			view:     ViewVaultDownload,
			title:    "Download from Vault",
			bodyFunc: func() mcpcanvas.BodyComponent { return VaultDownloadAppForm() },
			ids:      []string{"vault-download-form", "vault-source", "name", "output", "sink-local", "sink-drop", "vault-download-status", "out-link", "out-path", "start"},
			classes:  []string{"inline"},
		},
		{
			view:     ViewIPFSUpload,
			title:    "Upload to IPFS",
			bodyFunc: func() mcpcanvas.BodyComponent { return IPFSUploadAppForm() },
			ids:      []string{"ipfs-upload-form", "file", "file-name", "name", "ipfs-upload-status", "start", "out-cid", "ipfs-upload-progress", "ipfs-upload-progress-fill", "ipfs-upload-progress-label"},
			classes:  []string{"progress-track", "progress-fill", "progress-label"},
		},
		{
			view:     ViewIPFSDownload,
			title:    "Download from IPFS",
			bodyFunc: func() mcpcanvas.BodyComponent { return IPFSDownloadAppForm() },
			ids:      []string{"ipfs-download-form", "ipfs-source", "name", "output", "sink-local", "sink-drop", "ipfs-download-status", "out-link", "out-path", "start"},
			classes:  []string{"inline"},
		},
		{
			view:     ViewAuthSSO,
			title:    "Sign In",
			bodyFunc: func() mcpcanvas.BodyComponent { return AuthSSOAppForm() },
			ids:      []string{"sso-start", "sso-url", "sso-revoke", "sso-status"},
			classes:  []string{"btn-secondary"},
		},
		{
			view:     ViewAuthStatus,
			title:    "Account",
			bodyFunc: func() mcpcanvas.BodyComponent { return AuthStatusAppForm() },
			ids:      []string{"authstatus-status", "authstatus-outcome", "authstatus-message", "authstatus-refresh"},
		},
		{
			view:     ViewAccountPassword,
			title:    "Change Password",
			bodyFunc: func() mcpcanvas.BodyComponent { return AccountPasswordAppForm() },
			ids:      []string{"pw-start", "pw-url", "pw-status"},
			classes:  []string{"link-ready"},
		},
		{
			view:     ViewAccountEmail,
			title:    "Change Email",
			bodyFunc: func() mcpcanvas.BodyComponent { return AccountEmailAppForm() },
			ids:      []string{"em-start", "em-url", "em-status"},
			classes:  []string{"link-ready"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.view), func(t *testing.T) {
			r, src := newTestRenderer(t)
			doc, err := r.RenderDoc(context.Background(), tc.view, tc.title, tc.bodyFunc(), "v1.0.0")
			require.NoError(t, err)

			// Document shell (reference TestRenderMcpAppDoc assertions, plus
			// the injectable theme and version handshake global).
			for _, want := range []string{
				"<!doctype html>",
				"<title>" + tc.title + "</title>",
				"<style>",
				".injected-theme{color:red}",
				"<script type=\"module\">",
				"/* bundle:" + string(tc.view) + " pins_add */", // fake bundle, inlined like AppModuleJS(view)
				`window.__MCPCANVAS_VERSION__ = "1.0.0";`,
				"</body></html>",
			} {
				assert.Contains(t, doc, want, "rendered doc missing %q", want)
			}

			// Every element id the view's JS entry wires up.
			for _, id := range tc.ids {
				assert.Contains(t, doc, `id="`+id+`"`, "body missing JS anchor %q", id)
			}
			for _, class := range tc.classes {
				assert.Contains(t, doc, `class="`+class, "body missing style hook %q", class)
			}

			// Bundle selection: exactly the view's own name, i.e. the
			// reference bundleNames key → appsassets/dist/<view>.js.
			require.Len(t, src.opens, 1)
			assert.Equal(t, string(tc.view), src.opens[0],
				"asset source must be asked for the view's own bundle (appsassets/dist/%s.js)", src.opens[0])
		})
	}
}

// TestRenderDocRemainingViewsFormFields pins the form-field contracts the
// reference CLI tests leave implicit but the transfer bundles depend on for
// their URLSearchParams/form serialization: the file inputs are type=file,
// both sink radios share name="sink" with the value pair local/drop and
// sink-local checked by default, and each transfer submit button is the form's
// type=submit element. These mirror the ids tables of vault-upload.ts,
// vault-download.ts, ipfs-upload.ts and ipfs-download.ts.
func TestRenderDocRemainingViewsFormFields(t *testing.T) {
	r, _ := newTestRenderer(t)

	up, err := r.RenderDoc(context.Background(), ViewVaultUpload, "U", VaultUploadAppForm(), "1.0.0")
	require.NoError(t, err)
	assert.Contains(t, up, `id="vfile" name="vfile" type="file"`)

	ipfs, err := r.RenderDoc(context.Background(), ViewIPFSUpload, "U", IPFSUploadAppForm(), "1.0.0")
	require.NoError(t, err)
	assert.Contains(t, ipfs, `id="file" name="file" type="file"`)
	assert.Contains(t, ipfs, `role="progressbar"`)
	assert.Contains(t, ipfs, `aria-valuemax="100"`)

	for _, tc := range []struct {
		view View
		body func() mcpcanvas.BodyComponent
	}{
		{ViewVaultDownload, func() mcpcanvas.BodyComponent { return VaultDownloadAppForm() }},
		{ViewIPFSDownload, func() mcpcanvas.BodyComponent { return IPFSDownloadAppForm() }},
	} {
		doc, err := r.RenderDoc(context.Background(), tc.view, "D", tc.body(), "1.0.0")
		require.NoError(t, err)
		assert.Contains(t, doc, `id="sink-local" name="sink" type="radio" value="local" checked="checked"`)
		assert.Contains(t, doc, `id="sink-drop" name="sink" type="radio" value="drop"`)
	}
}
