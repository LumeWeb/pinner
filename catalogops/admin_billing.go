package catalogops

import (
	"context"
	"encoding/json"
	"fmt"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/portal-sdk/admin"
)

// catalogFloatArg reads a float arg from the input map, handling the numeric
// representations the frontends may produce (float64/float32/int/json.Number).
func catalogFloatArg(input map[string]any, key string, def float64) float64 {
	v, ok := input[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	}
	return def
}

// BillingCreditsListResult wraps the credits list result.
type BillingCreditsListResult struct {
	Count   int                 `json:"count"`
	Credits []*admin.CreditItem `json:"credits"`
}

// BillingPriceLinesListResult wraps the price lines list result.
type BillingPriceLinesListResult struct {
	Count      int                `json:"count"`
	PriceLines []*admin.PriceLine `json:"price_lines"`
}

// BillingPricingPlansListResult wraps the pricing plans list result.
type BillingPricingPlansListResult struct {
	Count int                      `json:"count"`
	Plans []*admin.PricingPlanItem `json:"plans"`
}

// BillingPricingPlanPeriodsListResult wraps the pricing plan periods list.
type BillingPricingPlanPeriodsListResult struct {
	Count   int                        `json:"count"`
	Periods []*admin.PricingPlanPeriod `json:"periods"`
}

// BillingSubscribersListResult wraps the subscribers list result.
type BillingSubscribersListResult struct {
	Count       int                 `json:"count"`
	Subscribers []*admin.Subscriber `json:"subscribers"`
}

// BillingPurgeResult reports the purged credit count.
type BillingPurgeResult struct {
	Purged int `json:"purged"`
}

// BillingDeletedCreditsListResult wraps the deleted credits list result.
type BillingDeletedCreditsListResult struct {
	Count   int                 `json:"count"`
	Credits []*admin.CreditItem `json:"credits"`
}

// BillingGenericActionResult reports a side-effect action outcome.
type BillingGenericActionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// BillingSyncResult reports a pricing plan sync.
type BillingSyncResult struct {
	Synced bool   `json:"synced"`
	PlanID string `json:"plan_id,omitempty"`
}

// BillingUserDeletedCredits wraps the deleted-credits-of-user list.
type BillingUserDeletedCredits struct {
	UserID  string              `json:"user_id"`
	Count   int                 `json:"count"`
	Credits []*admin.CreditItem `json:"credits"`
}

// BillingUserBalanceResult wraps the user balance.
type BillingUserBalanceResult struct {
	Balance *admin.UserBalance `json:"balance"`
}

// adminBillingCreditsList is the `admin billing credits list` operation.
func adminBillingCreditsList(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_list",
		Title:       "List billing credits",
		Summary:     "List billing credits",
		Description: "List billing credits with optional filters. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: append(opmesh.ListArgs(),
			opmesh.OperationArg{Name: "user-id", Type: opmesh.ArgTypeString, Help: "Filter by user ID"},
			opmesh.OperationArg{Name: "direction", Type: opmesh.ArgTypeString, Help: "Filter by direction (credit, debit)"},
			opmesh.OperationArg{Name: "type", Type: opmesh.ArgTypeString, Help: "Filter by type"},
		),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			params := &admin.GetApiBillingCreditsParams{}
			if v := opmesh.StrArg(input, "user-id", ""); v != "" {
				params.FiltersUserIdEq = &v
			}
			if v := opmesh.StrArg(input, "direction", ""); v != "" {
				params.DirectionEq = &v
			}
			if v := opmesh.StrArg(input, "type", ""); v != "" {
				params.TypeEq = &v
			}
			credits, _, err := svc.ListCredits(ctx, params)
			if err != nil {
				return nil, err
			}
			page := opmesh.ParseList(input)
			return billingCreditsListResult(slicePage(credits, page.Start, page.Limit)), nil
		}),
	})
}

// adminBillingCreditsGet is the `admin billing credits get` operation.
func adminBillingCreditsGet(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_get",
		Title:       "Get a credit",
		Summary:     "Get a credit by ID",
		Description: "Get a single billing credit by its ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<credit-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Credit ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_credits_get: credit ID is required")
			}
			return svc.GetCredit(ctx, id)
		}),
	})
}

// adminBillingCreditsCreate is the `admin billing credits create` operation.
func adminBillingCreditsCreate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_create",
		Title:       "Create a credit",
		Summary:     "Create a billing credit",
		Description: "Create a billing credit entry for a user. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeInt, Required: true, Help: "User ID"},
			{Name: "amount", Type: opmesh.ArgTypeString, Required: true, Help: "Amount"},
			{Name: "type", Type: opmesh.ArgTypeString, Required: true, Help: "Credit type"},
			{Name: "direction", Type: opmesh.ArgTypeString, Required: true, Help: "Direction (credit, debit)"},
			{Name: "description", Type: opmesh.ArgTypeString, Help: "Description"},
			{Name: "reference-id", Type: opmesh.ArgTypeString, Help: "Reference ID"},
			{Name: "reference-type", Type: opmesh.ArgTypeString, Help: "Reference type"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			userID := opmesh.IntArg(input, "user-id", 0)
			if userID == 0 {
				return nil, fmt.Errorf("admin_billing_credits_create: user-id is required")
			}
			req := &admin.CreditCreateRequest{
				Amount:    opmesh.StrArg(input, "amount", ""),
				Direction: opmesh.StrArg(input, "direction", ""),
				Type:      opmesh.StrArg(input, "type", ""),
				UserId:    userID,
			}
			if v := opmesh.StrArg(input, "description", ""); v != "" {
				req.Description = &v
			}
			if v := opmesh.StrArg(input, "reference-id", ""); v != "" {
				req.ReferenceId = &v
			}
			if v := opmesh.StrArg(input, "reference-type", ""); v != "" {
				req.ReferenceType = &v
			}
			return svc.CreateCredit(ctx, req)
		}),
	})
}

// adminBillingCreditsDelete is the `admin billing credits delete` operation.
func adminBillingCreditsDelete(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_delete",
		Title:       "Delete a credit",
		Summary:     "Soft-delete a credit",
		Description: "Soft-delete a billing credit by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<credit-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Credit ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_credits_delete: credit ID is required")
			}
			if err := svc.DeleteCredit(ctx, id); err != nil {
				return nil, err
			}
			return &BillingGenericActionResult{Success: true, Message: "credit deleted"}, nil
		}),
	})
}

// adminBillingCreditsRestore is the `admin billing credits restore` operation.
func adminBillingCreditsRestore(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_restore",
		Title:       "Restore a credit",
		Summary:     "Restore a soft-deleted credit",
		Description: "Restore a soft-deleted billing credit by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<credit-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Credit ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_credits_restore: credit ID is required")
			}
			return svc.RestoreCredit(ctx, id)
		}),
	})
}

// adminBillingCreditsPurge is the `admin billing credits purge` operation.
func adminBillingCreditsPurge(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_purge",
		Title:       "Purge credits",
		Summary:     "Purge soft-deleted credits",
		Description: "Permanently remove soft-deleted credits older than a duration. DESTRUCTIVE: requires confirm=true. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyDestructive,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: []opmesh.OperationArg{
			{Name: "older-than", Type: opmesh.ArgTypeString, Required: true, Help: "Age threshold, e.g. 720h"},
			{Name: "confirm", Type: opmesh.ArgTypeBool, Required: true, Help: "Confirm the destructive purge"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			if !opmesh.BoolArg(input, "confirm", false) {
				return nil, fmt.Errorf("admin_billing_credits_purge: confirmation is required")
			}
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			older := opmesh.StrArg(input, "older-than", "")
			if older == "" {
				return nil, fmt.Errorf("admin_billing_credits_purge: older-than is required")
			}
			req := &admin.CreditPurgeRequest{OlderThan: older}
			n, err := svc.PurgeCredits(ctx, req)
			if err != nil {
				return nil, err
			}
			return &BillingPurgeResult{Purged: n}, nil
		}),
	})
}

// adminBillingCreditsUserBalance is the `admin billing credits user-balance`
// operation.
func adminBillingCreditsUserBalance(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_user_balance",
		Title:       "View a user's credit balance",
		Summary:     "View a user's balance",
		Description: "View the current credit balance for a user. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_credits_user_balance: user-id is required")
			}
			bal, err := svc.GetUserBalance(ctx, uid)
			if err != nil {
				return nil, err
			}
			return &BillingUserBalanceResult{Balance: bal}, nil
		}),
	})
}

// adminBillingCreditsUserDeletedCredits is the
// `admin billing credits user-deleted-credits` operation.
func adminBillingCreditsUserDeletedCredits(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_credits_user_deleted_credits",
		Title:       "List a user's deleted credits",
		Summary:     "List a user's soft-deleted credits",
		Description: "List the soft-deleted credits for a user. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_credits_user_deleted_credits: user-id is required")
			}
			credits, total, err := svc.GetUserDeletedCredits(ctx, uid, nil)
			if err != nil {
				return nil, err
			}
			return &BillingUserDeletedCredits{UserID: uid, Count: total, Credits: credits}, nil
		}),
	})
}

// adminBillingPriceLinesList is the `admin billing price-lines list` operation.
func adminBillingPriceLinesList(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_list",
		Title:       "List price lines",
		Summary:     "List billing price lines",
		Description: "List all billing price lines. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args:        opmesh.ListArgs(),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			lines, _, err := svc.ListPriceLines(ctx)
			if err != nil {
				return nil, err
			}
			page := opmesh.ParseList(input)
			return billingPriceLinesListResult(slicePage(lines, page.Start, page.Limit)), nil
		}),
	})
}

// adminBillingPriceLinesGet is the `admin billing price-lines get` operation.
func adminBillingPriceLinesGet(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_get",
		Title:       "Get a price line",
		Summary:     "Get a price line by ID",
		Description: "Get a single billing price line with its associated plans. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<price-line-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Price line ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_price_lines_get: price line ID is required")
			}
			return svc.GetPriceLine(ctx, id)
		}),
	})
}

// adminBillingPriceLinesCreate is the `admin billing price-lines create`
// operation.
func adminBillingPriceLinesCreate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_create",
		Title:       "Create a price line",
		Summary:     "Create a billing price line",
		Description: "Create a billing price line. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: []opmesh.OperationArg{
			{Name: "name", Type: opmesh.ArgTypeString, Required: true, Help: "Price line name"},
			{Name: "description", Type: opmesh.ArgTypeString, Help: "Price line description"},
			{Name: "is-active", Type: opmesh.ArgTypeBool, Help: "Mark active"},
			{Name: "is-default", Type: opmesh.ArgTypeBool, Help: "Mark default"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			req := &admin.PriceLineCreateRequest{
				Name:        opmesh.StrArg(input, "name", ""),
				Description: opmesh.StrArg(input, "description", ""),
				IsActive:    opmesh.BoolArg(input, "is-active", false),
				IsDefault:   opmesh.BoolArg(input, "is-default", false),
			}
			return svc.CreatePriceLine(ctx, req)
		}),
	})
}

// adminBillingPriceLinesUpdate is the `admin billing price-lines update`
// operation.
func adminBillingPriceLinesUpdate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_update",
		Title:       "Update a price line",
		Summary:     "Update a billing price line",
		Description: "Update a billing price line by ID. Only the fields provided are changed; others keep their current values. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<price-line-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Price line ID"},
			{Name: "name", Type: opmesh.ArgTypeString, Help: "Price line name"},
			{Name: "description", Type: opmesh.ArgTypeString, Help: "Price line description"},
			{Name: "is-active", Type: opmesh.ArgTypeNullableBool, Help: "Mark active (omit to leave unchanged)"},
			{Name: "is-default", Type: opmesh.ArgTypeNullableBool, Help: "Mark default (omit to leave unchanged)"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_price_lines_update: price line ID is required")
			}
			// Partial update: the SDK update request carries plain bools with
			// no nullable semantics, so flags the caller omitted must be merged
			// from the existing record instead of silently clobbering them to
			// false (mirrors adminQuotaPlansUpdate).
			existing, err := svc.GetPriceLine(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get existing price line: %w", err)
			}
			req := &admin.PriceLineUpdateRequest{
				Name:        existing.Name,
				Description: existing.Description,
				IsActive:    existing.IsActive,
				IsDefault:   existing.IsDefault,
			}
			if n := opmesh.StrArg(input, "name", ""); n != "" {
				req.Name = n
			}
			if desc := opmesh.StrArg(input, "description", ""); desc != "" {
				req.Description = desc
			}
			if v := opmesh.BoolArgPtr(input, "is-active"); v != nil {
				req.IsActive = *v
			}
			if v := opmesh.BoolArgPtr(input, "is-default"); v != nil {
				req.IsDefault = *v
			}
			return svc.UpdatePriceLine(ctx, id, req)
		}),
	})
}

// adminBillingPriceLinesDelete is the `admin billing price-lines delete`
// operation.
func adminBillingPriceLinesDelete(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_delete",
		Title:       "Delete a price line",
		Summary:     "Delete a billing price line",
		Description: "Delete a billing price line by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<price-line-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Price line ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_price_lines_delete: price line ID is required")
			}
			if err := svc.DeletePriceLine(ctx, id); err != nil {
				return nil, err
			}
			return &BillingGenericActionResult{Success: true, Message: "price line deleted"}, nil
		}),
	})
}

// adminBillingPriceLinesAddPlan is the `admin billing price-lines add-plan`
// operation.
func adminBillingPriceLinesAddPlan(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_add_plan",
		Title:       "Add a plan to a price line",
		Summary:     "Add a pricing plan to a price line",
		Description: "Add a pricing plan to a price line at a position. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<price-line-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Price line ID"},
			{Name: "plan-id", Type: opmesh.ArgTypeInt, Required: true, Help: "Pricing plan ID"},
			{Name: "position", Type: opmesh.ArgTypeInt, Help: "Position"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_price_lines_add_plan: price line ID is required")
			}
			req := &admin.AddPlanToPriceLineRequest{PlanId: opmesh.IntArg(input, "plan-id", 0), Position: opmesh.IntArg(input, "position", 0)}
			return svc.AddPlanToPriceLine(ctx, id, req)
		}),
	})
}

// adminBillingPriceLinesDeletePlan is the `admin billing price-lines
// delete-plan` operation.
func adminBillingPriceLinesDeletePlan(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_delete_plan",
		Title:       "Remove a plan from a price line",
		Summary:     "Remove a pricing plan from a price line",
		Description: "Remove a pricing plan from a price line. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<price-line-id> <plan-id>",
		Args: []opmesh.OperationArg{
			{Name: "price-line-id", Type: opmesh.ArgTypeString, Required: true, Help: "Price line ID"},
			{Name: "plan-id", Type: opmesh.ArgTypeString, Required: true, Help: "Pricing plan ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			lineID := opmesh.StrArg(input, "price-line-id", "")
			planID := opmesh.StrArg(input, "plan-id", "")
			if lineID == "" || planID == "" {
				return nil, fmt.Errorf("admin_billing_price_lines_delete_plan: price-line-id and plan-id are required")
			}
			if err := svc.DeletePlanFromPriceLine(ctx, lineID, planID); err != nil {
				return nil, err
			}
			return &BillingGenericActionResult{Success: true, Message: "plan removed from price line"}, nil
		}),
	})
}

// adminBillingPriceLinesUpdatePlanPosition is the `admin billing price-lines
// update-plan-position` operation.
func adminBillingPriceLinesUpdatePlanPosition(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_price_lines_update_plan_position",
		Title:       "Update a plan's position in a price line",
		Summary:     "Update a plan's position in a price line",
		Description: "Update the position of a pricing plan within a price line. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<price-line-id> <plan-id>",
		Args: []opmesh.OperationArg{
			{Name: "price-line-id", Type: opmesh.ArgTypeString, Required: true, Help: "Price line ID"},
			{Name: "plan-id", Type: opmesh.ArgTypeString, Required: true, Help: "Pricing plan ID"},
			{Name: "position", Type: opmesh.ArgTypeInt, Required: true, Help: "New position"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			lineID := opmesh.StrArg(input, "price-line-id", "")
			planID := opmesh.StrArg(input, "plan-id", "")
			if lineID == "" || planID == "" {
				return nil, fmt.Errorf("admin_billing_price_lines_update_plan_position: price-line-id and plan-id are required")
			}
			req := &admin.UpdatePlanPositionRequest{Position: opmesh.IntArg(input, "position", 0)}
			return svc.UpdatePlanPosition(ctx, lineID, planID, req)
		}),
	})
}

// adminBillingPricingPlansList is the `admin billing pricing-plans list`
// operation.
func adminBillingPricingPlansList(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_list",
		Title:       "List pricing plans",
		Summary:     "List billing pricing plans",
		Description: "List all billing pricing plans. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args:        opmesh.ListArgs(),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			plans, _, err := svc.ListPricingPlans(ctx)
			if err != nil {
				return nil, err
			}
			page := opmesh.ParseList(input)
			return billingPricingPlansListResult(slicePage(plans, page.Start, page.Limit)), nil
		}),
	})
}

// adminBillingPricingPlansGet is the `admin billing pricing-plans get`
// operation.
func adminBillingPricingPlansGet(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_get",
		Title:       "Get a pricing plan",
		Summary:     "Get a billing pricing plan",
		Description: "Get a single billing pricing plan by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<plan-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Pricing plan ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plans_get: plan ID is required")
			}
			return svc.GetPricingPlan(ctx, id)
		}),
	})
}

// adminBillingPricingPlansCreate is the `admin billing pricing-plans create`
// operation.
func adminBillingPricingPlansCreate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_create",
		Title:       "Create a pricing plan",
		Summary:     "Create a billing pricing plan",
		Description: "Create a billing pricing plan. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: []opmesh.OperationArg{
			{Name: "name", Type: opmesh.ArgTypeString, Required: true, Help: "Plan name"},
			{Name: "description", Type: opmesh.ArgTypeString, Help: "Plan description"},
			{Name: "currency", Type: opmesh.ArgTypeString, Help: "Currency"},
			{Name: "is-active", Type: opmesh.ArgTypeBool, Help: "Mark active"},
			{Name: "is-public", Type: opmesh.ArgTypeBool, Help: "Mark public"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			req := &admin.PricingPlanCreateRequest{
				Name:        opmesh.StrArg(input, "name", ""),
				Description: opmesh.StrArg(input, "description", ""),
				Currency:    opmesh.StrArg(input, "currency", ""),
				IsActive:    opmesh.BoolArg(input, "is-active", false),
				IsPublic:    opmesh.BoolArg(input, "is-public", false),
			}
			return svc.CreatePricingPlan(ctx, req)
		}),
	})
}

// adminBillingPricingPlansUpdate is the `admin billing pricing-plans update`
// operation.
func adminBillingPricingPlansUpdate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_update",
		Title:       "Update a pricing plan",
		Summary:     "Update a billing pricing plan",
		Description: "Update a billing pricing plan by ID. Only the fields provided are changed; others keep their current values. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<plan-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Pricing plan ID"},
			{Name: "name", Type: opmesh.ArgTypeString, Help: "Plan name"},
			{Name: "description", Type: opmesh.ArgTypeString, Help: "Plan description"},
			{Name: "currency", Type: opmesh.ArgTypeString, Help: "Currency"},
			{Name: "is-active", Type: opmesh.ArgTypeNullableBool, Help: "Mark active (omit to leave unchanged)"},
			{Name: "is-public", Type: opmesh.ArgTypeNullableBool, Help: "Mark public (omit to leave unchanged)"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plans_update: plan ID is required")
			}
			// Partial update: the SDK update request carries plain bools with
			// no nullable semantics, so flags the caller omitted must be merged
			// from the existing record instead of silently clobbering them to
			// false (mirrors adminQuotaPlansUpdate).
			existing, err := svc.GetPricingPlan(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get existing pricing plan: %w", err)
			}
			req := &admin.PricingPlanUpdateRequest{
				Name:        existing.Name,
				Description: existing.Description,
				Currency:    existing.Currency,
				IsActive:    existing.IsActive,
				IsPublic:    existing.IsPublic,
			}
			if n := opmesh.StrArg(input, "name", ""); n != "" {
				req.Name = n
			}
			if desc := opmesh.StrArg(input, "description", ""); desc != "" {
				req.Description = desc
			}
			if cur := opmesh.StrArg(input, "currency", ""); cur != "" {
				req.Currency = cur
			}
			if v := opmesh.BoolArgPtr(input, "is-active"); v != nil {
				req.IsActive = *v
			}
			if v := opmesh.BoolArgPtr(input, "is-public"); v != nil {
				req.IsPublic = *v
			}
			return svc.UpdatePricingPlan(ctx, id, req)
		}),
	})
}

// adminBillingPricingPlansDelete is the `admin billing pricing-plans delete`
// operation.
func adminBillingPricingPlansDelete(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_delete",
		Title:       "Delete a pricing plan",
		Summary:     "Delete a billing pricing plan",
		Description: "Delete a billing pricing plan by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<plan-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Pricing plan ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plans_delete: plan ID is required")
			}
			if err := svc.DeletePricingPlan(ctx, id); err != nil {
				return nil, err
			}
			return &BillingGenericActionResult{Success: true, Message: "pricing plan deleted"}, nil
		}),
	})
}

// adminBillingPricingPlansSync is the `admin billing pricing-plans sync`
// operation.
func adminBillingPricingPlansSync(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_sync",
		Title:       "Sync a pricing plan",
		Summary:     "Sync a pricing plan with its gateway",
		Description: "Sync a billing pricing plan with its payment gateway. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<plan-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Pricing plan ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plans_sync: plan ID is required")
			}
			if err := svc.SyncPricingPlan(ctx, id); err != nil {
				return nil, err
			}
			return &BillingSyncResult{Synced: true, PlanID: id}, nil
		}),
	})
}

// adminBillingPricingPlansSyncAll is the `admin billing pricing-plans
// sync-all` operation.
func adminBillingPricingPlansSyncAll(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plans_sync_all",
		Title:       "Sync all pricing plans",
		Summary:     "Sync all pricing plans with gateways",
		Description: "Sync all billing pricing plans with their payment gateways. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			if err := svc.SyncAllPricingPlans(ctx); err != nil {
				return nil, err
			}
			return &BillingSyncResult{Synced: true}, nil
		}),
	})
}

// adminBillingPricingPlanPeriodsList is the
// `admin billing pricing-plan-periods list` operation.
func adminBillingPricingPlanPeriodsList(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plan_periods_list",
		Title:       "List pricing plan periods",
		Summary:     "List billing pricing plan periods",
		Description: "List all billing pricing plan periods. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args:        opmesh.ListArgs(),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			periods, _, err := svc.ListPricingPlanPeriods(ctx)
			if err != nil {
				return nil, err
			}
			page := opmesh.ParseList(input)
			return billingPricingPlanPeriodsListResult(slicePage(periods, page.Start, page.Limit)), nil
		}),
	})
}

// adminBillingPricingPlanPeriodsGet is the
// `admin billing pricing-plan-periods get` operation.
func adminBillingPricingPlanPeriodsGet(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plan_periods_get",
		Title:       "Get a pricing plan period",
		Summary:     "Get a billing pricing plan period",
		Description: "Get a single billing pricing plan period by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<period-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Period ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plan_periods_get: period ID is required")
			}
			return svc.GetPricingPlanPeriod(ctx, id)
		}),
	})
}

// adminBillingPricingPlanPeriodsCreate is the
// `admin billing pricing-plan-periods create` operation.
func adminBillingPricingPlanPeriodsCreate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plan_periods_create",
		Title:       "Create a pricing plan period",
		Summary:     "Create a billing pricing plan period",
		Description: "Create a billing pricing plan period. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: []opmesh.OperationArg{
			{Name: "pricing-plan-id", Type: opmesh.ArgTypeInt, Required: true, Help: "Pricing plan ID"},
			{Name: "quota-plan-id", Type: opmesh.ArgTypeInt, Required: true, Help: "Quota plan ID"},
			{Name: "cadence", Type: opmesh.ArgTypeString, Required: true, Help: "Cadence (e.g. monthly, yearly)"},
			{Name: "price-usd", Type: opmesh.ArgTypeFloat, Required: true, Help: "Price in USD"},
			{Name: "allow-free", Type: opmesh.ArgTypeBool, Help: "Allow free"},
			{Name: "rolling-days", Type: opmesh.ArgTypeInt, Help: "Rolling days"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			req := &admin.PricingPlanPeriodCreateRequest{
				PricingPlanId: opmesh.IntArg(input, "pricing-plan-id", 0),
				QuotaPlanId:   opmesh.IntArg(input, "quota-plan-id", 0),
				Cadence:       opmesh.StrArg(input, "cadence", ""),
				PriceUsd:      float32(catalogFloatArg(input, "price-usd", 0)),
			}
			if v := opmesh.BoolArgPtr(input, "allow-free"); v != nil {
				req.AllowFree = v
			}
			if v := opmesh.IntArgPtr(input, "rolling-days"); v != nil {
				req.RollingDays = v
			}
			return svc.CreatePricingPlanPeriod(ctx, req)
		}),
	})
}

// adminBillingPricingPlanPeriodsUpdate is the
// `admin billing pricing-plan-periods update` operation.
func adminBillingPricingPlanPeriodsUpdate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plan_periods_update",
		Title:       "Update a pricing plan period",
		Summary:     "Update a billing pricing plan period",
		Description: "Update a billing pricing plan period by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<period-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Period ID"},
			{Name: "cadence", Type: opmesh.ArgTypeString, Help: "Cadence"},
			{Name: "price-usd", Type: opmesh.ArgTypeFloat, Help: "Price in USD"},
			{Name: "allow-free", Type: opmesh.ArgTypeBool, Help: "Allow free"},
			{Name: "rolling-days", Type: opmesh.ArgTypeInt, Help: "Rolling days"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plan_periods_update: period ID is required")
			}
			// The SDK period-update request uses value fields for cadence/price,
			// so a partial update silently resets them. Merge from the existing
			// period instead, only overriding fields the caller actually sets.
			existing, err := svc.GetPricingPlanPeriod(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get existing pricing plan period: %w", err)
			}
			req := &admin.PricingPlanPeriodUpdateRequest{
				Cadence:     existing.Cadence,
				PriceUsd:    existing.PriceUsd,
				QuotaPlanId: existing.QuotaPlanId,
			}
			if c := opmesh.StrArg(input, "cadence", ""); c != "" {
				req.Cadence = c
			}
			if f := catalogFloatArg(input, "price-usd", 0); f != 0 {
				req.PriceUsd = float32(f)
			}
			if v := opmesh.BoolArgPtr(input, "allow-free"); v != nil {
				req.AllowFree = v
			}
			if v := opmesh.IntArgPtr(input, "rolling-days"); v != nil {
				req.RollingDays = v
			}
			return svc.UpdatePricingPlanPeriod(ctx, id, req)
		}),
	})
}

// adminBillingPricingPlanPeriodsDelete is the
// `admin billing pricing-plan-periods delete` operation.
func adminBillingPricingPlanPeriodsDelete(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_pricing_plan_periods_delete",
		Title:       "Delete a pricing plan period",
		Summary:     "Delete a billing pricing plan period",
		Description: "Delete a billing pricing plan period by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<period-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Period ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_pricing_plan_periods_delete: period ID is required")
			}
			if err := svc.DeletePricingPlanPeriod(ctx, id); err != nil {
				return nil, err
			}
			return &BillingGenericActionResult{Success: true, Message: "period deleted"}, nil
		}),
	})
}

// adminBillingSubscribersList is the `admin billing subscribers list`
// operation.
func adminBillingSubscribersList(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_list",
		Title:       "List subscribers",
		Summary:     "List billing subscribers",
		Description: "List all billing subscribers across gateways. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args:        opmesh.ListArgs(),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			subs, _, err := svc.ListSubscribers(ctx)
			if err != nil {
				return nil, err
			}
			page := opmesh.ParseList(input)
			return billingSubscribersListResult(slicePage(subs, page.Start, page.Limit)), nil
		}),
	})
}

// adminBillingSubscribersGet is the `admin billing subscribers get` operation.
func adminBillingSubscribersGet(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_get",
		Title:       "Get a subscriber",
		Summary:     "Get a billing subscriber",
		Description: "Get a single billing subscriber by ID. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<subscriber-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Subscriber ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_get: subscriber ID is required")
			}
			return svc.GetSubscriber(ctx, id)
		}),
	})
}

// adminBillingSubscribersListGateway is the
// `admin billing subscribers list-gateway` operation.
func adminBillingSubscribersListGateway(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_list_gateway",
		Title:       "List subscribers for a gateway",
		Summary:     "List billing subscribers for a gateway",
		Description: "List billing subscribers for a specific gateway. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<gateway-id>",
		Args: []opmesh.OperationArg{
			{Name: "gateway-id", Type: opmesh.ArgTypeString, Required: true, Help: "Gateway ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			gid := opmesh.StrArg(input, "gateway-id", "")
			if gid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_list_gateway: gateway-id is required")
			}
			subs, total, err := svc.ListGatewaySubscribers(ctx, gid)
			if err != nil {
				return nil, err
			}
			return &BillingSubscribersListResult{Count: total, Subscribers: subs}, nil
		}),
	})
}

// adminBillingSubscribersListUser is the `admin billing subscribers list-user`
// operation.
func adminBillingSubscribersListUser(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_list_user",
		Title:       "List subscribers for a user",
		Summary:     "List billing subscribers for a user",
		Description: "List billing subscribers for a specific user. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_list_user: user-id is required")
			}
			subs, total, err := svc.GetUserSubscribers(ctx, uid)
			if err != nil {
				return nil, err
			}
			return &BillingSubscribersListResult{Count: total, Subscribers: subs}, nil
		}),
	})
}

// adminBillingSubscribersCancel is the `admin billing subscribers cancel`
// operation.
func adminBillingSubscribersCancel(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_cancel",
		Title:       "Cancel a user's subscription",
		Summary:     "Cancel a user's subscription",
		Description: "Cancel a user's billing subscription. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
			{Name: "mode", Type: opmesh.ArgTypeString, Help: "Cancellation mode"},
			{Name: "immediate", Type: opmesh.ArgTypeBool, Help: "Cancel immediately"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_cancel: user-id is required")
			}
			req := &admin.CancelSubscriptionRequest{}
			if v := opmesh.StrArg(input, "mode", ""); v != "" {
				req.Mode = &v
			}
			if v := opmesh.BoolArgPtr(input, "immediate"); v != nil {
				req.Immediate = v
			}
			return svc.CancelUserSubscription(ctx, uid, req)
		}),
	})
}

// adminBillingSubscribersAbortCancel is the
// `admin billing subscribers abort-cancel` operation.
func adminBillingSubscribersAbortCancel(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_abort_cancel",
		Title:       "Abort a subscription cancellation",
		Summary:     "Abort a scheduled subscription cancellation",
		Description: "Abort a scheduled cancellation for a user's subscription. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_abort_cancel: user-id is required")
			}
			return svc.AbortUserSubscriptionCancellation(ctx, uid)
		}),
	})
}

// adminBillingSubscribersChangePlan is the
// `admin billing subscribers change-plan` operation.
func adminBillingSubscribersChangePlan(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_change_plan",
		Title:       "Change a user's plan",
		Summary:     "Change a user's subscription plan",
		Description: "Change a user's billing subscription plan. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
			{Name: "period-id", Type: opmesh.ArgTypeInt, Required: true, Help: "Pricing plan period ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_change_plan: user-id is required")
			}
			req := &admin.ChangePlanRequest{PeriodId: opmesh.IntArg(input, "period-id", 0)}
			return svc.ChangeUserPlan(ctx, uid, req)
		}),
	})
}

// adminBillingSubscribersPause is the `admin billing subscribers pause`
// operation.
func adminBillingSubscribersPause(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_pause",
		Title:       "Pause a user's subscription",
		Summary:     "Pause a user's subscription",
		Description: "Pause a user's billing subscription. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_pause: user-id is required")
			}
			return svc.PauseUserSubscription(ctx, uid)
		}),
	})
}

// adminBillingSubscribersResume is the `admin billing subscribers resume`
// operation.
func adminBillingSubscribersResume(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_subscribers_resume",
		Title:       "Resume a user's subscription",
		Summary:     "Resume a paused subscription",
		Description: "Resume a paused billing subscription. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "user-id", Type: opmesh.ArgTypeString, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			uid := opmesh.StrArg(input, "user-id", "")
			if uid == "" {
				return nil, fmt.Errorf("admin_billing_subscribers_resume: user-id is required")
			}
			return svc.ResumeUserSubscription(ctx, uid)
		}),
	})
}

// BillingOverviewResult reports billing entity counts for the overview.
type BillingOverviewResult struct {
	QuotaPlans   int `json:"quota_plans"`
	PriceLines   int `json:"price_lines"`
	PricingPlans int `json:"pricing_plans"`
	Periods      int `json:"periods"`
}

// adminBillingOverview is the `admin billing overview` operation. It aggregates
// entity counts across the quota and billing services.
func adminBillingOverview(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_billing_overview",
		Title:       "Billing overview",
		Summary:     "Show billing entity overview",
		Description: "Show an overview of billing entities and their relationship counts. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			billingSvc, err := d.billing()
			if err != nil {
				return nil, err
			}
			if err := billingSvc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			quotaSvc, err := d.quota()
			if err != nil {
				return nil, err
			}
			out := &BillingOverviewResult{}
			if plans, total, err := quotaSvc.ListPlans(ctx); err == nil {
				_ = plans
				out.QuotaPlans = total
			}
			if lines, total, err := billingSvc.ListPriceLines(ctx); err == nil {
				_ = lines
				out.PriceLines = total
			}
			if plans, total, err := billingSvc.ListPricingPlans(ctx); err == nil {
				_ = plans
				out.PricingPlans = total
			}
			if periods, total, err := billingSvc.ListPricingPlanPeriods(ctx); err == nil {
				_ = periods
				out.Periods = total
			}
			return out, nil
		}),
	})
}
