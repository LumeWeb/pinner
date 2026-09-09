package converge

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvergeImportIsolation pins the converge package's hard dependency
// boundary: its non-test sources may depend only on stdlib plus the two
// in-scope modules — go.lumeweb.com/pinner (its own module) and
// go.lumeweb.com/opmesh. No frontend presentation dependency may sneak in
// (directly or transitively through another local subpackage import in the
// non-test files); the banned-module check below enforces that list.
//
// This uses `go list -deps` on the package (test files included in the test
// binary but NOT in `go list -deps` of the package), so the test files' extra
// imports do not count against the shipped package's isolation.
func TestConvergeImportIsolation(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}{{else}}<stdlib>{{end}}", ".")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go list -deps .: %s", out)

	modules := strings.Split(strings.TrimSpace(string(out)), "\n")
	sawOpmesh := false
	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if m == "<stdlib>" {
			continue
		}
		switch m {
		case "go.lumeweb.com/pinner":
			// own module: allowed
		case "go.lumeweb.com/opmesh":
			sawOpmesh = true // the dependency must actually be engaged
		default:
			t.Errorf("converge package depends on unexpected module %q; allowed: stdlib, go.lumeweb.com/pinner, go.lumeweb.com/opmesh", m)
		}
		for _, banned := range []string{"pterm", "urfave", "mcpforge", "canimcp", "pinner-cli", "mcp-go", "modelcontextprotocol"} {
			assert.NotContains(t, m, banned, "banned frontend dependency in converge's dependency tree")
		}
	}
	assert.True(t, sawOpmesh, "go.lumeweb.com/opmesh must be in converge's dependency tree")
}
