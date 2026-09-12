package catalogops

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/pinner/core/apikeys"
)

// deleteRecordingAPIKeysService records the service call api_keys_delete makes
// so the confirm contract pins whether the deletion actually ran and with
// which force flag.
type deleteRecordingAPIKeysService struct {
	apikeys.Service

	gotID    string
	gotForce bool
	deleteFn func(idOrName string) error
}

func (f *deleteRecordingAPIKeysService) DeleteAPIKey(_ context.Context, idOrName string, force bool) error {
	f.gotID = idOrName
	f.gotForce = force
	if f.deleteFn != nil {
		return f.deleteFn(idOrName)
	}
	return nil
}

func apiKeysDeleteDeps(svc apikeys.Service) APIKeysDeps {
	return APIKeysDeps{Service: func(map[string]any) apikeys.Service { return svc }}
}

// TestAPIKeysDeleteCLIWithoutForceRuns pins the CLI contract: absent confirm
// (--force omitted) still executes and reaches the service with force=false.
// Deleting keys OTHER than the one currently authenticating must keep working
// without the flag; the self-delete force guard lives in the core service,
// which is the sole judge of the currently-authenticating key.
func TestAPIKeysDeleteCLIWithoutForceRuns(t *testing.T) {
	for _, input := range []map[string]any{{"id": "staging-key"}, {"id": "staging-key", "confirm": false}} {
		svc := &deleteRecordingAPIKeysService{}
		op := apiKeysDelete(apiKeysDeleteDeps(svc))

		res, err := op.Handler().Execute(context.Background(), input)
		require.NoError(t, err)
		require.Equal(t, "staging-key", svc.gotID)
		require.False(t, svc.gotForce, "service must see force=false")
		result, ok := res.(*APIKeyDeleteResult)
		require.True(t, ok, "result type = %T, want *APIKeyDeleteResult", res)
		require.Equal(t, "staging-key", result.ID)
	}
}

// TestAPIKeysDeleteForcePassesThrough pins that confirm=true rides to the
// service as force=true: that is the CLI's self-delete override (--force via
// GatePassthroughForce) AND the MCP hand-off's human confirmation value.
func TestAPIKeysDeleteForcePassesThrough(t *testing.T) {
	svc := &deleteRecordingAPIKeysService{}
	op := apiKeysDelete(apiKeysDeleteDeps(svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": "k", "confirm": true})
	require.NoError(t, err)
	require.Equal(t, "k", svc.gotID)
	require.True(t, svc.gotForce)
}

// TestAPIKeysDeleteDestructiveContract pinS the MCP-facing destructive-action
// contract in the descriptor itself: the description states the deletion is
// irreversible and names the human-confirmed confirm hand-off required for
// every key, while the CLI Help keeps its original self-delete-only semantic.
func TestAPIKeysDeleteDestructiveContract(t *testing.T) {
	op := apiKeysDelete(apiKeysDeleteDeps(&deleteRecordingAPIKeysService{}))
	require.Contains(t, op.Description(), "cannot be recovered")
	require.Contains(t, op.Description(), "confirm=true")
	require.Contains(t, op.Description(), "EVERY key")

	for _, a := range op.Args() {
		if a.Name != "confirm" {
			continue
		}
		require.False(t, a.AgentConfirm,
			"the confirm arg must NOT be AgentConfirm: api_keys_delete keeps the human hand-off for every deletion")
		require.Equal(t, "Allow deleting the key currently used for authentication", a.Help,
			"CLI flag help keeps the self-delete semantic")
	}
}

// TestAPIKeysDeleteServiceErrorPropagates pins that a service-side failure
// fails the invocation rather than being masked.
func TestAPIKeysDeleteServiceErrorPropagates(t *testing.T) {
	svc := &deleteRecordingAPIKeysService{deleteFn: func(string) error {
		return errors.New("api keys service unavailable")
	}}
	op := apiKeysDelete(apiKeysDeleteDeps(svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": "k", "confirm": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "api keys service unavailable")
}
