package appswire

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	mcpapps "go.lumeweb.com/mcpplane/apps"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
	"go.lumeweb.com/mcpplane/transfer"
)

// newTestUploadCoordinator builds a presigned upload coordinator with an
// immediate-complete async executor and a bounded max bytes.
func newTestUploadCoordinator(t *testing.T) *transfer.Upload {
	t.Helper()
	mgr := transfer.NewUploadTaskManager(func(context.Context, io.Reader, int64, string, bool, string, bool) (any, error) {
		return map[string]any{"cid": "QmTest"}, nil
	}, 0)
	return transfer.NewHTTPUpload(mgr, 0)
}

// TestUploadManagerDescriptorIdentity pins the launcher tool: name, title,
// table-backed resource URI, and model+app visibility meta.
func TestUploadManagerDescriptorIdentity(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	desc, err := UploadManagerDescriptor(hp)
	require.NoError(t, err)
	require.Equal(t, LauncherUploadManager, desc.Name)
	require.Equal(t, "Open Upload to IPFS App", desc.Title)
	v, _ := SpecForLauncher(LauncherUploadManager)
	ui := desc.Meta["ui"].(map[string]any)
	require.Equal(t, v.URI, ui["resourceUri"])
	require.Contains(t, desc.Description, "poll upload_status")
}

// TestUploadManagerDescriptorMints pins the launcher handler: a call with no
// handle mints a fresh operation (presigned url + upload_handle), and the
// handle resolves through the shared coordinator so the app can fulfill it.
func TestUploadManagerDescriptorMints(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	desc, err := UploadManagerDescriptor(hp)
	require.NoError(t, err)
	res, err := desc.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{}})
	require.NoError(t, err)
	sc := res.StructuredContent.(map[string]any)
	require.NotEmpty(t, sc["presigned_url"])
	require.NotEmpty(t, sc["upload_handle"])
	require.Equal(t, false, sc["continued"])
}

// TestUploadManagerDescriptorContinuesHandle pins that supplying a live handle
// continues that exact operation (continued=true) instead of minting a sibling.
func TestUploadManagerDescriptorContinuesHandle(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	desc, err := UploadManagerDescriptor(hp)
	require.NoError(t, err)
	ctx := context.Background()
	url, handle := hp.Prepare(ctx, "x", 0)

	res, err := desc.Handler(ctx, model.ToolRequest{Arguments: map[string]any{"handle": handle}})
	require.NoError(t, err)
	sc := res.StructuredContent.(map[string]any)
	require.Equal(t, true, sc["continued"])
	require.Equal(t, handle, sc["upload_handle"])
	require.Equal(t, url, sc["presigned_url"])
}

// TestUploadManagerDescriptorContinuesDespiteBadTTL pins Finding-1-style
// behavior: continuing a live handle must NOT parse TTL (documented as only
// applying to a fresh mint), so a malformed TTL with a valid handle still
// continues instead of failing with a spurious `invalid ttl` error.
func TestUploadManagerDescriptorContinuesDespiteBadTTL(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	desc, err := UploadManagerDescriptor(hp)
	require.NoError(t, err)
	ctx := context.Background()
	url, handle := hp.Prepare(ctx, "x", 0)

	res, err := desc.Handler(ctx, model.ToolRequest{Arguments: map[string]any{"handle": handle, "ttl": "not-a-duration"}})
	require.NoError(t, err, "a malformed TTL must not block continuing a live handle")
	sc := res.StructuredContent.(map[string]any)
	require.True(t, sc["continued"].(bool))
	require.Equal(t, url, sc["presigned_url"])
}

// TestUploadManagerHelpersNames pins the two app-only helper tools.
func TestUploadManagerHelpersNames(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	helpers, err := UploadManagerHelpers(hp)
	require.NoError(t, err)
	require.Len(t, helpers, 2)
	names := map[string]bool{}
	for _, h := range helpers {
		names[h.Name] = true
	}
	require.True(t, names["ipfs_upload_submit"])
	require.True(t, names["ipfs_upload_status"])
}

// TestUploadManagerHelperSubmitMints pins ipfs_upload_submit prepares a fresh
// operation when given no handle.
func TestUploadManagerHelperSubmitMints(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	helpers, err := UploadManagerHelpers(hp)
	require.NoError(t, err)
	var submit *model.ToolDescriptor
	for i := range helpers {
		if helpers[i].Name == "ipfs_upload_submit" {
			submit = &helpers[i]
		}
	}
	res, err := submit.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{}})
	require.NoError(t, err)
	sc := res.StructuredContent.(map[string]any)
	require.NotEmpty(t, sc["url"])
	require.NotEmpty(t, sc["upload_handle"])
}

// TestUploadManagerHelperStatusRejectsEmpty pins ipfs_upload_status requires a
// handle.
func TestUploadManagerHelperStatusRejectsEmpty(t *testing.T) {
	hp := newTestUploadCoordinator(t)
	helpers, err := UploadManagerHelpers(hp)
	require.NoError(t, err)
	var status *model.ToolDescriptor
	for i := range helpers {
		if helpers[i].Name == "ipfs_upload_status" {
			status = &helpers[i]
		}
	}
	_, err = status.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{}})
	require.ErrorContains(t, err, "handle is required")
}

// TestUploadManagerInstallerRegistersView pins the dependency-bound installer:
// given a real coordinator it registers the view on the registry (returning the
// launcher name), supplies the shared helpers, and applies the coordinator's
// connectDomains to the app's CSP. A nil coordinator is a hard error.
func TestUploadManagerInstallerRegistersView(t *testing.T) {
	registered := map[string]bool{}
	sdk.SetToolRegistrar(func(_ *sdk.Server, desc model.ToolDescriptor, _ model.ToolHandler) error {
		registered[desc.Name] = true
		return nil
	})
	t.Cleanup(func() { sdk.SetToolRegistrar(nil) })

	hp := newTestUploadCoordinator(t)
	catalog := catalogWith(LauncherUploadManager)
	registry := mcpapps.NewAppRegistry()
	rows := Selectable(CapHosted, nil)
	installers := Installers{LauncherUploadManager: UploadManagerInstaller(hp)}
	installed, err := Install(sdk.NewServer(nil), catalog, rows, installers, InstallOptions{
		Registry: registry,
		Render:   renderStub(),
	})
	require.NoError(t, err)
	require.Contains(t, installed, LauncherUploadManager)
	require.True(t, registered["ipfs_upload_submit"], "installer must register the submit helper")
	require.True(t, registered["ipfs_upload_status"], "installer must register the status helper")

	// A nil coordinator is a wiring bug: fail loudly.
	err = UploadManagerInstaller(nil)(InstallContext{})
	require.Error(t, err)
}
