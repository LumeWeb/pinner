package main

// fakes_race_test.go is a concurrency regression test for FakeServices: in
// --http mode the fake services are invoked from MCP tool handlers that run
// concurrently, so every shared in-memory store (pins, pinOrder, keys, zones,
// websites, apiKeys) must be guarded by FakeServices.mu. Run with
//
//	go test -race ./cmd/mcpharness/
//
// which fails loudly on any unsynchronized map read/write race.

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.lumeweb.com/pinner/core/pinning"
	"go.lumeweb.com/pinner/core/websites"
)

// TestFakeServicesConcurrentAccess hammers the shared fake stores from 8
// goroutines running mixed Pin/List/Unpin, CreateZone/ListZones,
// CreateKey/ListKeys, CreateAPIKey/ListAPIKeys and Create/List rounds.
func TestFakeServicesConcurrentAccess(t *testing.T) {
	f, err := NewFakeServices("race@example.com")
	if err != nil {
		t.Fatalf("NewFakeServices: %v", err)
	}

	pins := newFakePinningService(f)
	dnsSvc := newFakeDNSService(f)
	ipnsSvc := newFakeIPNSService(f)
	webSvc := newFakeWebsitesService(f)
	keySvc := &fakeAPIKeysService{svc: f}

	const (
		workers = 8
		iters   = 200
	)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < iters; i++ {
				cid := fmt.Sprintf("bafybeirace%02d%04d00000000000000000000000000", w, i)
				name := fmt.Sprintf("race-%d-%d", w, i)
				zone := fmt.Sprintf("race-%d-%d.example.com", w, i)
				key := fmt.Sprintf("race-key-%d-%d", w, i)

				// pinning
				if _, err := pins.Pin(ctx, cid, name, false); err != nil {
					t.Errorf("pin (%d,%d): %v", w, i, err)
				}
				if _, err := pins.List(ctx, pinning.ListOptions{}); err != nil {
					t.Errorf("pins list (%d,%d): %v", w, i, err)
				}
				if _, err := pins.Unpin(ctx, cid, false); err != nil {
					t.Errorf("unpin (%d,%d): %v", w, i, err)
				}

				// dns
				if _, err := dnsSvc.CreateZone(ctx, zone, nil); err != nil {
					t.Errorf("create zone (%d,%d): %v", w, i, err)
				}
				if _, err := dnsSvc.ListZones(ctx); err != nil {
					t.Errorf("list zones (%d,%d): %v", w, i, err)
				}

				// ipns
				if _, err := ipnsSvc.CreateKey(ctx, key, nil); err != nil {
					t.Errorf("create key (%d,%d): %v", w, i, err)
				}
				if _, err := ipnsSvc.ListKeys(ctx); err != nil {
					t.Errorf("list keys (%d,%d): %v", w, i, err)
				}

				// api keys
				if _, err := keySvc.CreateAPIKey(ctx, key); err != nil {
					t.Errorf("create api key (%d,%d): %v", w, i, err)
				}
				if _, _, err := keySvc.ListAPIKeys(ctx, ""); err != nil {
					t.Errorf("list api keys (%d,%d): %v", w, i, err)
				}

				// websites
				if _, err := webSvc.Create(ctx, zone, cid, "ipfs"); err != nil {
					t.Errorf("create website (%d,%d): %v", w, i, err)
				}
				if _, err := webSvc.List(ctx, websites.ListOptions{}); err != nil {
					t.Errorf("list websites (%d,%d): %v", w, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestTokenForDefaultAndEnvOverride guards the token derivation: the
// deterministic "token-<email>" default must stay (the sunpeak e2e suite and
// its fixture configs rely on token-e2e@example.com) and MCPHARNESS_TOKEN
// must override it so no token literal lives in source.
func TestTokenForDefaultAndEnvOverride(t *testing.T) {
	if got := tokenFor("e2e@example.com"); got != "token-e2e@example.com" {
		t.Fatalf("default token = %q, want %q", got, "token-e2e@example.com")
	}

	t.Setenv(harnessTokenEnv, "env-override-token")
	if got := tokenFor("e2e@example.com"); got != "env-override-token" {
		t.Fatalf("token with env override = %q, want %q", got, "env-override-token")
	}
}
