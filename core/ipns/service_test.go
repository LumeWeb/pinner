package ipns

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ipfs "go.lumeweb.com/ipfs-sdk"
)

// Regression tests for ResolveKeyID: a literal numeric argument is treated as
// a raw key ID directly, with no ListKeys round-trip; non-numeric arguments
// are resolved by matching against key names.
func TestResolveKeyID_NumericArgIsLiteralID(t *testing.T) {
	ctx := context.Background()

	sdk := &mockIPNSSDKService{
		listKeysFunc: func(ctx context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
			return []ipfs.IPNSKeyResponse{
				{Id: 42, Name: "my-key"},
			}, nil
		},
	}

	id, err := ResolveKeyID(ctx, sdk, "123")
	require.NoError(t, err)
	assert.Equal(t, 123, id)
	assert.Zero(t, sdk.listKeysCalls, "numeric arg must resolve as a literal ID without a ListKeys round-trip")
}

func TestResolveKeyID_NonNumericNameResolvesByName(t *testing.T) {
	ctx := context.Background()

	sdk := &mockIPNSSDKService{
		listKeysFunc: func(ctx context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
			return []ipfs.IPNSKeyResponse{
				{Id: 42, Name: "my-key"},
			}, nil
		},
	}

	id, err := ResolveKeyID(ctx, sdk, "my-key")
	require.NoError(t, err)
	assert.Equal(t, 42, id)
}

func TestResolveKeyID_NumericArgTakesPrecedenceOverNumericName(t *testing.T) {
	ctx := context.Background()

	// Intended contract tradeoff: a literal numeric arg always resolves as a
	// raw key ID, even when a key happens to carry that number as its name.
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
	assert.Equal(t, 123, id, "numeric arg resolves as a literal ID, not by matching the numeric key name")
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
