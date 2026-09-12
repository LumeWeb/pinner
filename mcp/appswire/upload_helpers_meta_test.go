package appswire

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUploadManagerHelpersToolInvocationMeta pins the OpenAI toolInvocaton
// labels each Upload to IPFS helper carries alongside its ui:// app meta: a
// present-tense invoking label and a completed-tense invoked label, and the
// app ui meta preserved.
func TestUploadManagerHelpersToolInvocationMeta(t *testing.T) {
	descs := UploadManagerHelpers(newTestUploadCoordinator(t))
	require.Len(t, descs, 2)
	require.True(t, descs[0].Name == "ipfs_upload_submit" || descs[0].Name == "ipfs_upload_status")

	for _, desc := range descs {
		// The app meta must survive the merge: the view stays attached.
		ui, ok := desc.Meta["ui"].(map[string]any)
		require.True(t, ok, "%s: ui meta lost", desc.Name)
		v, _ := SpecForLauncher(LauncherUploadManager)
		require.Equal(t, v.URI, ui["resourceUri"])

		// The reference contract reads each label at its own flat
		// slash-delimited key — ChatGPT does not read a nested object.
		require.NotEmpty(t, desc.Meta["openai/toolInvocation/invoking"],
			"%s: flat invoking label required", desc.Name)
		require.NotEmpty(t, desc.Meta["openai/toolInvocation/invoked"],
			"%s: flat invoked label required", desc.Name)
	}
}
