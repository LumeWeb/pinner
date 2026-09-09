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

// pagingOperationsService paginates f.rows by the Start/Limit cursor exactly
// like the concrete operations service (a positive Limit is a page size; Limit
// 0 means unbounded), so tests can exercise multi-page listings deterministically.
type pagingOperationsService struct {
	operations.Service

	rows []operations.OperationListItem
	// settleRowIdx/settleTo/settleAfterCall model a pending operation on a
	// later page transitioning to a terminal status between polls: from
	// poll (settleAfterCall + 1) onward, rows[settleRowIdx].Status is settleTo.
	settleRowIdx    int
	settleTo        string
	settleAfterCall int

	calls int
	opts  []operations.ListOptions
}

func (f *pagingOperationsService) RequireAuthenticated() error { return nil }

func (f *pagingOperationsService) List(ctx context.Context, opts operations.ListOptions) (*operations.OperationsListResult, error) {
	f.calls++
	f.opts = append(f.opts, opts)
	if f.settleAfterCall > 0 && f.calls > f.settleAfterCall && f.settleRowIdx < len(f.rows) {
		f.rows[f.settleRowIdx].Status = f.settleTo
	}
	if opts.Limit <= 0 {
		return &operations.OperationsListResult{
			Operations: append([]operations.OperationListItem(nil), f.rows...),
			Total:      len(f.rows),
		}, nil
	}
	start, end := opts.Start, opts.Start+opts.Limit
	if start > len(f.rows) {
		start = len(f.rows)
	}
	if end > len(f.rows) {
		end = len(f.rows)
	}
	return &operations.OperationsListResult{
		Operations: append([]operations.OperationListItem(nil), f.rows[start:end]...),
		Total:      len(f.rows),
	}, nil
}

// TestOperationsListWatchSettlesAcrossAllPages is a regression test: with
// watch=true and multiple pages where the FIRST page is settled but a LATER
// page still holds a pending operation, the watch must not exit on page 1. It
// must keep polling until the pending row on the later page reaches a terminal
// status, and only then return success.
func TestOperationsListWatchSettlesAcrossAllPages(t *testing.T) {
	withTightWatchBounds(t)

	// Page 1 holds a settled operation; page 2 holds a pending one that
	// transitions to a terminal status only after the first poll.
	svc := &pagingOperationsService{
		rows: []operations.OperationListItem{
			sampleOperationItem(1, "completed"),
			sampleOperationItem(2, "processing"),
		},
		settleRowIdx:    1,
		settleTo:        "completed",
		settleAfterCall: 1,
	}
	op := operationsList(OperationsDeps{Service: func(input map[string]any) operations.Service { return svc }})

	// page-size 1 forces a paginated initial options set; the watch path
	// must raise it so every poll sees ALL rows, not just page 1. A watch
	// that trusted the first (settled) page would return after ONE call —
	// before the pending row on page 2 was even observed.
	res, err := op.Handler().Execute(context.Background(), map[string]any{
		"watch": true, "page-size": 1,
	})
	if err != nil {
		t.Fatalf("watch list across pages: %v", err)
	}
	if svc.calls < 2 {
		t.Fatalf("list calls = %d, want >= 2 (must keep polling past the settled first page until the pending later page settles)", svc.calls)
	}
	lr, ok := res.(ListResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if lr.ListTotal() != 2 {
		t.Fatalf("Total = %d, want 2 (both rows)", lr.ListTotal())
	}
	for i, opts := range svc.opts {
		if opts.Limit != 0 {
			t.Fatalf("poll %d: Limit = %d, want 0 (unbounded) so settlement spans all pages", i, opts.Limit)
		}
		if !opts.IsWatch {
			t.Fatalf("poll %d: IsWatch = false, want true", i)
		}
	}
}

// TestOperationsListWatchAllPagesSettledSuccess pins that when every
// operation across the full (multi-row) result set is already terminal, the
// watch returns success on the first poll.
func TestOperationsListWatchAllPagesSettledSuccess(t *testing.T) {
	withTightWatchBounds(t)

	svc := &pagingOperationsService{rows: []operations.OperationListItem{
		sampleOperationItem(1, "completed"),
		sampleOperationItem(2, "failed"),
	}}
	op := operationsList(OperationsDeps{Service: func(input map[string]any) operations.Service { return svc }})

	res, err := op.Handler().Execute(context.Background(), map[string]any{
		"watch": true, "page-size": 1,
	})
	if err != nil {
		t.Fatalf("watch list: %v", err)
	}
	if svc.calls != 1 {
		t.Fatalf("list calls = %d, want 1 (all rows settled up front)", svc.calls)
	}
	lr, ok := res.(ListResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if lr.ListTotal() != 2 {
		t.Fatalf("Total = %d, want 2 (all rows, not just page size)", lr.ListTotal())
	}
	if svc.opts[0].Limit != 0 {
		t.Fatalf("watch Limit = %d, want 0 (unbounded)", svc.opts[0].Limit)
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
