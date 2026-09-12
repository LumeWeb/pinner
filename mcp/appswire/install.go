package appswire

import (
	"fmt"

	mcpapps "go.lumeweb.com/mcpplane/apps"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
)

// mcpAppView aliases the module's AppView so views.go/launcher.go can carry
// the spec without importing the SDK-typed constructor everywhere.
type mcpAppView = mcpapps.AppView

// Registry is the app registry view installation writes into. The composition
// root owns the instance (the CLI's process-global adapter or a hosted
// deployment's own registry) and, with it, the view-domain resolver that
// attributes ui:// views to the deployment's origin.
type Registry = mcpapps.AppRegistry

// AppCatalog is the module's tool-catalog lookup view installation attaches
// views through.
type AppCatalog = mcpapps.AppCatalog

// Installer installs ONE table row's app view onto srv. It is the seam for
// views that carry app-only helpers or live coordinators: the composition
// root supplies its per-view registration (typically a thin call into
// Install with the dependencies bound), returning an error only for genuine
// wiring bugs. A nil map entry or a nil Installer for a selected row means
// the view's dependencies are not wired on this deployment — the row is
// skipped, never an error.
type Installer func(srv *sdk.Server, catalog AppCatalog) error

// Installers maps launcher tool names to their dependency-bound installers.
type Installers map[string]Installer

// InstallOptions carries the composition-owned pieces view installation needs.
type InstallOptions struct {
	// Registry receives the view + tool→view association state. Required.
	Registry *Registry
	// Render builds an app view's HTML document from its canvas screen.
	// Required — screen markup is a composition (asset source + theme) fact.
	Render RenderFunc
	// Helpers supplies the app-only tool descriptors a view needs at install
	// time (e.g. the pin-create view's polling helper); keyed by launcher
	// name, nil for helper-free views.
	Helpers map[string][]model.ToolDescriptor
}

// Install registers the given table rows' app views on srv through registry,
// returning the launcher names that actually installed in the given order —
// the inventory the deployment feeds back into the SDK-free surfaces (the
// agent guide's Config.InstalledApps, installed-app records).
//
// Rows whose installer is missing/nil in installers are skipped: missing
// dependencies must not fail an assembly, and the returned inventory reports
// only what was really registered. Genuinely broken wiring (nil server,
// missing render, an installer returning an error) fails loudly with the
// failing row named.
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
	var installed []string
	for _, v := range rows {
		inst := installers[v.Launcher]
		if inst == nil {
			continue
		}
		if err := inst(srv, catalog); err != nil {
			return installed, fmt.Errorf("appswire: install %s: %w", v.Launcher, err)
		}
		installed = append(installed, v.Launcher)
	}
	return installed, nil
}

// Installer returns a generic Installer bound to one row: it builds the row's
// AppView from the table (via render, with bound helpers) and registers it on
// the given registry — the template for every helper-free, coordinator-free
// view (pin list, auth status, vault browser, download managers). Views with
// live dependencies wrap this installer with their dep binding.
func (v ViewSpec) Installer(registry *Registry, render RenderFunc, helpers []model.ToolDescriptor) Installer {
	spec := v
	return func(srv *sdk.Server, catalog AppCatalog) error {
		return registry.RegisterAppView(srv, catalog, spec.AppViewFor(render, helpers))
	}
}
