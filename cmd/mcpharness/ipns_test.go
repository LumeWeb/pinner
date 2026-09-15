package main

// ipns_test.go is a regression test for the harness fake IPNS service. The
// fake embeds the nil ipns.Service interface and, before fakeIPNSService
// implemented ListKeysPage, the non-search ipns_keys_list path panicked with a
// nil pointer dereference when the harness ran it. This test drives the real
// catalog operation through ipnsDepsFor to prove the non-search path resolves
// through the fake without panicking and reports the correct total.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/pinner/catalogops"
)

func TestFakeIPNSListKeysPageNonSearchDoesNotPanic(t *testing.T) {
	f, err := NewFakeServices("ipns@example.com")
	require.NoError(t, err)

	var listOp opmesh.Operation
	for _, op := range catalogops.IPNSOperations(ipnsDepsFor(f)) {
		if op.Name() == "ipns_keys_list" {
			listOp = op
			break
		}
	}
	require.NotNil(t, listOp, "ipns_keys_list operation must be present")

	// No search arg: the handler takes the non-search path and calls
	// svc.ListKeysPage, which must resolve on the fake rather than the
	// promoted nil interface.
	res, err := listOp.Handler().Execute(context.Background(), map[string]any{})
	require.NoError(t, err, "non-search ipns_keys_list must not panic or error")

	lr, ok := res.(catalogops.ListResult)
	require.True(t, ok, "result type = %T, want catalogops.ListResult", res)
	require.Equal(t, 1, lr.ListTotal(), "non-search list must report the seeded key total")
}
