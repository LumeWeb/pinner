package catalogops

import (
	"context"
	"testing"
	"time"

	"go.lumeweb.com/pinner/core/operations"
)

// fakeOperationsService is a minimal operations.Service fake: it embeds the
// interface for the untouched methods and records every List call so tests
// can assert the watch polling contract.
type fakeOperationsService struct {
	operations.Service

	listFn func(ctx context.Context, opts operations.ListOptions) (*operations.OperationsListResult, error)

	listCalls int
	listOpts  []operations.ListOptions
}

func (f *fakeOperationsService) RequireAuthenticated() error { return nil }

func (f *fakeOperationsService) List(ctx context.Context, opts operations.ListOptions) (*operations.OperationsListResult, error) {
	f.listCalls++
	f.listOpts = append(f.listOpts, opts)
	if f.listFn != nil {
		return f.listFn(ctx, opts)
	}
	return &operations.OperationsListResult{}, nil
}

func sampleOperationItem(id int, status string) operations.OperationListItem {
	return operations.OperationListItem{
		ID:     id,
		Status: status,
	}
}

func withTightWatchBounds(t *testing.T) {
	t.Helper()
	prevInterval, prevAttempts := operationsListWatchInterval, operationsListWatchAttempts
	operationsListWatchInterval = time.Millisecond
	operationsListWatchAttempts = 5
	t.Cleanup(func() {
		operationsListWatchInterval = prevInterval
		operationsListWatchAttempts = prevAttempts
	})
}

// TestOperationsListWatchHonorHelpers pins the settle check used by the poll
// loop.
func TestAllOperationsSettled(t *testing.T) {
	if !allOperationsSettled(nil) {
		t.Fatal("nil result should count as settled")
	}
	if !allOperationsSettled(&operations.OperationsListResult{}) {
		t.Fatal("empty result should count as settled")
	}
	settled := &operations.OperationsListResult{Operations: []operations.OperationListItem{
		sampleOperationItem(1, "completed"),
		sampleOperationItem(2, "failed"),
		sampleOperationItem(3, "duplicate"),
	}}
	if !allOperationsSettled(settled) {
		t.Fatal("all-terminal result should count as settled")
	}
	active := &operations.OperationsListResult{Operations: []operations.OperationListItem{
		sampleOperationItem(1, "completed"),
		sampleOperationItem(2, "processing"),
	}}
	if allOperationsSettled(active) {
		t.Fatal("result with a processing op must not count as settled")
	}
}

// TestOperationsListWatchPollsUntilSettled pins that watch=true re-lists (with
// IsWatch set so the row set survives settlement) until every operation has
// reached a terminal status, then returns the settled listing.
func TestOperationsListWatchPollsUntilSettled(t *testing.T) {
	withTightWatchBounds(t)

	// The first two polls still see an active operation; the third observes
	// the terminal transition.
	var svc *fakeOperationsService
	svc = &fakeOperationsService{
		listFn: func(ctx context.Context, opts operations.ListOptions) (*operations.OperationsListResult, error) {
			if svc.listCalls < 3 {
				return &operations.OperationsListResult{
					Operations: []operations.OperationListItem{sampleOperationItem(1, "processing")},
					Total:      1,
				}, nil
			}
			return &operations.OperationsListResult{
				Operations: []operations.OperationListItem{sampleOperationItem(1, "completed")},
				Total:      1,
			}, nil
		},
	}
	op := operationsList(OperationsDeps{Service: func(input map[string]any) operations.Service { return svc }})

	res, err := op.Handler().Execute(context.Background(), map[string]any{"watch": true})
	if err != nil {
		t.Fatalf("watch list: %v", err)
	}
	if svc.listCalls < 3 {
		t.Fatalf("list calls = %d, want at least 3 (poll until settled)", svc.listCalls)
	}
	if _, ok := res.(ListResult); !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	for i, opts := range svc.listOpts {
		if !opts.IsWatch {
			t.Fatalf("list call %d: IsWatch = false, want true", i)
		}
	}
}

// TestOperationsListNoWatchSingleCall pins that without watch the handler
// performs exactly one list call and does not set IsWatch.
func TestOperationsListNoWatchSingleCall(t *testing.T) {
	svc := &fakeOperationsService{}
	op := operationsList(OperationsDeps{Service: func(input map[string]any) operations.Service { return svc }})

	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if svc.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", svc.listCalls)
	}
	if svc.listOpts[0].IsWatch {
		t.Fatal("IsWatch = true without watch arg")
	}
}

// TestOperationsListWatchTimesOut pins the bounded-poll contract: watch gives
// up with a clear error instead of polling forever.
func TestOperationsListWatchTimesOut(t *testing.T) {
	withTightWatchBounds(t)

	svc := &fakeOperationsService{
		listFn: func(ctx context.Context, opts operations.ListOptions) (*operations.OperationsListResult, error) {
			return &operations.OperationsListResult{
				Operations: []operations.OperationListItem{sampleOperationItem(1, "pending")},
				Total:      1,
			}, nil
		},
	}
	op := operationsList(OperationsDeps{Service: func(input map[string]any) operations.Service { return svc }})

	_, err := op.Handler().Execute(context.Background(), map[string]any{"watch": true})
	if err == nil {
		t.Fatal("expected a timeout error when operations never settle")
	}
	if svc.listCalls != operationsListWatchAttempts {
		t.Fatalf("list calls = %d, want %d (bounded attempts)", svc.listCalls, operationsListWatchAttempts)
	}
}
