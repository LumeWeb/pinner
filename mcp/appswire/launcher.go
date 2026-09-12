package appswire

import (
	"context"
	"encoding/json"
	"fmt"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
	"go.lumeweb.com/mcpplane/toolargs"
	"go.lumeweb.com/pinner/canvas"
)

// OpenLauncherSpec re-declares the model-facing UI launcher identity (the
// former pinner-cli internal shape, hoisted here so both compositions build
// launchers from the one table). A launcher carries _meta.ui.resourceUri so a
// supporting host renders the app's iframe, and it is the ONLY tool that
// advertises the view's resourceUri.
type OpenLauncherSpec struct {
	// Name is the launcher tool name, e.g. "open_upload_manager".
	Name string
	// Title is the tool title.
	Title string
	// Description explains that this is a UI launcher (renders an app).
	Description string
	// Category is the discovery category.
	Category model.ToolCategory
	// ResourceURI is the ui:// view this launcher renders.
	ResourceURI string
	// InputSchema is the tool's argument schema. Most launchers take no
	// arguments (an empty-object schema, the default).
	InputSchema json.RawMessage
}

// OpenLauncherDescriptionBody is the ONE composer for the shared UI-launcher
// description skeleton used by every open_* launcher registration, so the
// boilerplate has a single source and the iframe/launcher wording stays
// identical across all launchers.
//
// app is the phrase following "Open the interactive "; humanPurpose completes
// "it renders an iframe for a human to ..."; body is optional app-specific
// context prose inserted between the "It is not a headless primitive" sentence
// and the headless-equivalent tail (empty for most launchers — use
// OpenLauncherDescription); headlessEquivalent is the full clause after "the
// headless equivalent is ".
func OpenLauncherDescriptionBody(app, humanPurpose, body, headlessEquivalent string) string {
	open := "Open the interactive " + app + ". This is a UI launcher: it renders an iframe for a human to " + humanPurpose + ". "
	if body == "" {
		return open + "It is not a headless primitive; the headless equivalent is " + headlessEquivalent + "."
	}
	return open + "It is not a headless primitive. " + body + " The headless equivalent is " + headlessEquivalent + "."
}

// OpenLauncherDescription is OpenLauncherDescriptionBody without the optional
// mid-description context prose — the default skeleton most launchers use.
func OpenLauncherDescription(app, humanPurpose, headlessEquivalent string) string {
	return OpenLauncherDescriptionBody(app, humanPurpose, "", headlessEquivalent)
}

// OpenLauncherDescriptionFor composes the launcher description from a table
// row, so a call site never re-types the skeleton arguments the table
// already carries.
func (v ViewSpec) OpenLauncherDescriptionFor() string {
	return OpenLauncherDescription(v.AppPhrase, v.HumanPurpose, v.Headless)
}

// NewOpenLauncherDescriptor builds a model-facing launcher tool for the given
// app view. The tool's handler returns a minimal structured result ("the app
// view is open"); the operation the view represents is driven by the iframe
// over callServerTool.
//
// The error return propagates the _meta.ui marshal failure (e.g. an empty
// ResourceURI): a launcher without its marshaled app meta would register as a
// plain tool whose app view silently fails to render, so that is a hard
// assembly error, not one to swallow.
func NewOpenLauncherDescriptor(spec OpenLauncherSpec) (model.ToolDescriptor, error) {
	appMeta, err := sdk.MarshalToolMeta(model.AppToolMeta{
		ResourceURI: spec.ResourceURI,
		Visibility:  []model.ToolVisibility{model.ToolVisibilityModel, model.ToolVisibilityApp},
	})
	if err != nil {
		return model.ToolDescriptor{}, fmt.Errorf("app launcher %q: %w", spec.Name, err)
	}
	if spec.InputSchema == nil {
		spec.InputSchema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	desc := spec.Description
	if desc == "" {
		desc = "Open the " + spec.Title + " app view."
	}
	desc += " open_app (the consolidated launcher) is the unified entry point on hosts where it is visible; this per-app launcher is also discoverable via search_tools."

	return model.ToolDescriptor{
		Name:        spec.Name,
		Title:       spec.Title,
		Description: desc,
		Category:    spec.Category,
		Meta:        appMeta,
		MCPTargets:  []model.ToolTarget{fallbackTarget(desc)},
		InputSchema: spec.InputSchema,
		Handler: func(ctx context.Context, request model.ToolRequest) (model.ToolResult, error) {
			// Launching the app is the action. Return a result carrying the
			// resource URI so a UI-capable host renders the iframe; surface
			// any arguments the model passed so the app/agent can act on them.
			sc := map[string]any{"view": spec.ResourceURI}
			for k, v := range request.Arguments {
				sc[k] = v
			}
			return model.ToolResult{
				StructuredContent: sc,
				Text:              toolargs.ResultJSONText(sc) + " The app view is open.",
			}, nil
		},
	}, nil
}

// NewLauncherDescriptorFor builds the launcher descriptor for a table row, so
// a composition registering standard views never re-types the skeleton
// arguments.
func (v ViewSpec) NewLauncherDescriptorFor() (model.ToolDescriptor, error) {
	return NewOpenLauncherDescriptor(OpenLauncherSpec{
		Name:        v.Launcher,
		Title:       v.Title,
		Description: v.OpenLauncherDescriptionFor(),
		Category:    v.Category,
		ResourceURI: v.URI,
	})
}

// fallbackTarget builds the always-visible model.ToolTarget the launcher
// descriptors resolve through (the trivial core of the CLI's toolforge
// fallback, inlined so this package stays free of CLI imports).
func fallbackTarget(desc string) model.ToolTarget {
	return model.ToolTarget{Visible: true, Description: desc}
}

// RenderFunc renders a canvas view's complete mcp-app HTML document. The
// composition root supplies it (canvas.Renderer needs an embedded asset
// source + theme CSS the shared table cannot own).
type RenderFunc func(view canvas.View) string

// AppViewFor builds the mcpplane AppView for a table row using render for the
// HTML document. helpers ride along as the app-only tools the view calls
// (nil for helper-free views).
func (v ViewSpec) AppViewFor(render RenderFunc, helpers []model.ToolDescriptor) mcpAppView {
	return mcpAppView{
		URI:           v.URI,
		Name:          v.ResourceName,
		Title:         v.ResourceTitle,
		Description:   v.ResourceDescription,
		HTML:          render(v.View),
		PrefersBorder: v.PrefersBorder,
		AttachTo:      []string{v.Launcher},
		Helpers:       helpers,
	}
}
