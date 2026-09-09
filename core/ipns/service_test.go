package ipns

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ipfs "go.lumeweb.com/ipfs-sdk"
)

// Regression tests for ResolveKeyID: a key whose NAME is numeric must resolve
// by NAME (to its own key ID) instead of being parsed as a literal ID.
func TestResolveKeyID_NumericNameResolvesByName(t *testing.T) {
	ctx := context.Background()

	sdk := &mockIPNSSDKService{
		listKeysFunc: func(ctx context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
			return []ipfs.IPNSKeyResponse{
				{Id: 42, Name: "my-key"},
				{Id: 77, Name: "123"},
			}, nil
		},
	}

	id, err := ResolveKeyID(ctx, sdk, "123")
	require.NoError(t, err)
	assert.Equal(t, 77, id, "numeric-named key must resolve to its key ID, not the parsed name")
}

func TestResolveKeyID_NumericFallbackWhenNoNameMatches(t *testing.T) {
	ctx := context.Background()

	sdk := &mockIPNSSDKService{
		listKeysFunc: func(ctx context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
			return []ipfs.IPNSKeyResponse{
				{Id: 42, Name: "my-key"},
			}, nil
		},
	}

	// "123" matches no key NAME, so fall back to the parsed numeric ID.
	id, err := ResolveKeyID(ctx, sdk, "123")
	require.NoError(t, err)
	assert.Equal(t, 123, id)
}

func TestResolveKeyID_NonNumericNameNotFound(t *testing.T) {
	ctx := context.Background()

	sdk := &mockIPNSSDKService{
		listKeysFunc: func(ctx context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
			return []ipfs.IPNSKeyResponse{
				{Id: 42, Name: "my-key"},
			}, nil
		},
	}

	_, err := ResolveKeyID(ctx, sdk, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `IPNS key not found for name "does-not-exist"`)
}

func TestResolveKeyID_ListKeysError(t *testing.T) {
	ctx := context.Background()

	sdk := &mockIPNSSDKService{
		listKeysFunc: func(ctx context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
			return nil, errors.New("boom")
		},
	}

	_, err := ResolveKeyID(ctx, sdk, "my-key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to look up IPNS key by name: boom")
}
