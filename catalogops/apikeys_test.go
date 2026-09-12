package catalogops

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/pinner/core/apikeys"
	portalsdk "go.lumeweb.com/portal-sdk"
)

// fakeAPIKeysService mocks apikeys.Service for the api-keys handlers,
// overriding only the methods the tests exercise. It embeds the real
// interface so unimplemented methods panic rather than silently succeed.
type fakeAPIKeysService struct {
	apikeys.Service

	createFn func(ctx context.Context, name string) (*portalsdk.APIKey, error)
}

func (f *fakeAPIKeysService) CreateAPIKey(ctx context.Context, name string) (*portalsdk.APIKey, error) {
	if f.createFn != nil {
		return f.createFn(ctx, name)
	}
	return portalsdk.NewAPIKey(name, "sdk-test-value"), nil
}

// recordingKeyDrop is a fake APIKeyDrop coordinator pinning the one-time OOB
// contract from the coordinator side: it records the deposited key and
// returns a stub drop URL.
type recordingKeyDrop struct {
	gotKey   *portalsdk.APIKey
	returnFn func(key *portalsdk.APIKey) (string, error)
}

func (r *recordingKeyDrop) Drop(_ context.Context, key *portalsdk.APIKey) (string, error) {
	r.gotKey = key
	if r.returnFn != nil {
		return r.returnFn(key)
	}
	return "https://account.example.com/oob/keydrop/test-handle", nil
}

func apiKeysCreateDeps(svc apikeys.Service, drop func(map[string]any) APIKeyDrop) APIKeysDeps {
	return APIKeysDeps{
		Service:         func(map[string]any) apikeys.Service { return svc },
		OOBKeyDropBuild: drop,
	}
}

// TestAPIKeysCreateOOBDropNeverCarriesToken pins the policy contract on the
// model-facing wiring: with an OOB drop coordinator wired, the handler hands
// the freshly created key value to the coordinator and the tool result
// carries ONLY the drop URL — never the token.
func TestAPIKeysCreateOOBDropNeverCarriesToken(t *testing.T) {
	drop := &recordingKeyDrop{}
	op := apiKeysCreate(apiKeysCreateDeps(&fakeAPIKeysService{}, func(map[string]any) APIKeyDrop {
		return drop
	}))

	res, err := op.Handler().Execute(context.Background(), map[string]any{"name": "test-key"})
	require.NoError(t, err)
	result, ok := res.(*APIKeyCreateResult)
	require.True(t, ok, "result type = %T, want *APIKeyCreateResult", res)

	require.Empty(t, result.Token, "key value must never be delivered on a drop-wired (MCP) channel")
	require.Equal(t, "https://account.example.com/oob/keydrop/test-handle", result.DropURL)
	require.Equal(t, "test-key", result.Name)
	require.NotEmpty(t, result.UUID)
	require.NotNil(t, drop.gotKey, "coordinator must receive the created key for one-time human retrieval")
	require.Equal(t, "sdk-test-value", drop.gotKey.Token)
}

// TestAPIKeysCreateCLIDeliversToken pins the human-at-terminal wiring: with
// no OOB drop wired (the CLI surface), the key value is delivered directly in
// the result — the terminal is the human's channel and the one-time display
// contract is preserved.
func TestAPIKeysCreateCLIDeliversToken(t *testing.T) {
	op := apiKeysCreate(apiKeysCreateDeps(&fakeAPIKeysService{}, nil))

	res, err := op.Handler().Execute(context.Background(), map[string]any{"name": "test-key"})
	require.NoError(t, err)
	result, ok := res.(*APIKeyCreateResult)
	require.True(t, ok, "result type = %T, want *APIKeyCreateResult", res)

	require.Equal(t, "sdk-test-value", result.Token)
	require.Empty(t, result.DropURL)
}

// TestAPIKeysCreateDropErrorPropagates pins that a coordinator failure fails
// the invocation rather than falling back to returning the key value.
func TestAPIKeysCreateDropErrorPropagates(t *testing.T) {
	drop := &recordingKeyDrop{returnFn: func(*portalsdk.APIKey) (string, error) {
		return "", errors.New("drop store unavailable")
	}}
	op := apiKeysCreate(apiKeysCreateDeps(&fakeAPIKeysService{}, func(map[string]any) APIKeyDrop {
		return drop
	}))

	_, err := op.Handler().Execute(context.Background(), map[string]any{"name": "test-key"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "drop store unavailable")
}

// TestAPIKeysCreateRequiresName pins the friendly required-arg error.
func TestAPIKeysCreateRequiresName(t *testing.T) {
	op := apiKeysCreate(apiKeysCreateDeps(&fakeAPIKeysService{}, nil))
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	require.Equal(t, "api_keys_create: key name is required", err.Error())
}
