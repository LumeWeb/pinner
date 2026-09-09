// Package catalogops implements operations-domain operations for the
// operation pinner. catalogops depends only on the core OperationsService
// interface and injects the concrete service via deps.
package catalogops

import (
	"context"
	"fmt"
	"time"

	"go.lumeweb.com/opmesh"
	account "go.lumeweb.com/portal-sdk"
	"go.lumeweb.com/pinner/core/operations"
)

// OperationsDeps injects an OperationsService (e.g. NewOperationsService);
// catalogops only depends on its contract, and the caller wires the concrete
// implementation.
type OperationsDeps struct {
	// Service returns a live OperationsService for the current invocation,
	// honoring the per-invocation auth-token override in the input map.
	Service func(input map[string]any) operations.Service
}

// OperationsOperations returns the catalog operations for the operations
// domain (operations list, operations get).
func OperationsOperations(d OperationsDeps) []opmesh.Operation {
	return []opmesh.Operation{
		operationsList(d),
		operationsGet(d),
	}
}

func operationsList(d OperationsDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name: "operations_list", Title: "List account operations", Summary: "List account operations",
		Description: "List account operations (uploads, pins, and other processing tasks) with optional filters and pagination. By default only active operations (pending, processing) are shown; pass --all to include completed, failed, and duplicate operations.",
		Category:    "operations", Safety: opmesh.SafetyRead, Interaction: opmesh.InteractionAgentSafe, Visibility: opmesh.VisibilityBoth,
		Positional: "",
		Args: append(opmesh.ListArgs(),
			opmesh.OperationArg{Name: "search", Type: opmesh.ArgTypeString, Help: "Full-text search evaluated server-side against operation type, status, protocol, or CID; composes with the filters below"},
			opmesh.OperationArg{Name: "status", Type: opmesh.ArgTypeStringSlice, Help: "Filter by status (repeatable; pending, processing, completed, failed, duplicate)"},
			opmesh.OperationArg{Name: "all", Type: opmesh.ArgTypeBool, Default: "false", Help: "Show operations in all statuses (overrides the default active-only filter)"},
			opmesh.OperationArg{Name: "operation", Type: opmesh.ArgTypeString, Help: "Filter by operation type (e.g. upload, pin)"},
			opmesh.OperationArg{Name: "protocol", Type: opmesh.ArgTypeString, Help: "Filter by protocol (e.g. ipfs)"},
			opmesh.OperationArg{Name: "cid", Type: opmesh.ArgTypeString, Help: "Filter by CID"},
			opmesh.OperationArg{Name: "sort", Type: opmesh.ArgTypeString, Help: "Sort results (e.g. id:desc, started:asc). Defaults to id:desc."},
			opmesh.OperationArg{Name: "watch", Type: opmesh.ArgTypeBool, Default: "false", Help: "Poll until all listed operations settle (bounded poll)"},
		),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("operations service unavailable")
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			page := opmesh.ParseListPage(input, 10)
			opts := operations.ListOptions{
				Search:          opmesh.SearchArg(input),
				StatusFilters:   opmesh.StrSliceArg(input, "status"),
				IncludeAll:      opmesh.BoolArg(input, "all", false),
				OperationFilter: opmesh.StrArg(input, "operation", ""),
				ProtocolFilter:  opmesh.StrArg(input, "protocol", ""),
				CIDFilter:       opmesh.StrArg(input, "cid", ""),
				Sort:            opmesh.StrArg(input, "sort", ""),
				Start:           page.Start,
				Limit:           page.Limit,
			}
			if opmesh.BoolArg(input, "watch", false) {
				// Watch keeps rows visible past settlement (the service skips
				// the default active-status filter) so the settle check below
				// can observe the terminal transition.
				opts.IsWatch = true
				return pollOperationsList(ctx, svc, opts)
			}
			res, err := svc.List(ctx, opts)
			if err != nil {
				return nil, err
			}
			return newOperationsListResult(res), nil
		}),
	})
}

// Watch polling knobs. Vars (not consts) so tests can tighten the interval
// and attempts without wall-clock-sensitive sleeps.
var (
	// operationsListWatchInterval is the delay between re-list attempts while
	// watching the operations list for settlement.
	operationsListWatchInterval = 2 * time.Second
	// operationsListWatchAttempts bounds watch polling so operations that
	// never settle cannot block an agent indefinitely (~5 minutes at the
	// default interval).
	operationsListWatchAttempts = 150
)

// allOperationsSettled reports whether every listed operation has reached a
// terminal status (an empty list is trivially settled).
func allOperationsSettled(res *operations.OperationsListResult) bool {
	if res == nil {
		return true
	}
	for _, op := range res.Operations {
		if !account.OperationStatus(op.Status).IsSettled() {
			return false
		}
	}
	return true
}

// pollOperationsList re-lists until every returned operation has settled, the
// bound expires, or the context is canceled — mirroring the CLI's
// watch-on-list loop and honoring the documented `watch` contract.
func pollOperationsList(ctx context.Context, svc operations.Service, opts operations.ListOptions) (any, error) {
	// The settlement decision must span ALL matching rows, not just the
	// caller's requested slice: with watch=true the service returns every
	// status (IsWatch skips the active-only filter), and a pending operation
	// outside that slice — whether on a later page or BEFORE the caller's
	// Start offset — would otherwise be missed. The backend treats a positive
	// Limit as page size and Limit 0 as "unbounded", so the watch path clears
	// BOTH the page size and the Start offset and every poll fetches the
	// complete matching result set. Non-watch listings keep their normalized
	// pagination untouched.
	opts.Limit = 0
	opts.Start = 0
	for attempt := 0; attempt < operationsListWatchAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(operationsListWatchInterval):
			}
		}
		res, err := svc.List(ctx, opts)
		if err != nil {
			return nil, err
		}
		if allOperationsSettled(res) {
			return newOperationsListResult(res), nil
		}
	}
	return nil, fmt.Errorf("operations_list: timed out waiting for operations to settle")
}

// newOperationsListResult wraps the core operations list result into the
// shared ListResult contract so the CLI and MCP surfaces render it uniformly.
func newOperationsListResult(res *operations.OperationsListResult) ListResult {
	headers := []string{"ID", "OPERATION", "PROTOCOL", "STATUS", "CID", "STARTED"}
	if res == nil {
		return NewListResult([]operations.OperationListItem{}, ListResultMeta{
			Noun: "operation(s)", Headers: headers,
		})
	}
	rows := make([][]string, 0, len(res.Operations))
	for _, o := range res.Operations {
		cid := o.CID
		if cid == "" {
			cid = "-"
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", o.ID),
			o.OperationDisplayName,
			o.ProtocolDisplayName,
			o.StatusDisplayName,
			cid,
			o.StartedAt,
		})
	}
	return NewListResult(res.Operations, ListResultMeta{
		Noun: "operation(s)", Headers: headers, Rows: rows, Total: res.Total,
	})
}

func operationsGet(d OperationsDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name: "operations_get", Title: "Get operation details", Summary: "Get details of an operation",
		Description: "Get the full details of a single account operation by ID, optionally waiting for it to complete.",
		Category:    "operations", Safety: opmesh.SafetyRead, Interaction: opmesh.InteractionAgentSafe, Visibility: opmesh.VisibilityBoth,
		Positional: "<id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeInt, Required: true, Help: "Operation ID"},
			{Name: "watch", Type: opmesh.ArgTypeBool, Default: "false", Help: "Wait for the operation to complete"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("operations service unavailable")
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := int64(opmesh.IntArg(input, "id", 0))
			if id == 0 {
				return nil, fmt.Errorf("operations_get: operation ID is required")
			}
			if opmesh.BoolArg(input, "watch", false) {
				return svc.Watch(ctx, id)
			}
			return svc.Get(ctx, id)
		}),
	})
}
