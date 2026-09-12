package appswire

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/mcpplane/model"
)

// TestToolInvocationMeta pins the merged meta shape an app-only helper gets in
// one call: the app ui meta for the view (so the tool stays attached to it)
// plus the two OpenAI toolInvocation labels at their flat slash-delimited keys.
func TestToolInvocationMeta(t *testing.T) {
	v, _ := SpecForLauncher(LauncherUploadManager)
	meta, err := ToolInvocationMeta(v.URI,
		[]model.ToolVisibility{model.ToolVisibilityApp},
		"Preparing upload…", "Upload endpoint prepared")
	require.NoError(t, err)

	ui, ok := meta["ui"].(map[string]any)
	require.True(t, ok, "ui meta missing")
	require.Equal(t, v.URI, ui["resourceUri"])
	require.Equal(t, string(model.ToolVisibilityApp), ui["visibility"].([]any)[0])

	// The reference contract reads each label at its own flat
	// slash-delimited key — ChatGPT does not read a nested object.
	require.Equal(t, "Preparing upload…", meta["openai/toolInvocation/invoking"])
	require.Equal(t, "Upload endpoint prepared", meta["openai/toolInvocation/invoked"])
}

// TestToolInvocationMetaErrs pins the refusal cases: an empty label pair is a
// caller bug (a helper with no invocation copy) and an unknown/empty resource
// URI is the same view-attachment loss NewOpenLauncherDescriptor refuses.
func TestToolInvocationMetaErrs(t *testing.T) {
	v, _ := SpecForLauncher(LauncherUploadManager)

	_, err := ToolInvocationMeta(v.URI, nil, "", "Upload endpoint prepared")
	require.ErrorContains(t, err, "invoking")

	_, err = ToolInvocationMeta(v.URI, nil, "Preparing upload…", "")
	require.ErrorContains(t, err, "invoked")

	_, err = ToolInvocationMeta("", nil, "Preparing upload…", "Upload endpoint prepared")
	require.ErrorContains(t, err, "resourceUri")
}
