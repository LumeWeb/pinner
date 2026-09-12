package canvasassets

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.lumeweb.com/pinner/canvas"
)

// Tests for the SDK-neutral MCP Apps render/asset layer. The app JS behavioral
// logic is tested by the pinner-cli packages/apps vitest suite against the real
// TS source; these tests cover the Go-side seam: the canvas-delegated document
// shell, the embedded theme and bundles, and that every view renders its own
// body markup and exactly its own bundle.

// pinAppViews maps every canvas view to the exact ui:// app title a RenderFunc
// consumer would pass, plus a marker element id unique to that view's body. The
// view slug equals the bundleNames key (canvas view slugs and bundle names are
// the same inventory), so tests can pin per-view bundle selection while
// rendering through the delegation only.
var pinAppViews = []struct {
	view   canvas.View
	title  string
	bodyID string // marker element id unique to this view's body
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
// document: the shell always ends with <script type="module">{ModuleJS}</
// script></body></html>, so the module script is exactly
// doc[start : len(doc)-len(docTail)].
const docScriptOpen = `<script type="module">`
const docTail = `</script></body></html>`

// docVersionGlobalPrefix is the handshake global injected ahead of every
// bundle by mcpcanvas.VersionGlobal.
const docVersionGlobalPrefix = "window.__MCPCANVAS_VERSION__ = "

// extractModuleScript returns the inline module script of a rendered app
// document, failing the test if the shell shape is off.
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

func TestRenderAppDocShell(t *testing.T) {
	for _, tc := range pinAppViews {
		doc := RenderAppDoc(tc.view, tc.title)
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

func TestRenderAppDocViewStructure(t *testing.T) {
	for _, tc := range pinAppViews {
		doc := RenderAppDoc(tc.view, tc.title)
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
		// The module script must be exactly the canvas version global
		// (window.__MCPCANVAS_VERSION__ = "<semver>";) followed byte-for-byte
		// by this view's own embedded bundle — per-view bundle selection.
		module := extractModuleScript(t, tc.view, doc)
		bundle, err := fs.ReadFile(Assets, bundleNames[string(tc.view)])
		if err != nil {
			t.Fatalf("view %q: read embedded bundle %s: %v (run `make assets`)", tc.view, bundleNames[string(tc.view)], err)
		}
		globalEnd := strings.Index(module, `";`)
		if globalEnd < 0 {
			t.Errorf("view %q: module script missing a terminated version global", tc.view)
			continue
		}
		globalEnd += len(`";`)
		if !strings.HasPrefix(module, docVersionGlobalPrefix) {
			t.Errorf("view %q: module script does not start with the version global", tc.view)
		}
		if got := module[globalEnd:]; len(got) != len(bundle) {
			t.Errorf("view %q: module script payload is %d bytes, want %d (bundle %s)", tc.view, len(got), len(bundle), bundleNames[string(tc.view)])
		} else if string(got) != string(bundle) {
			t.Errorf("view %q: module script is not byte-equal to its own bundle %s (wrong bundle selected)", tc.view, bundleNames[string(tc.view)])
		}
	}
}

func TestMcpAppThemeCSSEmbedded(t *testing.T) {
	if strings.TrimSpace(ThemeCSS) == "" {
		t.Fatal("embedded app theme CSS is empty — run `make cssbuild` before building Go")
	}
	for _, tc := range pinAppViews {
		doc := RenderAppDoc(tc.view, tc.title)
		for _, want := range []string{"<style>", "app-shell"} {
			if !strings.Contains(doc, want) {
				t.Errorf("view %q: rendered doc missing %q (theme not inlined?)", tc.view, want)
			}
		}
	}
	// Status palette utilities are referenced by the non-flow views' bodies.
	doc := RenderAppDoc(canvas.ViewPinList, "Pins")
	for _, want := range []string{"text-status-ok", "text-status-error"} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered doc missing %q (theme not inlined?)", want)
		}
	}
}

// fromSpecifierRe matches the module-specifier string in `import ... from
// "spec"`, side-effect `import "spec"`, and dynamic `import("spec")` forms.
// Minified bundles drop the space around the keyword/specifier, so both
// `from "x"` and `from"x"` (and `import"x"` / `import("x")`) must match.
var fromSpecifierRe = regexp.MustCompile(`from\s*["']([^"']+)["']|(?:^|[;)\]}])import\s*["']([^"']+)["']|(?:^|[;)\]}])import\s*\(\s*["']([^"']+)["']`)

// bareModuleSpecifiers returns any module specifiers in an inline-ready bundle
// that the browser cannot resolve on its own: bare package specifiers (e.g.
// "@uppy/core") that do NOT start with ".", "/", or a URL scheme. The sandboxed
// ui:// iframe serves each app as a single inline <script type="module"> with
// no importer and no node_modules, so any such specifier throws
// "Failed to resolve module specifier ..." and kills the app at load time.
func bareModuleSpecifiers(src string) []string {
	seen := map[string]bool{}
	for _, m := range fromSpecifierRe.FindAllStringSubmatch(src, -1) {
		spec := m[1]
		if spec == "" {
			spec = m[2]
		}
		if spec == "" {
			spec = m[3]
		}
		if spec == "" {
			continue
		}
		if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
			continue
		}
		if isResolvableURL(spec) {
			continue
		}
		seen[spec] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func isResolvableURL(spec string) bool {
	for _, p := range []string{"https://", "http://", "//", "data:", "blob:"} {
		if strings.HasPrefix(spec, p) {
			return true
		}
	}
	return false
}

func TestBareModuleSpecifiers(t *testing.T) {
	good := []string{
		`const a = 1;`,
		`import "./local.js";`,
		`import x from "/abs/mod.js";`,
		`import x from "https://cdn.example/lib.js";`,
	}
	bad := []string{
		`import e from "@uppy/core";`,
		`import t from "@uppy/xhr-upload";`,
		`import "zod";`,
		`import { x } from "@modelcontextprotocol/sdk/client.js";`,
		`import("@uppy/core");`,
		`import ("@uppy/xhr-upload");`,
	}
	for _, s := range good {
		if got := bareModuleSpecifiers(s); len(got) != 0 {
			t.Errorf("bareModuleSpecifiers(%q) = %v, want []", s, got)
		}
	}
	for _, s := range bad {
		got := bareModuleSpecifiers(s)
		if len(got) == 0 {
			t.Errorf("bareModuleSpecifiers(%q) = [] , want a flagged specifier", s)
		}
	}
}

func TestAppBundlesEmbedded(t *testing.T) {
	apps := make([]string, 0, len(bundleNames))
	for app := range bundleNames {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	for _, app := range apps {
		src, err := fs.ReadFile(Assets, bundleNames[app])
		if err != nil {
			t.Fatalf("app bundle %q missing from embed FS at %s (run `make assets`): %v", app, bundleNames[app], err)
		}
		if strings.TrimSpace(string(src)) == "" {
			t.Fatalf("app bundle %q is empty", app)
		}
		if bare := bareModuleSpecifiers(string(src)); len(bare) > 0 {
			t.Errorf("app bundle %q is not inline-module-ready (bare imports the browser cannot resolve: %v). "+
				"Run `make assets` — a dependency missing from the bundle stays external.", app, bare)
		}
	}
}

func TestBundleRendersIntoDoc(t *testing.T) {
	doc := RenderAppDoc(canvas.ViewPin, "Create a Pin")
	for _, want := range []string{"<!doctype html>", docScriptOpen, "pins_add"} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered doc missing %q", want)
		}
	}
}

// versionGlobalSemverRe matches the normalized, v-stripped semver core the
// handshake global must advertise (MAJOR.MINOR.PATCH with optional
// -prerelease/+build suffix).
var versionGlobalSemverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func TestRenderAppDocVersionGlobal(t *testing.T) {
	doc := RenderAppDoc(canvas.ViewPin, "Create a Pin")
	idx := strings.Index(doc, docVersionGlobalPrefix)
	if idx < 0 {
		t.Fatalf("rendered doc does not inject %s", docVersionGlobalPrefix)
	}
	rest := doc[idx+len(docVersionGlobalPrefix):]
	end := strings.IndexByte(rest, ';')
	if end <= 0 {
		t.Fatalf("version global assignment unterminated")
	}
	quoted := rest[:end]
	if len(quoted) < 3 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
		t.Fatalf("version not a quoted string literal: %q", quoted)
	}
	val := strings.Trim(quoted, `"`)
	if val == "" {
		t.Fatalf("version global is empty")
	}
	if !versionGlobalSemverRe.MatchString(val) {
		t.Fatalf("version global %q is not normalized semver — the ext-apps host would reject the handshake", val)
	}
	module := extractModuleScript(t, canvas.ViewPin, doc)
	if !strings.HasPrefix(module, docVersionGlobalPrefix) {
		t.Fatalf("version global is not the module script's prefix")
	}
}
