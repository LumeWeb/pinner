package appswire

import (
	"fmt"

	mcpapps "go.lumeweb.com/mcpplane/apps"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
)

// mcpAppView aliases the module's AppView so the spec shape (launcher.go) can
// construct views without spelling the SDK type.
type mcpAppView = mcpapps.AppView

// Registry is the app registry view installation writes into. The composition
// root owns the instance (the CLI's process-global adapter or a hosted
// deployment's own registry) and, with it, the view-domain resolver that
// attributes ui:// views to the deployment's origin.
type Registry = mcpapps.AppRegistry

// AppCatalog is the module's tool-catalog lookup view installation attaches
// views through.
type AppCatalog = mcpapps.AppCatalog

// InstallContext carries the validated installation facts an installer
// receives: the official server and catalog to attach tools through, the app
// registry the view + tool→view association is written into, and the render
// func for the view's HTML document. Installers read ALL of these from the
// context — never from closure-captured copies — so the install options
// validated by Install are exactly the ones the installation runs against.
type InstallContext struct {
	// Server is the official SDK server tools and resources register on.
	Server *sdk.Server
	// Catalog is the tool-catalog lookup view installation attaches through.
	Catalog AppCatalog
	// Registry receives the view + tool→view association state.
	Registry *Registry
	// Render builds an app view's HTML document from its canvas screen.
	Render RenderFunc
	// Helpers are the app-only tool descriptors each view needs at install
	// time (e.g. the pin-create view's polling helper), keyed by launcher
	// name; a nil or missing key means the view is helper-free.
	Helpers map[string][]model.ToolDescriptor
}

// Installer installs ONE table row's app view onto the context Install
// passes — the seam for views that carry app-only helpers or live
// coordinators: the composition root supplies its per-view installation
// (typically a thin wrapper that binds the view's dependencies to
// ViewInstaller), returning an error only for genuine wiring bugs. A nil map
// entry or a nil Installer for a selected row means the view's dependencies
// are not wired on this deployment — the row is skipped, never an error.
type Installer func(ic InstallContext) error

// Installers maps launcher tool names to their installers.
type Installers map[string]Installer

// InstallOptions carries the composition-owned pieces view installation needs.
// Everything here flows to installers only through InstallContext: an option
// validated by Install is exactly what the installation sees.
type InstallOptions struct {
	// Registry receives the view + tool→view association state. Required.
	Registry *Registry
	// Render builds an app view's HTML document from its canvas screen.
	// Required — screen markup is a composition (asset source + theme) fact.
	Render RenderFunc
	// Helpers supplies the app-only tool descriptors a view needs at install
	// time (e.g. the pin-create view's polling helper); keyed by launcher
	// name, nil for helper-free views. Threaded to installers through
	// InstallContext, like every other option here.
	Helpers map[string][]model.ToolDescriptor
}

// Install registers the given table rows' app views on srv, returning the
// launcher names that actually installed in the given order — the inventory
// the deployment feeds back into the SDK-free surfaces (the agent guide's
// Config.InstalledApps, installed-app records).
//
// Registry and Render arrive at the installer only through the
// InstallContext, so a composition cannot validate one pair here and run the
// installers against a different (or nil) one.
//
// Rows whose installer is missing/nil in installers are skipped: missing
// dependencies must not fail an assembly, and the returned inventory reports
// only what was really registered. Genuinely broken wiring (nil server,
// missing registry/render, an installer returning an error) fails loudly with
// the failing row named.
func Install(srv *sdk.Server, catalog AppCatalog, rows []ViewSpec, installers Installers, opts InstallOptions) ([]string, error) {
	if srv == nil {
		return nil, fmt.Errorf("appswire: nil server")
	}
	if opts.Registry == nil {
		return nil, fmt.Errorf("appswire: nil app registry")
	}
	if opts.Render == nil {
		return nil, fmt.Errorf("appswire: nil render func")
	}
	context := InstallContext{
		Server:   srv,
		Catalog:  catalog,
		Registry: opts.Registry,
		Render:   opts.Render,
		Helpers:  opts.Helpers,
	}
	var installed []string
	for _, v := range rows {
		inst := installers[v.Launcher]
		if inst == nil {
			continue
		}
		if err := inst(context); err != nil {
			return installed, fmt.Errorf("appswire: install %s: %w", v.Launcher, err)
		}
		installed = append(installed, v.Launcher)
	}
	return installed, nil
}

// ViewInstaller returns an Installer bound to one row: on the install
// context it builds the row's AppView from the table (rendered via the
// context's render func, with the context's per-row helpers) and registers it
// on the context's registry — the template for every helper-free,
// coordinator-free view (pin list, auth status, vault browser, download
// managers) and any view whose helpers were supplied through
// InstallOptions.Helpers. Views with live dependencies wrap this installer
// with their dependency binding and control-flow skips.
func (v ViewSpec) ViewInstaller() Installer {
	return func(ic InstallContext) error {
		return ic.Registry.RegisterAppView(ic.Server, ic.Catalog, v.AppViewFor(ic.Render, ic.Helpers[v.Launcher]))
	}
}
