package catalogops

// Boundary-isolation guard for the opmesh-migration core path.
//
// The opmesh overhaul's hard boundary: the opmesh-core path — this package,
// pinnerops, and the stdlib-only catalogmeta frontend-metadata boundary — may
// never depend on a frontend framework (pterm, urfave/cli, MCP SDKs,
// mcpforge, canimcp, pinner-cli). MCP presentation machinery lives in the
// catalogmcp boundary package, which this package intentionally does NOT
// import; mcpforge is expected (and required) in catalogmcp's tree and
// forbidden everywhere else on the core path.

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var bannedFrontendModules = []string{
	"pterm", "urfave", "mcpforge", "canimcp", "pinner-cli",
	"mcp-go", "modelcontextprotocol",
}

// TestCorePathFrontendIsolation verifies that `go list -deps` for catalogops,
// pinnerops, and catalogmeta contains no banned frontend module. catalogops is
// the natural host for this test because it anchors the core operation
// model's end of the boundary.
func TestCorePathFrontendIsolation(t *testing.T) {
	for _, pkg := range []string{
		"go.lumeweb.com/pinner/catalogops",
		"go.lumeweb.com/pinner/pinnerops",
		"go.lumeweb.com/pinner/catalogmeta",
	} {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}{{else}}<stdlib>{{end}}", pkg)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "go list -deps %s: %s", pkg, out)

			sawOpmesh := false
			for _, m := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				m = strings.TrimSpace(m)
				if m == "" || m == "<stdlib>" {
					continue
				}
				if m == "go.lumeweb.com/opmesh" {
					sawOpmesh = true // the target model must be engaged
				}
				for _, banned := range bannedFrontendModules {
					assert.NotContains(t, m, banned,
						"%s must not depend on the banned frontend module fragment %q", pkg, banned)
				}
			}
			if pkg == "go.lumeweb.com/pinner/catalogops" || pkg == "go.lumeweb.com/pinner/pinnerops" {
				assert.True(t, sawOpmesh, "%s must depend on go.lumeweb.com/opmesh", pkg)
			}
		})
	}
}

// TestCatalogMCPExcludesMCPFromCorePath pins the inverse: the frontend-metadata
// boundary packages are the ONLY in-module homes of the frontend machinery —
// catalogmcp must depend on mcpforge, which in turn must be absent from the
// core path (covered above), so the two sides cannot silently swap roles.
func TestCatalogMCPExcludesMCPFromCorePath(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}{{else}}<stdlib>{{end}}", "go.lumeweb.com/pinner/catalogmcp")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go list -deps catalogmcp: %s", out)

	found := false
	for _, m := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(m) == "go.lumeweb.com/mcpforge" {
			found = true
		}
	}
	assert.True(t, found, "catalogmcp (the MCP boundary) must own the mcpforge dependency")
}
