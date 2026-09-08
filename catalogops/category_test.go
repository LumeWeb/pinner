package catalogops

import "testing"

// TestVaultCategoryPinsValue guards the canonical wire category for vault
// operations. Catalog clients and agent instructions filter on the literal
// "vault", so the shared constant must not drift from that value, and vault
// operations must carry that constant (not an inline divergent literal).
func TestVaultCategoryPinsValue(t *testing.T) {
	if CategoryVault != "vault" {
		t.Fatalf("CategoryVault = %q, want %q", CategoryVault, "vault")
	}
	if got := vaultStatus(VaultDeps{}).Category(); got != CategoryVault {
		t.Fatalf("vault_status Category = %q, want %q", got, CategoryVault)
	}
}
