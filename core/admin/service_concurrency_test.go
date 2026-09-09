package admin

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkadmin "go.lumeweb.com/portal-sdk/admin"

	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
)

// hammerGetService registers a counting config mock, builds the service under
// test via the given factory (which returns its getService closure), then
// hammers getService from several goroutines (started on a common barrier).
// It asserts that lazy init runs exactly once (observed via Config() calls
// made after construction) and that every caller receives the same instance;
// a follow-up sequential call reuses the cached instance without re-init.
func hammerGetService[S comparable](t *testing.T, build func(cfgMgr config.Manager) func(ctx context.Context) (S, error)) {
	t.Helper()

	const goroutines = 16

	cfgMgr := configmocks.NewMockManager(t)
	var configCalls atomic.Int64
	cfgMgr.EXPECT().Config().RunAndReturn(func() *config.Config {
		configCalls.Add(1)
		return &config.Config{
			AuthToken:    adminTestAuthToken,
			BaseEndpoint: "https://api.test.com",
			Secure:       true,
		}
	}).Maybe()

	getService := build(cfgMgr)

	// Construction may have consumed Config() calls; only count from now on.
	baseCalls := configCalls.Load()

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]S, goroutines)
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			s, err := getService(context.Background())
			results[i] = s
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	// Exactly ONE init (token exchange + client creation), not one per goroutine.
	assert.EqualValues(t, 1, configCalls.Load()-baseCalls,
		"expected a single lazy init under the write lock")

	// Idempotence: every goroutine received the same instance.
	for i, s := range results {
		assert.Equalf(t, results[0], s, "goroutine %d got a different instance", i)
	}
	require.NotZero(t, results[0])

	// A sequential re-call must reuse the cached instance without re-init.
	callsBefore := configCalls.Load()
	again, err := getService(context.Background())
	require.NoError(t, err)
	assert.Equal(t, results[0], again)
	assert.Equal(t, callsBefore, configCalls.Load())
}

// Regression tests for the getService double-checked lock: concurrent
// cold-start callers must initialize the underlying admin service exactly
// once and all receive the same instance (no redundant token exchange /
// client creation). Applied to every getService variant.
func TestQuotaAdminService_getService_ConcurrentInit(t *testing.T) {
	hammerGetService(t, func(cfgMgr config.Manager) func(ctx context.Context) (*sdkadmin.QuotaService, error) {
		svc := NewQuotaAdminService(cfgMgr, "https://api.test.com").(*quotaAdminService)
		return svc.getService
	})
}

func TestBillingAdminService_getService_ConcurrentInit(t *testing.T) {
	hammerGetService(t, func(cfgMgr config.Manager) func(ctx context.Context) (*sdkadmin.BillingService, error) {
		svc := NewBillingAdminService(cfgMgr, "https://api.test.com").(*billingAdminService)
		return svc.getService
	})
}

func TestWebsiteAdminService_getService_ConcurrentInit(t *testing.T) {
	hammerGetService(t, func(cfgMgr config.Manager) func(ctx context.Context) (*sdkadmin.WebsiteService, error) {
		svc := NewWebsiteAdminService(cfgMgr, "https://api.test.com").(*websiteAdminService)
		return svc.getService
	})
}

func TestProfilingAdminService_getService_ConcurrentInit(t *testing.T) {
	hammerGetService(t, func(cfgMgr config.Manager) func(ctx context.Context) (*sdkadmin.ProfilingService, error) {
		svc := NewProfilingAdminService(cfgMgr, "https://api.test.com").(*profilingAdminService)
		return svc.getService
	})
}

func TestPlatformDomainAdminService_getService_ConcurrentInit(t *testing.T) {
	hammerGetService(t, func(cfgMgr config.Manager) func(ctx context.Context) (*sdkadmin.PlatformDomainService, error) {
		svc := NewPlatformDomainAdminService(cfgMgr, "https://api.test.com").(*platformDomainAdminService)
		return svc.getService
	})
}

func TestSocialProviderAdminService_getService_ConcurrentInit(t *testing.T) {
	hammerGetService(t, func(cfgMgr config.Manager) func(ctx context.Context) (*sdkadmin.SocialProviderService, error) {
		svc := NewSocialProviderAdminService(cfgMgr, "https://api.test.com").(*socialProviderAdminService)
		return svc.getService
	})
}
