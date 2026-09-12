package appswire

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/sdk"
)

// ToolInvocationMeta builds the _meta for an app-only helper tool in one call:
// it marshals the app meta for the tool's ui:// view (same shape as
// sdk.MarshalToolMeta) and adds the OpenAI toolInvocation labels — invoking is
// the present-tense label shown while the tool runs, invoked the completed
// label shown after it finishes. The resource URI and both labels must be
// non-empty; errors are returned rather than dropped so a helper can never
// silently lose its view attachment.
func ToolInvocationMeta(resourceURI string, visibility []model.ToolVisibility, invoking, invoked string) (mcp.Meta, error) {
	if invoking == "" || invoked == "" {
		return nil, fmt.Errorf("appswire: toolInvocation meta requires non-empty invoking and invoked labels")
	}
	meta, err := sdk.MarshalToolMeta(model.AppToolMeta{
		ResourceURI: resourceURI,
		Visibility:  visibility,
	})
	if err != nil {
		return nil, fmt.Errorf("appswire: toolInvocation meta: %w", err)
	}
	// The reference contract reads each label at its own flat slash-delimited
	// _meta key ("openai/toolInvocation/invoking"), not a nested object.
	// MarshalToolMeta mints a fresh map per call, so mutating it is safe.
	meta["openai/toolInvocation/invoking"] = invoking
	meta["openai/toolInvocation/invoked"] = invoked
	return meta, nil
}
