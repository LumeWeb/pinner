package appswire

import (
	"testing"

	"github.com/stretchr/testify/require"
	mcpapps "go.lumeweb.com/mcpplane/apps"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
	"go.lumeweb.com/pinner/canvas"
)

// registeredHelpers records the helper tool names the install registrar sees;
// reset around the process-global sdk.SetToolRegistrar swap.
var registeredHelpers []string

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

// TestHelpersThreadedThroughViewInstaller pins the helpers contract
// END-TO-END: Install threads InstallOptions.Helpers into the install
// context, and the real ViewSpec.ViewInstaller registers the row's helpers
// from the context (ic.Helpers[v.Launcher]) onto the server. A regression or
// mis-key of the context's helper read breaks this test, not just a
// custom-closure stand-in.
func TestHelpersThreadedThroughViewInstaller(t *testing.T) {
	sdk.SetToolRegistrar(func(srv *sdk.Server, desc model.ToolDescriptor, handler model.ToolHandler) error {
		registeredHelpers = append(registeredHelpers, desc.Name)
		return nil
	})
	defer sdk.SetToolRegistrar(nil)

	// The real registry constructor: a zero AppRegistry has nil internal
	// maps, which RegisterAppView writes into.
	registry := mcpapps.NewAppRegistry()
	v := firstRow()
	helpers := []model.ToolDescriptor{{Name: "pin_status_test_helper"}}
	_, err := Install(sdk.NewServer(nil), catalogWith(v.Launcher), []ViewSpec{v}, Installers{
		v.Launcher: v.ViewInstaller(),
	}, InstallOptions{
		Registry: registry,
		Render:   renderStub(),
		Helpers:  map[string][]model.ToolDescriptor{v.Launcher: helpers},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"pin_status_test_helper"}, registeredHelpers,
		"ViewInstaller must register the context-threaded helpers for the row")
}

// catalogWith returns an AppCatalog stub that knows the given tool names, so
// RegisterAppView's attach-target validation passes.
func catalogWith(names ...string) AppCatalog {
	entries := map[string]*model.ToolEntry{}
	for _, n := range names {
		entries[n] = &model.ToolEntry{Name: n}
	}
	return stubCatalog{entries: entries}
}

type stubCatalog struct {
	entries map[string]*model.ToolEntry
}

func (s stubCatalog) Get(name string) (*model.ToolEntry, bool) {
	e, ok := s.entries[name]
	return e, ok
}

func TestMissingInstallerSkipsRow(t *testing.T) {
	installed, err := Install(&sdk.Server{}, nil, []ViewSpec{firstRow()}, nil,
		InstallOptions{Registry: &Registry{}, Render: renderStub()})
	require.NoError(t, err)
	require.Empty(t, installed,
		"a row without its dependency-bound installer is skipped, never an error")
}
