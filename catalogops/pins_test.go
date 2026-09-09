package catalogops

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
	"go.lumeweb.com/pinner/core/pinning"
)

// fakePinsService is a minimal in-package pinning.PinningService fake for the
// pins operations. All methods default to an "unimplemented" error so a test
// must opt into the behavior it exercises, surfacing unexpected calls
// (mirroring mockENSIPNSService in ens_test.go).
type fakePinsService struct {
	pinning.PinningService // unimplemented methods panic on the nil embedded interface

	list       func(ctx context.Context, opts pinning.ListOptions) ([]pinning.Pin, error)
	unpin      func(ctx context.Context, cid string, confirm bool) (*pinning.UnpinResult, error)
	unpinBatch func(ctx context.Context, cids []string, opts pinning.BatchOptions) (*pinning.BatchResult, error)
	unpinAll   func(ctx context.Context, statusFilter string, opts pinning.BatchOptions) (*pinning.BatchResult, error)
}

func (m *fakePinsService) RequireAuthenticated() error { return nil }

func (m *fakePinsService) List(ctx context.Context, opts pinning.ListOptions) ([]pinning.Pin, error) {
	if m.list == nil {
		return nil, errors.New("unexpected List")
	}
	return m.list(ctx, opts)
}

func (m *fakePinsService) Unpin(ctx context.Context, cid string, confirm bool) (*pinning.UnpinResult, error) {
	if m.unpin == nil {
		return nil, errors.New("unexpected Unpin")
	}
	return m.unpin(ctx, cid, confirm)
}

func (m *fakePinsService) UnpinBatch(ctx context.Context, cids []string, opts pinning.BatchOptions) (*pinning.BatchResult, error) {
	if m.unpinBatch == nil {
		return nil, errors.New("unexpected UnpinBatch")
	}
	return m.unpinBatch(ctx, cids, opts)
}

func (m *fakePinsService) UnpinAll(ctx context.Context, statusFilter string, opts pinning.BatchOptions) (*pinning.BatchResult, error) {
	if m.unpinAll == nil {
		return nil, errors.New("unexpected UnpinAll")
	}
	return m.unpinAll(ctx, statusFilter, opts)
}

// pinsDepsFor builds PinsDeps whose service is the given fake. Like
// ensDepsFor in ens_test.go, it routes through NewAuthenticated (the
// auth-token path) so service() returns the fake regardless of config.
func pinsDepsFor(t *testing.T, svc pinning.PinningService) PinsDeps {
	return PinsDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		NewAuthenticated: func(_ config.Manager, _ bool, _ string) pinning.PinningService {
			return svc
		},
		// A non-empty config token makes service() route through
		// NewAuthenticated (the auth-token path) instead of falling through to
		// a nil ServiceFactory. The value is an opaque placeholder, never
		// compared to a real secret.
		GetAuthToken: func() string { return "dummy" },
	}
}

// invokePinsRM registers the real pins operations in a fresh catalog and
// invokes pins_rm end to end. ActorHuman bypasses the catalog-level
// destructive gate so the handler's OWN gates and service calls are what get
// exercised (the model-actor gate lives in opmesh, not in pinsRemove).
func invokePinsRM(t *testing.T, svc pinning.PinningService, input map[string]any) (any, error) {
	t.Helper()
	cat := opmesh.NewCatalog()
	for _, op := range PinsOperations(pinsDepsFor(t, svc)) {
		if err := cat.Add(op); err != nil {
			t.Fatalf("Add(%q): %v", op.Name(), err)
		}
	}
	return cat.Invoke(context.Background(), "pins_rm", input, opmesh.ActorHuman)
}

// TestPinsRemove_All_RequiresConfirm pins the unpin-all confirmation gate:
// without confirm the handler must refuse before UnpinAll runs; with confirm
// it must delegate to UnpinAll with the threaded options.
func TestPinsRemove_All_RequiresConfirm(t *testing.T) {
	var unpinAllCalls int
	var gotStatusFilter string
	var gotOpts pinning.BatchOptions
	svc := &fakePinsService{
		unpinAll: func(_ context.Context, statusFilter string, opts pinning.BatchOptions) (*pinning.BatchResult, error) {
			unpinAllCalls++
			gotStatusFilter = statusFilter
			gotOpts = opts
			return &pinning.BatchResult{Total: 1}, nil
		},
	}

	t.Run("all without confirm is rejected", func(t *testing.T) {
		unpinAllCalls = 0
		// confirm is a required arg with no default; false is a filled value
		// that runs the handler, which must then refuse the mutation — the
		// same convention as ens_test.go's "human unpoint without confirm".
		_, err := invokePinsRM(t, svc, map[string]any{"all": true, "confirm": false})
		require.Error(t, err)
		require.Contains(t, err.Error(), "confirmation is required")
		require.Equal(t, 0, unpinAllCalls, "UnpinAll must not run without confirm")
	})

	t.Run("all with confirm unpins", func(t *testing.T) {
		res, err := invokePinsRM(t, svc, map[string]any{"all": true, "confirm": true})
		require.NoError(t, err)
		require.Equal(t, 1, unpinAllCalls, "confirm=true must delegate to UnpinAll")
		require.Equal(t, "", gotStatusFilter)
		require.True(t, gotOpts.Progress, "unpin-all must run with Progress (matches the pre-gate behavior)")
		br, ok := res.(*pinning.BatchResult)
		require.True(t, ok)
		require.Equal(t, 1, br.Total)
	})
}

// TestPinsRemove_Batch_RequiresConfirm pins the multi-CID confirmation gate:
// without confirm the handler refuses before UnpinBatch runs; with confirm it
// delegates to UnpinBatch with the original CID order.
func TestPinsRemove_Batch_RequiresConfirm(t *testing.T) {
	var unpinBatchCalls int
	var gotCIDs []string
	svc := &fakePinsService{
		unpinBatch: func(_ context.Context, cids []string, _ pinning.BatchOptions) (*pinning.BatchResult, error) {
			unpinBatchCalls++
			gotCIDs = append([]string(nil), cids...)
			return &pinning.BatchResult{Total: len(cids)}, nil
		},
	}

	cids := []string{"bafybeigone", "bafybegtwo"}

	t.Run("batch without confirm is rejected", func(t *testing.T) {
		unpinBatchCalls = 0
		_, err := invokePinsRM(t, svc, map[string]any{"cids": cids, "confirm": false})
		require.Error(t, err)
		require.Contains(t, err.Error(), "confirmation is required")
		require.Equal(t, 0, unpinBatchCalls, "UnpinBatch must not run without confirm")
	})

	t.Run("batch with confirm unpins", func(t *testing.T) {
		res, err := invokePinsRM(t, svc, map[string]any{"cids": cids, "confirm": true})
		require.NoError(t, err)
		require.Equal(t, 1, unpinBatchCalls, "confirm=true must delegate to UnpinBatch")
		require.Equal(t, cids, gotCIDs)
		br, ok := res.(*pinning.BatchResult)
		require.True(t, ok)
		require.Equal(t, len(cids), br.Total)
	})
}

// TestPinsRemove_All_DryRunWithoutConfirm pins that a dry-run is non-mutating
// and therefore NOT blocked by the confirmation gate: all=true + dry-run=true
// without confirm must return the DryRunResult preview built from a read-only
// List, and must never call UnpinAll.
func TestPinsRemove_All_DryRunWithoutConfirm(t *testing.T) {
	var unpinAllCalls int
	svc := &fakePinsService{
		list: func(_ context.Context, _ pinning.ListOptions) ([]pinning.Pin, error) {
			return []pinning.Pin{{RequestID: "req-1", CID: "bafybeigone"}, {RequestID: "req-2", CID: "bafybegtwo"}}, nil
		},
		unpinAll: func(_ context.Context, _ string, _ pinning.BatchOptions) (*pinning.BatchResult, error) {
			unpinAllCalls++
			return &pinning.BatchResult{}, nil
		},
	}

	res, err := invokePinsRM(t, svc, map[string]any{"all": true, "dry-run": true, "confirm": false})
	require.NoError(t, err, "dry-run must work without confirm (it is non-mutating)")
	require.Equal(t, 0, unpinAllCalls, "dry-run must never call UnpinAll")

	dr, ok := res.(*DryRunResult)
	require.True(t, ok)
	require.Equal(t, []string{"req-1", "req-2"}, dr.CIDs)
}

// TestPinsRemove_SingleUnpinsWithConfirm pins that the single-CID path still
// threads confirm=true through to Unpin after the batch/all gates were added.
func TestPinsRemove_SingleUnpinsWithConfirm(t *testing.T) {
	var gotCID string
	var gotConfirm bool
	svc := &fakePinsService{
		unpin: func(_ context.Context, cid string, confirm bool) (*pinning.UnpinResult, error) {
			gotCID = cid
			gotConfirm = confirm
			return &pinning.UnpinResult{CID: cid}, nil
		},
	}

	res, err := invokePinsRM(t, svc, map[string]any{"cids": []string{"bafybeigone"}, "confirm": true})
	require.NoError(t, err)
	require.Equal(t, "bafybeigone", gotCID)
	require.True(t, gotConfirm, "single-CID Unpin must receive confirm=true")
	r, ok := res.(*pinning.UnpinResult)
	require.True(t, ok)
	require.Equal(t, "bafybeigone", r.CID)
}
