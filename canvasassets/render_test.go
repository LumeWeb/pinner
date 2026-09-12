package canvasassets

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"go.lumeweb.com/pinner/canvas"
)

// Tests for the SDK-neutral MCP Apps render seam provided by this package
// (NewRenderer / Render / RenderVersioned). The app JS behavioral logic is
// tested against the real TS source in the JS toolchain; these tests cover the
// Go-side seam: the canvas-delegated document shell, the embedded theme and
// bundles, and that every view renders its own body markup and exactly its own
// bundle.

// pinnedViews maps every canvas view to the canonical title Render uses, plus
// a marker element id unique to that view's body. The view slug equals the
// bundleNames key, so tests can pin per-view bundle selection while rendering
// through the public seam only.
var pinnedViews = []struct {
	view   canvas.View
	title  string
	bodyID string
}{
	{canvas.ViewPin, "Create a Pin", "pin-form"},
	{canvas.ViewPinList, "Pins", "pinlist-table"},
	{canvas.ViewVaultBrowser, "Vault browser", "vault-list"},
	{canvas.ViewVaultCreate, "Create Vault", "vault-create-start"},
	{canvas.ViewVaultRestore, "Restore Vault", "vault-restore-start"},
	{canvas.ViewVaultUpload, "Upload to Vault", "vault-upload-form"},
	{canvas.ViewVaultDownload, "Download from Vault", "vault-download-form"},
	{canvas.ViewIPFSUpload, "Upload to IPFS", "ipfs-upload-form"},
	{canvas.ViewIPFSDownload, "Download from IPFS", "ipfs-download-form"},
	{canvas.ViewAuthSSO, "Sign In", "sso-start"},
	{canvas.ViewAuthStatus, "Account", "authstatus-status"},
	{canvas.ViewAccountPassword, "Change Password", "pw-start"},
	{canvas.ViewAccountEmail, "Change Email", "em-start"},
}

// docScriptOpen/docTail delimit the inline module script inside a rendered
// document: the shell ends with <script type="module">{ModuleJS}</script>\
// </body></html>, so the module script is exactly
// doc[start : len(doc)-len(docTail)].
const docScriptOpen = `<script type="module">`
const docTail = `</script></body></html>`

// docVersionGlobalPrefix is the handshake global injected ahead of every
// bundle by mcpcanvas.VersionGlobal.
const docVersionGlobalPrefix = "window.__MCPCANVAS_VERSION__ = "

func extractModuleScript(t *testing.T, view canvas.View, doc string) string {
	t.Helper()
	if !strings.HasSuffix(doc, docTail) {
		t.Fatalf("view %q: rendered doc does not end with %q", view, docTail)
	}
	i := strings.LastIndex(doc, docScriptOpen)
	if i < 0 {
		t.Fatalf("view %q: rendered doc missing %q", view, docScriptOpen)
	}
	return doc[i+len(docScriptOpen) : len(doc)-len(docTail)]
}

// TestRenderShell pins that Render produces the self-contained document shell
// for every view: doctype, title, inline module script, the canvas version
// handshake global, and a closed document.
func TestRenderShell(t *testing.T) {
	for _, tc := range pinnedViews {
		doc, err := Render(tc.view)
		if err != nil {
			t.Fatalf("view %q: Render: %v", tc.view, err)
		}
		for _, want := range []string{
			"<!doctype html>",
			"<title>" + tc.title + "</title>",
			docScriptOpen,
			"window.__MCPCANVAS_VERSION__ = ",
			"</body></html>",
		} {
			if !strings.Contains(doc, want) {
				t.Errorf("view %q: render doc missing %q", tc.view, want)
			}
		}
	}
}

// TestRenderViewStructure structurally pins every ui:// view document: the
// exact title, the shared shell fragments, the inline theme, the view's marker
// body id, and that the document's module script is exactly the canvas version
// global followed byte-for-byte by that view's own embedded bundle
// (per-view bundle selection through manifest verification).
func TestRenderViewStructure(t *testing.T) {
	for _, tc := range pinnedViews {
		doc, err := Render(tc.view)
		if err != nil {
			t.Fatalf("view %q: Render: %v", tc.view, err)
		}
		for _, want := range []string{
			"<!doctype html>",
			"<title>" + tc.title + "</title>",
			"<style>",
			".app-shell",
			`id="` + tc.bodyID + `"`,
			docScriptOpen,
			docVersionGlobalPrefix,
			docTail,
		} {
			if !strings.Contains(doc, want) {
				t.Errorf("view %q: rendered doc missing %q", tc.view, want)
			}
		}
		module := extractModuleScript(t, tc.view, doc)
		bundle, err := fs.ReadFile(Assets, bundleNames[string(tc.view)])
		if err != nil {
			t.Fatalf("view %q: read embedded bundle %s: %v (run the assets target)", tc.view, bundleNames[string(tc.view)], err)
		}
		if !strings.HasPrefix(module, docVersionGlobalPrefix) {
			t.Errorf("view %q: module script does not start with the version global", tc.view)
			continue
		}
		globalEnd := strings.Index(module, `";`)
		if globalEnd < 0 {
			t.Errorf("view %q: module script missing a terminated version global", tc.view)
			continue
		}
		globalEnd += len(`";`)
		got := module[globalEnd:]
		if len(got) != len(bundle) {
			t.Errorf("view %q: module script payload is %d bytes, want %d (bundle %s)", tc.view, len(got), len(bundle), bundleNames[string(tc.view)])
		} else if string(got) != string(bundle) {
			t.Errorf("view %q: module script is not byte-equal to its own bundle %s (wrong bundle selected)", tc.view, bundleNames[string(tc.view)])
		}
	}
}

// TestThemeEmbedded pins that the compiled Tailwind theme is embedded and
// inlined into every app document. A missing/empty tailwind.css (CSS not
// compiled before Go) would leave apps unstyled, so a passing test also proves
// the cssbuild step ran.
func TestThemeEmbedded(t *testing.T) {
	if strings.TrimSpace(ThemeCSS) == "" {
		t.Fatal("embedded app theme CSS is empty — run the cssbuild assets step before building Go")
	}
	for _, tc := range pinnedViews {
		doc, err := Render(tc.view)
		if err != nil {
			t.Fatalf("view %q: Render: %v", tc.view, err)
		}
		for _, want := range []string{"<style>", "app-shell"} {
			if !strings.Contains(doc, want) {
				t.Errorf("view %q: rendered doc missing %q (theme not inlined?)", tc.view, want)
			}
		}
	}
}

// TestRenderVersioned pins the handshake is stamped with the requested version
// (mcpcanvas normalizes non-semver values such as "develop" to "1.0.0").
func TestRenderVersioned(t *testing.T) {
	doc, err := RenderVersioned(canvas.ViewPin, "2.3.4")
	if err != nil {
		t.Fatalf("RenderVersioned: %v", err)
	}
	if !strings.Contains(doc, `"2.3.4"`) {
		t.Errorf("rendered doc missing version-stamped handshake for 2.3.4")
	}
}

// TestRenderDocExplicitTitle pins the RenderDoc convenience renders a view with
// an explicit title + version (the seam the reference CLI uses to keep its own
// titles/build version while reusing the shared embedded assets + theme).
func TestRenderDocExplicitTitle(t *testing.T) {
	doc, err := RenderDoc(canvas.ViewPin, "My Custom Title", "1.2.3")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if !strings.Contains(doc, "<title>My Custom Title</title>") {
		t.Errorf("RenderDoc did not use the explicit title")
	}
	if !strings.Contains(doc, `"1.2.3"`) {
		t.Errorf("RenderDoc did not stamp the explicit version")
	}
}

// TestRenderUnknownView pins that an unregistered view errors (wrapping
// canvas.ErrUnknownView) rather than silently rendering an empty document.
func TestRenderUnknownView(t *testing.T) {
	_, err := Render(canvas.View("does-not-exist"))
	if err == nil {
		t.Fatal("expected error for unknown view, got nil")
	}
	if !errors.Is(err, canvas.ErrUnknownView) {
		t.Errorf("error %q does not wrap canvas.ErrUnknownView", err)
	}
}
