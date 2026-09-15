package catalogops

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ipfs "go.lumeweb.com/ipfs-sdk"

	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
	"go.lumeweb.com/pinner/core/ipns"
)

// TestIPNSServiceAuthTokenPrecedence verifies that IPNSDeps.service honors the
// per-invocation --auth-token flag override (threaded through the input map)
// ahead of the GetAuthToken config fallback, mirroring PinsDeps.service.
func TestIPNSServiceAuthTokenPrecedence(t *testing.T) {
	var gotToken string
	d := IPNSDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		NewAuthenticated: func(_ config.Manager, token string, _ bool) (ipns.Service, error) {
			gotToken = token
			return nil, nil
		},
		GetAuthToken: func() string { return "config-token" },
	}

	// Flag override wins.
	if _, err := d.service(map[string]any{AuthTokenInputKey: "flag-token"}); err != nil {
		t.Fatalf("service(flag): unexpected error: %v", err)
	}
	if gotToken != "flag-token" {
		t.Fatalf("flag override: got %q, want flag-token", gotToken)
	}

	// Falls back to GetAuthToken config token when no flag override.
	if _, err := d.service(map[string]any{}); err != nil {
		t.Fatalf("service(config): unexpected error: %v", err)
	}
	if gotToken != "config-token" {
		t.Fatalf("config fallback: got %q, want config-token", gotToken)
	}
}

// TestIPNSServiceNilConfigError verifies a nil config manager yields a clean
// error instead of a nil-pointer panic inside NewAuthenticated/ServiceFactory,
// mirroring PinsDeps.service's guard.
func TestIPNSServiceNilConfigError(t *testing.T) {
	d := IPNSDeps{
		// CfgMgr unset -> config() returns nil -> service() must error.
		NewAuthenticated: func(_ config.Manager, _ string, _ bool) (ipns.Service, error) {
			t.Fatal("NewAuthenticated must not be reached with a nil config manager")
			return nil, nil
		},
	}
	if _, err := d.service(map[string]any{}); err == nil {
		t.Fatal("service: expected error for nil config manager, got nil")
	}
}

// fakeIPNSListKeysService fakes ipns.Service for the list handler, returning a
// pre-filtered (server-side name-filtered) set from ListKeys so tests can drive
// the search branch's client-side paging.
type fakeIPNSListKeysService struct {
	ipns.Service
	keys []ipfs.IPNSKeyResponse
}

func (s *fakeIPNSListKeysService) RequireAuthenticated() error { return nil }

func (s *fakeIPNSListKeysService) ListKeys(context.Context, ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
	return s.keys, nil
}

// TestIPNSKeysListSearchPaging guards the regression where the search branch
// of ipns_keys_list dropped the page/page-size cursor and returned every
// filtered key regardless of the requested window. With a search given it must
// page the full filtered set client-side (slicePage) while preserving the full
// filtered total, so page 2+ reports the right rows and an accurate count.
func TestIPNSKeysListSearchPaging(t *testing.T) {
	keys := make([]ipfs.IPNSKeyResponse, 5)
	for i := range keys {
		keys[i] = ipfs.IPNSKeyResponse{
			Id:       i + 1,
			Name:     fmt.Sprintf("key-%d", i+1),
			IpnsName: fmt.Sprintf("k51qzi5uqu5dg%04d", i),
			PeerId:   "12D3KooWFakePeerId",
			Created:  time.Now(),
		}
	}
	svc := &fakeIPNSListKeysService{keys: keys}
	op := ipnsKeysList(IPNSDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		ServiceFactory: func(config.Manager, bool, ...ipns.Option) ipns.Service {
			return svc
		},
	})

	// Page 2 of page-size 2 over 5 filtered keys -> rows key-3,key-4; total 5.
	res, err := op.Handler().Execute(context.Background(), map[string]any{
		"search": "key", "page": 2, "page-size": 2,
	})
	require.NoError(t, err)

	got, ok := res.(ListResult)
	require.True(t, ok, "result type = %T, want ListResult", res)
	require.Equal(t, 2, got.ListCount(), "page 2 of size 2 should yield 2 rows")
	require.Equal(t, 5, got.ListTotal(), "full filtered total must be preserved")

	items, ok := got.ListItems().([]ipfs.IPNSKeyResponse)
	require.True(t, ok, "items type = %T, want []ipfs.IPNSKeyResponse", got.ListItems())
	require.Len(t, items, 2)
	require.Equal(t, "key-3", items[0].Name)
	require.Equal(t, "key-4", items[1].Name)

	// Page 1 of size 10 over 5 keys must not truncate: all 5 rows, total 5.
	res, err = op.Handler().Execute(context.Background(), map[string]any{
		"search": "key", "page": 1, "page-size": 10,
	})
	require.NoError(t, err)
	got, ok = res.(ListResult)
	require.True(t, ok)
	require.Equal(t, 5, got.ListCount())
	require.Equal(t, 5, got.ListTotal())
}
