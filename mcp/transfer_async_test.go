package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/transfer"
)

// newTestUploadManager builds an UploadTaskManager with an immediate-complete
// executor, matching the async upload path.
func newTestUploadManager() *transfer.UploadTaskManager {
	return transfer.NewUploadTaskManager(func(context.Context, io.Reader, int64, string, bool, string, bool) (any, error) {
		return map[string]any{"cid": "QmTest"}, nil
	}, 0)
}

// TestNewAsyncUploadToolsNilManager yields no tools when the manager is nil, so
// a deployment without an async surface never advertises upload_status.
func TestNewAsyncUploadToolsNilManager(t *testing.T) {
	require.Nil(t, NewAsyncUploadTools(nil))
}

// TestNewAsyncUploadToolsNamesAndCategories pins the async tool set.
func TestNewAsyncUploadToolsNamesAndCategories(t *testing.T) {
	descs := NewAsyncUploadTools(newTestUploadManager())
	require.Len(t, descs, 3)
	got := map[string]model.ToolDescriptor{}
	for _, d := range descs {
		got[d.Name] = d
		require.Equal(t, model.CategoryCore, d.Category)
	}
	for _, name := range []string{"upload_status", "upload_cancel", "upload_list"} {
		require.Contains(t, got, name)
	}
}

// TestAsyncUploadStatusLifecycle runs an upload through the manager and asserts
// upload_status reports completion, upload_list enumerates it, and an unknown
// handle errors cleanly.
func TestAsyncUploadStatusLifecycle(t *testing.T) {
	mgr := newTestUploadManager()
	descs := NewAsyncUploadTools(mgr)
	var status, list *model.ToolDescriptor
	for i := range descs {
		switch descs[i].Name {
		case "upload_status":
			status = &descs[i]
		case "upload_list":
			list = &descs[i]
		}
	}

	id, err := mgr.Start(context.Background(), io.NopCloser(strings.NewReader("x")), 1, "d.txt", false)
	require.NoError(t, err)

	// upload_list sees the handle.
	lres, err := list.Handler(context.Background(), model.ToolRequest{})
	require.NoError(t, err)
	require.Contains(t, string(mustJSON(t, lres.StructuredContent)), id)

	require.Eventually(t, func() bool {
		res, err := status.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{"handle": id}})
		return err == nil && strings.Contains(string(mustJSON(t, res.StructuredContent)), "completed")
	}, 2*time.Second, 10*time.Millisecond)

	// Unknown handle: clean error.
	_, err = status.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{"handle": "nope"}})
	require.Error(t, err)
}

// TestAsyncUploadCancelRejectsUnknownAndCompletes asserts cancel fails for an
// unknown handle and reports success for a live one.
func TestAsyncUploadCancelLifecycle(t *testing.T) {
	mgr := newTestUploadManager()
	descs := NewAsyncUploadTools(mgr)
	var cancel *model.ToolDescriptor
	for i := range descs {
		if descs[i].Name == "upload_cancel" {
			cancel = &descs[i]
		}
	}

	_, err := cancel.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{"handle": "nope"}})
	require.Error(t, err)

	id, err := mgr.Start(context.Background(), io.NopCloser(strings.NewReader("x")), 1, "d.txt", false)
	require.NoError(t, err)
	res, err := cancel.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{"handle": id}})
	require.NoError(t, err)
	require.Contains(t, string(mustJSON(t, res.StructuredContent)), "cancelled")
}

// TestNewAsyncUploadToolsMissingHandle pins status/cancel reject an empty
// handle rather than querying the manager.
func TestNewAsyncUploadToolsMissingHandle(t *testing.T) {
	descs := NewAsyncUploadTools(newTestUploadManager())
	for _, name := range []string{"upload_status", "upload_cancel"} {
		var desc *model.ToolDescriptor
		for i := range descs {
			if descs[i].Name == name {
				desc = &descs[i]
			}
		}
		_, err := desc.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{}})
		require.ErrorContains(t, err, "handle is required")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
