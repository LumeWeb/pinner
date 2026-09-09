package assembly

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSurfaceZeroIsFull verifies the zero Surface behaves as the full surface,
// preserving backward compatibility for call sites that do not opt in.
func TestSurfaceZeroIsFull(t *testing.T) {
	var s Surface
	assert.True(t, s.IsZero())
	assert.True(t, s.AccountOn())
	assert.True(t, s.VaultOn())
	assert.True(t, s.PinsOn())
	assert.True(t, s.WebsitesOn())
	assert.True(t, s.DNSOn())
	assert.True(t, s.IPNSOn())
	assert.True(t, s.ENSOn())
	assert.True(t, s.OperationsOn())
	assert.True(t, s.AdminOn())
	assert.True(t, s.UploadOn())
}

// TestSurfacePresets verifies the exported presets: FullSurface turns on
// everything, while HostedSurface deliberately leaves out the Sia vault and
// portal admin but keeps the rest.
func TestSurfacePresets(t *testing.T) {
	assert.Equal(t, Surface{Account: true, Vault: true, Pins: true, Websites: true, DNS: true, IPNS: true, ENS: true, Operations: true, Admin: true, Upload: true}, FullSurface)

	assert.False(t, HostedSurface.IsZero())
	assert.False(t, HostedSurface.VaultOn(), "hosted surface must disable vault")
	assert.False(t, HostedSurface.AdminOn(), "hosted surface must disable admin")
	for _, on := range []bool{
		HostedSurface.AccountOn(),
		HostedSurface.PinsOn(),
		HostedSurface.WebsitesOn(),
		HostedSurface.DNSOn(),
		HostedSurface.IPNSOn(),
		HostedSurface.ENSOn(),
		HostedSurface.OperationsOn(),
		HostedSurface.UploadOn(),
	} {
		assert.True(t, on, "hosted surface must keep non-vault/admin domains")
	}
}

// TestSurfaceFlagOnSingleUnsetFieldIsDisabled verifies that once any field is
// set (the surface is no longer zero), every unset field is treated as
// disabled — the zero value is full, but a populated surface is explicit.
func TestSurfaceFlagOnSingleUnsetFieldIsDisabled(t *testing.T) {
	s := Surface{Account: true}
	assert.False(t, s.IsZero())
	assert.True(t, s.AccountOn())
	assert.False(t, s.VaultOn(), "populated surface treats unset fields as disabled")
	assert.False(t, s.AdminOn())
}
