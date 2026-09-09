package catalogops

import (
	"context"
	"strings"
	"testing"

	coreadmin "go.lumeweb.com/pinner/core/admin"
	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"

	"go.lumeweb.com/portal-sdk/admin"
)

// fakeBillingService is a minimal admin.BillingAdminService fake: it embeds
// the interface for the untouched methods and drives just the price-line and
// pricing-plan pieces the update operations exercise with function fields so
// tests can capture the exact update requests.
type fakeBillingService struct {
	coreadmin.BillingAdminService

	requireAuth       func() error
	getPriceLineFn    func(ctx context.Context, id string) (*admin.PriceLineDetailResponse, error)
	updatePriceLineFn func(ctx context.Context, id string, req *admin.PriceLineUpdateRequest) (*admin.PriceLine, error)
	getPricingPlanFn  func(ctx context.Context, id string) (*admin.PricingPlan, error)
	updatePricingPlan func(ctx context.Context, id string, req *admin.PricingPlanUpdateRequest) (*admin.PricingPlan, error)
}

func (f *fakeBillingService) RequireAuthenticated() error {
	if f.requireAuth != nil {
		return f.requireAuth()
	}
	return nil
}

func (f *fakeBillingService) GetPriceLine(ctx context.Context, id string) (*admin.PriceLineDetailResponse, error) {
	if f.getPriceLineFn != nil {
		return f.getPriceLineFn(ctx, id)
	}
	return nil, nil
}

func (f *fakeBillingService) UpdatePriceLine(ctx context.Context, id string, req *admin.PriceLineUpdateRequest) (*admin.PriceLine, error) {
	if f.updatePriceLineFn != nil {
		return f.updatePriceLineFn(ctx, id, req)
	}
	return nil, nil
}

func (f *fakeBillingService) GetPricingPlan(ctx context.Context, id string) (*admin.PricingPlan, error) {
	if f.getPricingPlanFn != nil {
		return f.getPricingPlanFn(ctx, id)
	}
	return nil, nil
}

func (f *fakeBillingService) UpdatePricingPlan(ctx context.Context, id string, req *admin.PricingPlanUpdateRequest) (*admin.PricingPlan, error) {
	if f.updatePricingPlan != nil {
		return f.updatePricingPlan(ctx, id, req)
	}
	return nil, nil
}

func testBillingDeps(t *testing.T, svc *fakeBillingService) AdminDeps {
	return AdminDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		BillingAdminService: func(cfgMgr config.Manager) (coreadmin.BillingAdminService, error) {
			return svc, nil
		},
	}
}

// TestAdminBillingPriceLinesUpdatePartialPreservesFlags pins that a partial
// rename (only name provided, no is-active/is-default) fetches the existing
// record and preserves the current flag values instead of clobbering them
// to false.
func TestAdminBillingPriceLinesUpdatePartialPreservesFlags(t *testing.T) {
	var gotReq *admin.PriceLineUpdateRequest
	svc := &fakeBillingService{
		getPriceLineFn: func(ctx context.Context, id string) (*admin.PriceLineDetailResponse, error) {
			if id != "42" {
				t.Fatalf("GetPriceLine id = %q, want 42", id)
			}
			return &admin.PriceLineDetailResponse{
				Name:        "Old Name",
				Description: "Old description",
				IsActive:    true,
				IsDefault:   true,
			}, nil
		},
		updatePriceLineFn: func(ctx context.Context, id string, req *admin.PriceLineUpdateRequest) (*admin.PriceLine, error) {
			gotReq = req
			return &admin.PriceLine{}, nil
		},
	}
	op := adminBillingPriceLinesUpdate(testBillingDeps(t, svc))

	// Only the name is provided: no is-active/is-default args at all.
	_, err := op.Handler().Execute(context.Background(), map[string]any{
		"id":   "42",
		"name": "New Name",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if gotReq == nil {
		t.Fatal("update request did not reach the service")
	}
	if gotReq.Name != "New Name" {
		t.Fatalf("Name = %q, want New Name", gotReq.Name)
	}
	if gotReq.Description != "Old description" {
		t.Fatalf("Description = %q, want the existing description preserved", gotReq.Description)
	}
	if !gotReq.IsActive {
		t.Fatal("IsActive = false, want the existing true preserved on partial update")
	}
	if !gotReq.IsDefault {
		t.Fatal("IsDefault = false, want the existing true preserved on partial update")
	}
}

// TestAdminBillingPriceLinesUpdateExplicitFlagsOverride pins that explicit
// flag args still override the existing values.
func TestAdminBillingPriceLinesUpdateExplicitFlagsOverride(t *testing.T) {
	var gotReq *admin.PriceLineUpdateRequest
	svc := &fakeBillingService{
		getPriceLineFn: func(ctx context.Context, id string) (*admin.PriceLineDetailResponse, error) {
			return &admin.PriceLineDetailResponse{Name: "Old", IsActive: true, IsDefault: true}, nil
		},
		updatePriceLineFn: func(ctx context.Context, id string, req *admin.PriceLineUpdateRequest) (*admin.PriceLine, error) {
			gotReq = req
			return &admin.PriceLine{}, nil
		},
	}
	op := adminBillingPriceLinesUpdate(testBillingDeps(t, svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{
		"id":   "42",
		"name": "New Name",
		// BoolArgPtr reads bools; false is an explicit override.
		"is-active":  false,
		"is-default": true,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if gotReq == nil {
		t.Fatal("update request did not reach the service")
	}
	if gotReq.IsActive {
		t.Fatal("IsActive = true, want the explicitly provided false")
	}
	if !gotReq.IsDefault {
		t.Fatal("IsDefault = false, want the explicitly provided true")
	}
	if gotReq.Name != "New Name" {
		t.Fatalf("Name = %q, want New Name", gotReq.Name)
	}
}

// TestAdminBillingPriceLinesUpdateGetError asserts a failed existing-record
// fetch fails clearly instead of sending a clobbered request.
func TestAdminBillingPriceLinesUpdateGetError(t *testing.T) {
	svc := &fakeBillingService{
		getPriceLineFn: func(ctx context.Context, id string) (*admin.PriceLineDetailResponse, error) {
			return nil, context.Canceled
		},
	}
	op := adminBillingPriceLinesUpdate(testBillingDeps(t, svc))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": "42", "name": "New Name"})
	if err == nil {
		t.Fatal("expected an error when the existing price line cannot be fetched")
	}
}

// TestAdminBillingPriceLinesUpdateNilExistingNotFound is a regression test:
// the SDK getter can return (nil, nil) for a non-existent record, which used
// to nil-panic on the existing.* merge. The update must fail clearly with a
// not-found error and must never reach the update call.
func TestAdminBillingPriceLinesUpdateNilExistingNotFound(t *testing.T) {
	updated := false
	svc := &fakeBillingService{
		// Default getPriceLineFn returns (nil, nil).
		updatePriceLineFn: func(ctx context.Context, id string, req *admin.PriceLineUpdateRequest) (*admin.PriceLine, error) {
			updated = true
			return &admin.PriceLine{}, nil
		},
	}
	op := adminBillingPriceLinesUpdate(testBillingDeps(t, svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": "42", "name": "New Name"})
	if err == nil {
		t.Fatal("expected a not-found error when the existing price line is nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should mention not-found, got %v", err)
	}
	if updated {
		t.Fatal("update must not be invoked when the existing record is missing")
	}
}

// TestAdminBillingPricingPlansUpdateNilExistingNotFound is the pricing-plan
// counterpart of the (nil, nil) getter regression above.
func TestAdminBillingPricingPlansUpdateNilExistingNotFound(t *testing.T) {
	updated := false
	svc := &fakeBillingService{
		// Default getPricingPlanFn returns (nil, nil).
		updatePricingPlan: func(ctx context.Context, id string, req *admin.PricingPlanUpdateRequest) (*admin.PricingPlan, error) {
			updated = true
			return &admin.PricingPlan{}, nil
		},
	}
	op := adminBillingPricingPlansUpdate(testBillingDeps(t, svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": "7", "name": "New Name"})
	if err == nil {
		t.Fatal("expected a not-found error when the existing pricing plan is nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should mention not-found, got %v", err)
	}
	if updated {
		t.Fatal("update must not be invoked when the existing record is missing")
	}
}

// TestAdminBillingPricingPlansUpdatePartialPreservesFlags pins that a partial
// pricing plan update (only name provided, no is-active/is-public) fetches
// the existing record and preserves the current flag values instead of
// clobbering them to false.
func TestAdminBillingPricingPlansUpdatePartialPreservesFlags(t *testing.T) {
	var gotReq *admin.PricingPlanUpdateRequest
	svc := &fakeBillingService{
		getPricingPlanFn: func(ctx context.Context, id string) (*admin.PricingPlan, error) {
			if id != "7" {
				t.Fatalf("GetPricingPlan id = %q, want 7", id)
			}
			plan := &admin.PricingPlan{}
			plan.Name = "Starter"
			plan.Description = "Old description"
			plan.Currency = "usd"
			plan.IsActive = true
			plan.IsPublic = true
			return plan, nil
		},
		updatePricingPlan: func(ctx context.Context, id string, req *admin.PricingPlanUpdateRequest) (*admin.PricingPlan, error) {
			gotReq = req
			return &admin.PricingPlan{}, nil
		},
	}
	op := adminBillingPricingPlansUpdate(testBillingDeps(t, svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{
		"id":   "7",
		"name": "Starter Plus",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if gotReq == nil {
		t.Fatal("update request did not reach the service")
	}
	if gotReq.Name != "Starter Plus" {
		t.Fatalf("Name = %q, want Starter Plus", gotReq.Name)
	}
	if gotReq.Description != "Old description" {
		t.Fatalf("Description = %q, want the existing description preserved", gotReq.Description)
	}
	if gotReq.Currency != "usd" {
		t.Fatalf("Currency = %q, want the existing usd preserved", gotReq.Currency)
	}
	if !gotReq.IsActive {
		t.Fatal("IsActive = false, want the existing true preserved on partial update")
	}
	if !gotReq.IsPublic {
		t.Fatal("IsPublic = false, want the existing true preserved on partial update")
	}
}

// TestAdminBillingPricingPlansUpdateExplicitFlagsOverride pins that explicit
// flag args still override the existing values.
func TestAdminBillingPricingPlansUpdateExplicitFlagsOverride(t *testing.T) {
	var gotReq *admin.PricingPlanUpdateRequest
	svc := &fakeBillingService{
		getPricingPlanFn: func(ctx context.Context, id string) (*admin.PricingPlan, error) {
			plan := &admin.PricingPlan{}
			plan.Name = "Starter"
			plan.IsActive = true
			plan.IsPublic = true
			return plan, nil
		},
		updatePricingPlan: func(ctx context.Context, id string, req *admin.PricingPlanUpdateRequest) (*admin.PricingPlan, error) {
			gotReq = req
			return &admin.PricingPlan{}, nil
		},
	}
	op := adminBillingPricingPlansUpdate(testBillingDeps(t, svc))

	_, err := op.Handler().Execute(context.Background(), map[string]any{
		"id":        "7",
		"name":      "Starter Plus",
		"is-active": false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if gotReq == nil {
		t.Fatal("update request did not reach the service")
	}
	if gotReq.IsActive {
		t.Fatal("IsActive = true, want the explicitly provided false")
	}
	if !gotReq.IsPublic {
		t.Fatal("IsPublic = false, want the existing true preserved")
	}
}
