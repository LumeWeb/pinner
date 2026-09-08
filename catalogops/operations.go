// Package catalogops implements operations-domain operations for the
// operation pinner. catalogops depends only on the core OperationsService
// interface and injects the concrete service via deps.
package catalogops

import (
	"context"
	"fmt"

	"go.lumeweb.com/pinner"
	"go.lumeweb.com/pinner/core/operations"
)

// OperationsDeps injects an OperationsService. The concrete implementation
// lives in the pkg/cli frontend (NewOperationsService); catalogops only
// depends on its contract.
type OperationsDeps struct {
	// Service returns a live OperationsService for the current invocation,
	// honoring the per-invocation auth-token override in the input map.
	Service func(input map[string]any) operations.Service
}

// OperationsOperations returns the catalog operations for the operations
// domain (operations list, operations get).
func OperationsOperations(d OperationsDeps) []pinner.Operation {
	return []pinner.Operation{
		operationsList(d),
		operationsGet(d),
	}
}

func operationsList(d OperationsDeps) pinner.Operation {
	return pinner.NewOperation(pinner.OperationSpec{
		Name: "operations_list", Title: "List account operations", Summary: "List account operations",
		Description: "List account operations (uploads, pins, and other processing tasks) with optional filters and pagination. By default only active operations (pending, processing) are shown; pass --all to include completed, failed, and duplicate operations.",
		Category:    "operations", Safety: pinner.SafetyRead, Interaction: pinner.InteractionAgentSafe, Visibility: pinner.VisibilityBoth,
		Positional: "",
		Args: append(pinner.ListArgs(),
			pinner.OperationArg{Name: "search", Type: pinner.ArgTypeString, Help: "Full-text search evaluated server-side against operation type, status, protocol, or CID; composes with the filters below", AgentHelp: "Full-text search term evaluated server-side against operation type, status, protocol, or CID. Composes (AND) with the structured filters."},
			pinner.OperationArg{Name: "status", Type: pinner.ArgTypeStringSlice, Help: "Filter by status (repeatable; pending, processing, completed, failed, duplicate)", AgentHelp: "One or more statuses to filter by. Valid values: pending, processing, completed, failed, duplicate. When omitted, only active operations (pending, processing) are returned unless all=true."},
			pinner.OperationArg{Name: "all", Type: pinner.ArgTypeBool, Default: "false", Help: "Show operations in all statuses (overrides the default active-only filter)", AgentHelp: "When true, return operations in any status, overriding the default that shows only pending and processing. Ignored when status is explicitly provided."},
			pinner.OperationArg{Name: "operation", Type: pinner.ArgTypeString, Help: "Filter by operation type (e.g. upload, pin)"},
			pinner.OperationArg{Name: "protocol", Type: pinner.ArgTypeString, Help: "Filter by protocol (e.g. ipfs)"},
			pinner.OperationArg{Name: "cid", Type: pinner.ArgTypeString, Help: "Filter by CID"},
			pinner.OperationArg{Name: "sort", Type: pinner.ArgTypeString, Help: "Sort results (e.g. id:desc, started:asc). Defaults to id:desc.", AgentHelp: "Sort field and direction, e.g. \"id:desc\" or \"started:asc\". Defaults to id:desc."},
			pinner.OperationArg{Name: "watch", Type: pinner.ArgTypeBool, Default: "false", Help: "Poll until the list settles"},
		),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("operations service unavailable")
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			page := pinner.ParseListPage(input, 10)
			res, err := svc.List(ctx, operations.ListOptions{
				Search:          pinner.SearchArg(input),
				StatusFilters:   pinner.StrSliceArg(input, "status"),
				IncludeAll:      pinner.BoolArg(input, "all", false),
				OperationFilter: pinner.StrArg(input, "operation", ""),
				ProtocolFilter:  pinner.StrArg(input, "protocol", ""),
				CIDFilter:       pinner.StrArg(input, "cid", ""),
				Sort:            pinner.StrArg(input, "sort", ""),
				Start:           page.Start,
				Limit:           page.Limit,
			})
			if err != nil {
				return nil, err
			}
			return newOperationsListResult(res), nil
		}),
	})
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

func operationsGet(d OperationsDeps) pinner.Operation {
	return pinner.NewOperation(pinner.OperationSpec{
		Name: "operations_get", Title: "Get operation details", Summary: "Get details of an operation",
		Description: "Get the full details of a single account operation by ID, optionally waiting for it to complete.",
		Category:    "operations", Safety: pinner.SafetyRead, Interaction: pinner.InteractionAgentSafe, Visibility: pinner.VisibilityBoth,
		Positional: "<id>",
		Args: []pinner.OperationArg{
			{Name: "id", Type: pinner.ArgTypeInt, Required: true, Help: "Operation ID"},
			{Name: "watch", Type: pinner.ArgTypeBool, Default: "false", Help: "Wait for the operation to complete"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("operations service unavailable")
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := int64(pinner.IntArg(input, "id", 0))
			if id == 0 {
				return nil, fmt.Errorf("operations_get: operation ID is required")
			}
			if pinner.BoolArg(input, "watch", false) {
				return svc.Watch(ctx, id)
			}
			return svc.Get(ctx, id)
		}),
	})
}
