package admin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
	coreerrors "go.lumeweb.com/pinner/core/errors"
	"go.lumeweb.com/portal-sdk/admin"
)

// newUnauthConfigManager returns a mock manager whose live config has no AuthToken.
func newUnauthConfigManager(t *testing.T) config.Manager {
	t.Helper()
	cfgMgr := configmocks.NewMockManager(t)
	cfgMgr.EXPECT().Config().Return(&config.Config{}).Maybe()
	return cfgMgr
}

func newUnauthQuotaAdminService(t *testing.T) *quotaAdminService {
	return &quotaAdminService{
		adminServiceBase: newAdminServiceBase(newUnauthConfigManager(t), ""),
	}
}

func newUnauthBillingAdminService(t *testing.T) *billingAdminService {
	return &billingAdminService{
		adminServiceBase: newAdminServiceBase(newUnauthConfigManager(t), ""),
	}
}

func newUnauthWebsiteAdminService(t *testing.T) *websiteAdminService {
	return &websiteAdminService{
		adminServiceBase: newAdminServiceBase(newUnauthConfigManager(t), ""),
	}
}

func newUnauthProfilingAdminService(t *testing.T) *profilingAdminService {
	return &profilingAdminService{
		adminServiceBase: newAdminServiceBase(newUnauthConfigManager(t), ""),
	}
}

func TestQuotaAdminService_Unauthenticated(t *testing.T) {
	svc := newUnauthQuotaAdminService(t)
	ctx := context.Background()

	_, _, err := svc.ListPlans(ctx)
	require.Error(t, err)
	_, err = svc.CreatePlan(ctx, &admin.QuotaPlan{})
	require.Error(t, err)
	_, err = svc.GetPlan(ctx, "p1")
	require.Error(t, err)
	_, err = svc.UpdatePlan(ctx, "p1", &admin.QuotaPlan{})
	require.Error(t, err)
	err = svc.DeletePlan(ctx, "p1")
	require.Error(t, err)
	err = svc.SetDefaultPlan(ctx, "p1")
	require.Error(t, err)
	_, _, err = svc.ListAllowances(ctx)
	require.Error(t, err)
	_, err = svc.CreateAllowance(ctx, 1, "src", "type", 100, 100, 100, time.Now())
	require.Error(t, err)
	_, err = svc.UpdateAllowance(ctx, "g1", 1, "src", "type", 100, 100, 100, time.Now())
	require.Error(t, err)
	err = svc.DeleteAllowance(ctx, "g1")
	require.Error(t, err)
	_, err = svc.GetStats(ctx)
	require.Error(t, err)
	_, _, err = svc.Reconcile(ctx, nil)
	require.Error(t, err)
	_, err = svc.Cleanup(ctx, 30)
	require.Error(t, err)
	_, err = svc.UpdateUserConfig(ctx, 1, &admin.UserQuotaConfigUpdate{})
	require.Error(t, err)
	_, _, err = svc.ListUserConfigs(ctx)
	require.Error(t, err)
	err = svc.ResetUserPlan(ctx, 1)
	require.Error(t, err)
}

func TestBillingAdminService_Unauthenticated(t *testing.T) {
	svc := newUnauthBillingAdminService(t)
	ctx := context.Background()

	_, _, err := svc.ListCredits(ctx, nil)
	require.Error(t, err)
	_, err = svc.CreateCredit(ctx, &admin.CreditCreateRequest{})
	require.Error(t, err)
	_, err = svc.GetCredit(ctx, "c1")
	require.Error(t, err)
	err = svc.DeleteCredit(ctx, "c1")
	require.Error(t, err)
	_, err = svc.RestoreCredit(ctx, "c1")
	require.Error(t, err)
	_, err = svc.PurgeCredits(ctx, &admin.CreditPurgeRequest{})
	require.Error(t, err)
	_, err = svc.GetUserBalance(ctx, "1")
	require.Error(t, err)
	_, _, err = svc.GetUserDeletedCredits(ctx, "1", nil)
	require.Error(t, err)
	_, _, err = svc.ListPriceLines(ctx)
	require.Error(t, err)
	_, err = svc.CreatePriceLine(ctx, &admin.PriceLineCreateRequest{})
	require.Error(t, err)
	_, err = svc.GetPriceLine(ctx, "pl1")
	require.Error(t, err)
	_, err = svc.UpdatePriceLine(ctx, "pl1", &admin.PriceLineUpdateRequest{})
	require.Error(t, err)
	err = svc.DeletePriceLine(ctx, "pl1")
	require.Error(t, err)
	_, _, err = svc.ListPricingPlans(ctx)
	require.Error(t, err)
	_, err = svc.GetPricingPlan(ctx, "pp1")
	require.Error(t, err)
	_, err = svc.CreatePricingPlan(ctx, &admin.PricingPlanCreateRequest{})
	require.Error(t, err)
	_, err = svc.UpdatePricingPlan(ctx, "pp1", &admin.PricingPlanUpdateRequest{})
	require.Error(t, err)
	err = svc.DeletePricingPlan(ctx, "pp1")
	require.Error(t, err)
	_, _, err = svc.ListPricingPlanPeriods(ctx)
	require.Error(t, err)
	_, err = svc.CreatePricingPlanPeriod(ctx, &admin.PricingPlanPeriodCreateRequest{})
	require.Error(t, err)
	_, err = svc.GetPricingPlanPeriod(ctx, "per1")
	require.Error(t, err)
	_, err = svc.UpdatePricingPlanPeriod(ctx, "per1", &admin.PricingPlanPeriodUpdateRequest{})
	require.Error(t, err)
	err = svc.DeletePricingPlanPeriod(ctx, "per1")
	require.Error(t, err)
	_, _, err = svc.ListSubscribers(ctx)
	require.Error(t, err)
	_, err = svc.GetSubscriber(ctx, "sub1")
	require.Error(t, err)
	_, _, err = svc.ListGatewaySubscribers(ctx, "gw1")
	require.Error(t, err)
	_, _, err = svc.GetUserSubscribers(ctx, "1")
	require.Error(t, err)
	_, err = svc.CancelUserSubscription(ctx, "1", &admin.CancelSubscriptionRequest{})
	require.Error(t, err)
	_, err = svc.AbortUserSubscriptionCancellation(ctx, "1")
	require.Error(t, err)
	_, err = svc.ChangeUserPlan(ctx, "1", &admin.ChangePlanRequest{})
	require.Error(t, err)
}

func TestWebsiteAdminService_Unauthenticated(t *testing.T) {
	svc := newUnauthWebsiteAdminService(t)
	ctx := context.Background()
	_, err := svc.BlockWebsite(ctx, "example.com")
	require.Error(t, err)
	_, err = svc.UnblockWebsite(ctx, "example.com")
	require.Error(t, err)
}

func TestProfilingAdminService_Unauthenticated(t *testing.T) {
	svc := newUnauthProfilingAdminService(t)
	ctx := context.Background()
	_, err := svc.GetProfileIndex(ctx)
	require.Error(t, err)
	_, err = svc.GetBlockProfile(ctx)
	require.Error(t, err)
	err = svc.SetBlockProfileRate(ctx, 1)
	require.Error(t, err)
	_, err = svc.GetCmdline(ctx)
	require.Error(t, err)
	_, err = svc.GetGoroutineProfile(ctx)
	require.Error(t, err)
	_, err = svc.GetHeapProfile(ctx)
	require.Error(t, err)
	_, err = svc.GetMutexProfile(ctx)
	require.Error(t, err)
	err = svc.SetMutexProfileFraction(ctx, 1)
	require.Error(t, err)
	_, err = svc.GetCPUProfile(ctx)
	require.Error(t, err)
	_, err = svc.GetStatus(ctx)
	require.Error(t, err)
	_, err = svc.GetSymbol(ctx)
	require.Error(t, err)
	_, err = svc.GetThreadcreate(ctx)
	require.Error(t, err)
	_, err = svc.GetTrace(ctx)
	require.Error(t, err)
}

func TestAdminServiceBase_RequireAuthenticated(t *testing.T) {
	t.Run("not authenticated", func(t *testing.T) {
		base := newAdminServiceBase(newUnauthConfigManager(t), "")
		err := base.RequireAuthenticated()
		require.Error(t, err)
		assert.Equal(t, coreerrors.ErrNotAuthenticated, err)
	})
	t.Run("authenticated", func(t *testing.T) {
		cfgMgr := configmocks.NewMockManager(t)
		cfgMgr.EXPECT().Config().Return(&config.Config{AuthToken: adminTestAuthToken}).Maybe()
		base := newAdminServiceBase(cfgMgr, "")
		err := base.RequireAuthenticated()
		require.NoError(t, err)
	})
}
