package assembly

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSurfaceZeroIsFull verifies the zero DomainScope behaves as the full surface,
// preserving backward compatibility for call sites that do not opt in.
func TestSurfaceZeroIsFull(t *testing.T) {
	var s DomainScope
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

// TestSurfacePresets verifies the exported presets: FullDomainScope turns on
// everything, while HostedDomainScope deliberately leaves out the Sia vault and
// portal admin but keeps the rest.
func TestSurfacePresets(t *testing.T) {
	assert.Equal(t, DomainScope{Account: true, Vault: true, Pins: true, Websites: true, DNS: true, IPNS: true, ENS: true, Operations: true, Admin: true, Upload: true}, FullDomainScope)

	assert.False(t, HostedDomainScope.IsZero())
	assert.False(t, HostedDomainScope.VaultOn(), "hosted surface must disable vault")
	assert.False(t, HostedDomainScope.AdminOn(), "hosted surface must disable admin")
	for _, on := range []bool{
		HostedDomainScope.AccountOn(),
		HostedDomainScope.PinsOn(),
		HostedDomainScope.WebsitesOn(),
		HostedDomainScope.DNSOn(),
		HostedDomainScope.IPNSOn(),
		HostedDomainScope.ENSOn(),
		HostedDomainScope.OperationsOn(),
		HostedDomainScope.UploadOn(),
	} {
		assert.True(t, on, "hosted surface must keep non-vault/admin domains")
	}
}

// TestSurfaceFlagOnSingleUnsetFieldIsDisabled verifies that once any field is
// set (the surface is no longer zero), every unset field is treated as
// disabled — the zero value is full, but a populated surface is explicit.
func TestSurfaceFlagOnSingleUnsetFieldIsDisabled(t *testing.T) {
	s := DomainScope{Account: true}
	assert.False(t, s.IsZero())
	assert.True(t, s.AccountOn())
	assert.False(t, s.VaultOn(), "populated surface treats unset fields as disabled")
	assert.False(t, s.AdminOn())
}
