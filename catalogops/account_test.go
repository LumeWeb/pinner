package catalogops

import (
	"context"
	"errors"
	"testing"

	"go.lumeweb.com/pinner/catalogmeta"
	"go.lumeweb.com/pinner/core/auth"
	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
	account "go.lumeweb.com/portal-sdk"
)

// fakeAuthService mocks auth.AuthService for the account otp disable handler,
// overriding only the methods each test exercises. It embeds the real
// interface so unimplemented methods panic rather than silently succeed.
type fakeAuthService struct {
	auth.AuthService

	disableOTPFn   func(ctx context.Context, password string) (*auth.DisableOTPResult, error)
	quotaFn        func(ctx context.Context) (*account.QuotaStatus, error)
	subscriptionFn func(ctx context.Context) (*account.SubscriptionStatus, error)
}

func (f *fakeAuthService) GetSubscriptionStatus(ctx context.Context) (*account.SubscriptionStatus, error) {
	if f.subscriptionFn != nil {
		return f.subscriptionFn(ctx)
	}
	return &account.SubscriptionStatus{}, nil
}

func (f *fakeAuthService) DisableOTP(ctx context.Context, password string) (*auth.DisableOTPResult, error) {
	if f.disableOTPFn != nil {
		return f.disableOTPFn(ctx, password)
	}
	return &auth.DisableOTPResult{}, nil
}

func (f *fakeAuthService) GetQuota(ctx context.Context) (*account.QuotaStatus, error) {
	if f.quotaFn != nil {
		return f.quotaFn(ctx)
	}
	return &account.QuotaStatus{}, nil
}

func intPtr(v int) *int { return &v }

// accountDisableDeps returns an AccountDeps whose auth service is backed by the
// given fake, with a config manager present so authClientHandler resolves.
func accountDisableDeps(t *testing.T, fake *fakeAuthService) AccountDeps {
	t.Helper()
	return AccountDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		AuthService: func(cfgMgr config.Manager, token string) auth.AuthService {
			return fake
		},
	}
}

func TestAccountOTPDisableCallsDisableOTP(t *testing.T) {
	var gotPassword string
	fake := &fakeAuthService{
		disableOTPFn: func(_ context.Context, password string) (*auth.DisableOTPResult, error) {
			gotPassword = password
			return &auth.DisableOTPResult{}, nil
		},
	}

	const testPassword = "test-password-not-a-secret"
	op := accountOTPDisable(accountDisableDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"password": testPassword})
	if err != nil {
		t.Fatalf("otp disable handler: %v", err)
	}
	if gotPassword != testPassword {
		t.Fatalf("DisableOTP password = %q, want %q", gotPassword, testPassword)
	}
	result, ok := res.(*AccountOTPDisableResult)
	if !ok {
		t.Fatalf("result type = %T, want *AccountOTPDisableResult", res)
	}
	if result.Message != "Two-factor authentication disabled." {
		t.Fatalf("message = %q, want %q", result.Message, "Two-factor authentication disabled.")
	}
}

func TestAccountOTPDisableRequiresPassword(t *testing.T) {
	fake := &fakeAuthService{
		disableOTPFn: func(_ context.Context, password string) (*auth.DisableOTPResult, error) {
			t.Fatal("DisableOTP must not be called when password is missing")
			return nil, nil
		},
	}

	op := accountOTPDisable(accountDisableDeps(t, fake))
	if _, err := op.Handler().Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected error when password arg is missing")
	}
}

func TestAccountOTPDisablePropagatesServiceError(t *testing.T) {
	fake := &fakeAuthService{
		disableOTPFn: func(_ context.Context, password string) (*auth.DisableOTPResult, error) {
			return nil, errors.New("invalid password")
		},
	}

	op := accountOTPDisable(accountDisableDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"password": "wrong"})
	if err == nil {
		t.Fatal("expected error when DisableOTP fails")
	}
	if err.Error() != "account_otp_disable: invalid password" {
		t.Fatalf("err = %q, want account_otp_disable: invalid password", err.Error())
	}
}

// TestAccountUpdateEnvCLIOnly verifies account_update_email and
// account_update_password declare EnvCLIOnly: they are valid only on the urfave
// CLI frontend and are omitted from every MCP surface (they pass the user's
// password through the LLM channel and duplicate the OOB browser hand-off
// tools). The Environment metadata lives on the frontend-metadata boundary
// (catalogmeta), keyed by the stable operation ID; this test pins the same
// contract there.
func TestAccountUpdateEnvCLIOnly(t *testing.T) {
	for _, name := range []string{"account_update_email", "account_update_password", "account_otp_disable"} {
		if env := catalogmeta.EnvironmentOf(name); env != catalogmeta.EnvCLIOnly {
			t.Errorf("catalogmeta.EnvironmentOf(%q) = %v, want EnvCLIOnly", name, env)
		}
	}
	if env := catalogmeta.EnvironmentOf("account_info"); env != catalogmeta.EnvBoth {
		t.Errorf("catalogmeta.EnvironmentOf(account_info) = %v, want EnvBoth", env)
	}
}

// TestAccountQuotaReturnsTypedResult verifies the account_quota handler maps a
// positive-remaining quota into a typed result with has_quota=true (quota
// covers access regardless of subscription).
func TestAccountQuotaReturnsTypedResult(t *testing.T) {
	fake := &fakeAuthService{
		quotaFn: func(_ context.Context) (*account.QuotaStatus, error) {
			q := &account.QuotaStatus{}
			q.Upload.Used = 5
			q.Upload.Limit = intPtr(100)
			q.Upload.Remaining = intPtr(95)
			q.Upload.Percentage = 5
			q.Download.Remaining = intPtr(0)
			return q, nil
		},
	}
	op := accountQuota(accountDisableDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("account quota handler: %v", err)
	}
	result, ok := res.(*AccountQuotaResult)
	if !ok {
		t.Fatalf("result type = %T, want *AccountQuotaResult", res)
	}
	if !result.HasQuota {
		t.Fatal("expected has_quota=true when upload has remaining allowance")
	}
	if result.Upload.Used != 5 || result.Upload.Remaining == nil || *result.Upload.Remaining != 95 {
		t.Fatalf("upload fields wrong: %+v", result.Upload)
	}
	if result.Message == "" {
		t.Fatal("expected a human-readable message")
	}
}

// TestAccountQuotaNoRemaining verifies has_quota=false when every dimension is
// exhausted, and the message reflects that a subscription/grant is needed.
func TestAccountQuotaNoRemaining(t *testing.T) {
	fake := &fakeAuthService{
		quotaFn: func(_ context.Context) (*account.QuotaStatus, error) {
			q := &account.QuotaStatus{}
			q.Upload.Remaining = intPtr(0)
			q.Download.Remaining = intPtr(0)
			q.Storage.Remaining = intPtr(0)
			return q, nil
		},
	}
	op := accountQuota(accountDisableDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("account quota handler: %v", err)
	}
	result := res.(*AccountQuotaResult)
	if result.HasQuota {
		t.Fatal("expected has_quota=false when all dimensions are exhausted")
	}
}

// TestAccountQuotaNilRemainingIsNotCovered verifies that a dimension with an
// omitted remaining bound (nil) is NOT treated as covered: the schema has no
// explicit unlimited marker and zero-quota accounts report all-nil, so an
// account with no explicit allowance must not surface has_quota=true.
func TestAccountQuotaNilRemainingIsNotCovered(t *testing.T) {
	fake := &fakeAuthService{
		quotaFn: func(_ context.Context) (*account.QuotaStatus, error) {
			return &account.QuotaStatus{}, nil // all remaining == nil (no explicit allowance)
		},
	}
	op := accountQuota(accountDisableDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("account quota handler: %v", err)
	}
	result := res.(*AccountQuotaResult)
	if result.HasQuota {
		t.Fatal("expected has_quota=false when no dimension has an explicit positive remaining allowance")
	}
}

// TestAccountQuotaPropagatesServiceError verifies the handler propagates a
// GetQuota error with the operation prefix.
func TestAccountQuotaPropagatesServiceError(t *testing.T) {
	fake := &fakeAuthService{
		quotaFn: func(_ context.Context) (*account.QuotaStatus, error) {
			return nil, errors.New("quota service down")
		},
	}
	op := accountQuota(accountDisableDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error when GetQuota fails")
	}
	if err.Error() != "account_quota: quota service down" {
		t.Fatalf("err = %q, want account_quota: quota service down", err.Error())
	}
}

// subscriptionDeps returns an AccountDeps wired like accountDisableDeps
// (config manager present so authClientHandler resolves) plus a PortalURL
// stub, with HostedPlugin already switchable by the caller.
func subscriptionDeps(t *testing.T, fake *fakeAuthService) AccountDeps {
	deps := accountDisableDeps(t, fake)
	deps.PortalURL = func(_ config.Manager) string { return "https://account.portal/account/subscription" }
	return deps
}

// TestAccountSubscriptionHostedSuppressesDeepLink pins the plugin-commerce
// policy gate: a hosted assembly's account_subscription result carries no
// web_url and phrases the outcome as a plain entitlement fact with no
// subscribe prompting.
func TestAccountSubscriptionHostedSuppressesDeepLink(t *testing.T) {
	for _, tc := range []struct {
		name        string
		subscribed  bool
		wantMessage string
	}{
		{"subscribed", true, "Subscribed."},
		{"not subscribed", false, "Not subscribed."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuthService{
				subscriptionFn: func(_ context.Context) (*account.SubscriptionStatus, error) {
					st := &account.SubscriptionStatus{}
					st.IsSubscribed = tc.subscribed
					return st, nil
				},
			}
			deps := subscriptionDeps(t, fake)
			deps.HostedPlugin = true
			op := accountSubscription(deps)
			res, err := op.Handler().Execute(context.Background(), map[string]any{})
			if err != nil {
				t.Fatalf("account subscription handler: %v", err)
			}
			got, ok := res.(*AccountSubscriptionResult)
			if !ok {
				t.Fatalf("result type = %T, want *AccountSubscriptionResult", res)
			}
			if got.WebURL != "" {
				t.Errorf("hosted result WebURL = %q, want empty (plugin policy forbids subscription deep-links)", got.WebURL)
			}
			if got.Message != tc.wantMessage {
				t.Errorf("hosted result Message = %q, want %q", got.Message, tc.wantMessage)
			}
		})
	}
}

// TestAccountSubscriptionLocalKeepsDeepLink pins that the non-hosted
// assembly keeps the full deep-link UX: web_url populated from the PortalURL
// dep, messages unchanged.
func TestAccountSubscriptionLocalKeepsDeepLink(t *testing.T) {
	const portalURL = "https://account.portal/account/subscription"
	for _, tc := range []struct {
		name        string
		subscribed  bool
		wantMessage string
	}{
		{"subscribed", true, "Subscribed. Manage your subscription in the web app."},
		{"not subscribed", false, "Not subscribed. Open the web app to choose a plan and subscribe."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuthService{
				subscriptionFn: func(_ context.Context) (*account.SubscriptionStatus, error) {
					st := &account.SubscriptionStatus{}
					st.IsSubscribed = tc.subscribed
					return st, nil
				},
			}
			op := accountSubscription(subscriptionDeps(t, fake))
			res, err := op.Handler().Execute(context.Background(), map[string]any{})
			if err != nil {
				t.Fatalf("account subscription handler: %v", err)
			}
			got, ok := res.(*AccountSubscriptionResult)
			if !ok {
				t.Fatalf("result type = %T, want *AccountSubscriptionResult", res)
			}
			if got.WebURL != portalURL {
				t.Errorf("local result WebURL = %q, want %q", got.WebURL, portalURL)
			}
			if got.Message != tc.wantMessage {
				t.Errorf("local result Message = %q, want %q", got.Message, tc.wantMessage)
			}
		})
	}
}

// TestAccountQuotaHostedSuppressesDeepLink pins the plugin-commerce policy
// gate on account_quota: a hosted assembly's result carries no web_url, and
// its no-quota message is the sanctioned entitlement explanation with no
// subscribe prompting.
func TestAccountQuotaHostedSuppressesDeepLink(t *testing.T) {
	fake := &fakeAuthService{
		quotaFn: func(_ context.Context) (*account.QuotaStatus, error) {
			q := &account.QuotaStatus{}
			q.Upload.Remaining = intPtr(0)
			q.Download.Remaining = intPtr(0)
			q.Storage.Remaining = intPtr(0)
			return q, nil
		},
	}
	deps := subscriptionDeps(t, fake)
	deps.HostedPlugin = true
	op := accountQuota(deps)
	res, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("account quota handler: %v", err)
	}
	got, ok := res.(*AccountQuotaResult)
	if !ok {
		t.Fatalf("result type = %T, want *AccountQuotaResult", res)
	}
	if got.WebURL != "" {
		t.Errorf("hosted result WebURL = %q, want empty (plugin policy forbids subscription deep-links)", got.WebURL)
	}
	if got.Message != "Paid actions are unavailable on this account: no remaining granted usage and no active subscription." {
		t.Errorf("hosted result Message = %q", got.Message)
	}
}
