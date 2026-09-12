package appswire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/mcpplane/sdk"
	"go.lumeweb.com/pinner/canvas"
)

// Install drives installation through its validated options: the registry
// and render an installer runs against are exactly the ones Install checked,
// never closure-captured copies that could diverge (or be nil) from what the
// composition passed.

func firstRow() ViewSpec { return All()[0] }

func renderStub() RenderFunc {
	return func(view canvas.View) string { return "html" }
}

// TestInstallValidatesRequiredOptions pins that missing server/registry/
// render fail the assembly loudly — the options are load-bearing, because
// installers receive them through InstallContext.
func TestInstallValidatesRequiredOptions(t *testing.T) {
	installers := Installers{firstRow().Launcher: func(ic InstallContext) error { return nil }}
	rows := []ViewSpec{firstRow()}
	opts := InstallOptions{Registry: &Registry{}, Render: renderStub()}

	_, err := Install(nil, nil, rows, installers, opts)
	require.ErrorContains(t, err, "nil server")

	srv := &sdk.Server{}
	_, err = Install(srv, nil, rows, installers, InstallOptions{Render: renderStub()})
	require.ErrorContains(t, err, "nil app registry")

	_, err = Install(srv, nil, rows, installers, InstallOptions{Registry: &Registry{}})
	require.ErrorContains(t, err, "nil render func")

	// All present: the validation passes (installer itself is a no-op).
	_, err = Install(srv, nil, rows, installers, opts)
	require.NoError(t, err)
}

// TestInstallContextCarriesValidatedOptions pins the contract the validated
// options exist for: an installer sees the same registry and render Install
// checked, so a composition can never deref a nil closure-captured copy.
func TestInstallContextCarriesValidatedOptions(t *testing.T) {
	registry := &Registry{}
	render := renderStub()
	var seen InstallContext
	v := firstRow()
	installed, err := Install(&sdk.Server{}, nil, []ViewSpec{v}, Installers{
		v.Launcher: func(ic InstallContext) error {
			seen = ic
			return nil
		},
	}, InstallOptions{Registry: registry, Render: render})
	require.NoError(t, err)
	require.Equal(t, []string{v.Launcher}, installed)
	require.Same(t, registry, seen.Registry)
	require.NotNil(t, seen.Render)
	require.NotNil(t, seen.Server)
}

func TestMissingInstallerSkipsRow(t *testing.T) {
	installed, err := Install(&sdk.Server{}, nil, []ViewSpec{firstRow()}, nil,
		InstallOptions{Registry: &Registry{}, Render: renderStub()})
	require.NoError(t, err)
	require.Empty(t, installed,
		"a row without its dependency-bound installer is skipped, never an error")
}
