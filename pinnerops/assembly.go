package pinnerops

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"go.lumeweb.com/pinner"
	"go.lumeweb.com/pinner/catalogops"
	"go.lumeweb.com/pinner/core/config"
)

// ErrNotAuthenticated is returned by a CredentialResolver when the current
// request has no usable Portal API credential. Handlers surface it as a
// structured needs_auth hand-off rather than a bare error.
var ErrNotAuthenticated = errors.New("not authenticated")

// CredentialResolver resolves the Portal API token for the authenticated
// principal of the current request. It is the seam that lets a hosted
// (Portal-embedded) server route the MCP OAuth backend to Portal's own OAuth
// library/IdP (which has already validated the caller and established a
// user) instead of forcing the CLI's config-token assumptions.
//
// The CLI/local MCP server uses ConfigCredentialResolver (reads the bearer
// token from the pinner config). A hosted server supplies an implementation
// that maps the Portal-authenticated user onto a Portal API JWT.
type CredentialResolver interface {
	// TokenForRequest returns the Portal API token for the currently
	// authenticated request, or ErrNotAuthenticated when there is none.
	TokenForRequest(ctx context.Context) (string, error)
}

// ConfigCredentialResolver adapts a config accessor into a CredentialResolver
// that reads the stored bearer token, mirroring how the CLI/local MCP server
// has always obtained its bearer token.
type ConfigCredentialResolver struct {
	// AuthToken returns the config-stored bearer token. It is resolved per
	// invocation so a token edit stays live. When nil (or when it yields an
	// empty token) TokenForRequest returns ErrNotAuthenticated.
	AuthToken func() (string, error)
}

// TokenForRequest implements CredentialResolver.
func (r ConfigCredentialResolver) TokenForRequest(ctx context.Context) (string, error) {
	token := ""
	var err error
	if r.AuthToken != nil {
		token, err = r.AuthToken()
		if err != nil {
			return "", err
		}
	}
	if token == "" {
		return "", ErrNotAuthenticated
	}
	return token, nil
}

var _ CredentialResolver = ConfigCredentialResolver{}

// CatalogDepsBundle carries the concrete dependency graph the operation-catalog
// assembly needs to construct every catalogops domain. It is built by a product
// wiring layer (which has the config manager and all core service factories)
// and handed to AssembleCatalogOps. Each domain's deps use getter/closures
// resolved per invocation (the lazy-deps pattern used throughout catalogops) so
// a test/global override stays live and services always use fresh config,
// never a package-init snapshot.
//
// The bundle deliberately spans the whole catalogops surface: auth, account,
// vault, vault-setup, pins, websites, dns, ipns, ens, api-keys, operations, and
// admin. A domain whose deps are nil degrades to ops that fail with a clear
// "service unavailable" error rather than panicking, so the bundle can be added
// incrementally.
type CatalogDepsBundle struct {
	// CfgMgr returns a live config manager for the current invocation.
	CfgMgr func() config.Manager

	// CredentialResolver resolves the Portal API token for the authenticated
	// request. When nil, the CLI/local default (read the bearer token from
	// config) is used. A hosted server sets this to map a Portal-authenticated
	// user onto a Portal API JWT.
	CredentialResolver CredentialResolver

	Auth       catalogops.AuthDeps
	Account    catalogops.AccountDeps
	Vault      catalogops.VaultDeps
	VaultSetup catalogops.VaultDeps
	Pins       catalogops.PinsDeps
	Websites   catalogops.WebsitesDeps
	DNS        catalogops.DNSDeps
	IPNS       catalogops.IPNSDeps
	ENS        catalogops.ENSDeps
	APIKeys    catalogops.APIKeysDeps
	Operations catalogops.OperationsDeps
	Admin      catalogops.AdminDeps
}

// assembleCatalogOps registers every operation produced by one catalogops
// domain provider into cat. It stops at the first registration error so a
// genuinely malformed operation (duplicate name, invalid arg metadata) is
// surfaced rather than silently dropped, matching how product wiring treats
// catalog construction.
func assembleCatalogOps(cat pinner.Catalog, ops []pinner.Operation) error {
	for _, op := range ops {
		if err := cat.Add(op); err != nil {
			return err
		}
	}
	return nil
}

// AssembleCatalogOps builds a single operation catalog covering the
// catalogops surface: auth, account, vault-setup, vault, pins, websites, dns,
// ipns, ens, api-keys, operations, and admin operations. Each domain's
// operations are derived from the corresponding CatalogDepsBundle field via
// the catalogops provider functions. A nil deps field is fine: catalogops
// degrades such a domain to operations that fail with a clear "service
// unavailable" error at execution time, so registration never fails purely
// because a dependency is missing.
//
// surface controls which domains are registered. A domain whose surface flag
// is disabled is simply not added to the catalog, so a restricted surface
// (e.g. hosted mode, which excludes the Sia vault and portal admin) never
// advertises — or can invoke — those operations. The zero surface is the full
// surface.
//
// hosted declares whether this is a hosted (Portal-embedded) assembly. It is
// passed explicitly by the caller's server-construction path rather than
// inferred from surface equality or the presence of a CredentialResolver,
// which are orthogonal to deployment context. When hosted, operations whose
// Environment is EnvCLIOnly or EnvLocalOnly (e.g. auth_login/auth_logout,
// which mutate shared local config a stateless hosted server does not have)
// are excluded.
//
// A nil bundle is a wiring bug and is rejected here.
//
// Note on the return type: pinner.NewCatalog returns the pinner.Catalog
// interface (its concrete backing type is unexported), so the assembled
// catalog is returned as the pinner.Catalog interface, which exposes Add for
// registration and Search/Get/Describe/Invoke for consumption.
func AssembleCatalogOps(deps *CatalogDepsBundle, surface Surface, hosted bool) (pinner.Catalog, error) {
	if deps == nil {
		return nil, fmt.Errorf("catalog assembly: nil catalog deps bundle")
	}

	cat := pinner.NewCatalog()

	// Map each catalogops domain to its surface flag. A disabled domain's
	// operations are never produced, so they are absent from search/describe/
	// invoke and their hand-off/setup handlers are never reachable.
	domains := []struct {
		name    string
		enabled bool
		ops     []pinner.Operation
	}{
		{"auth", surface.AccountOn(), catalogops.AuthOperations(deps.Auth)},
		{"account", surface.AccountOn(), catalogops.AccountOperations(deps.Account)},
		{"api-keys", surface.AccountOn(), catalogops.APIKeysOperations(deps.APIKeys)},
		{"vault-setup", surface.VaultOn(), catalogops.VaultSetupOperations(deps.VaultSetup)},
		{"vault", surface.VaultOn(), catalogops.VaultOperations(deps.Vault)},
		{"pins", surface.PinsOn(), catalogops.PinsOperations(deps.Pins)},
		{"websites", surface.WebsitesOn(), catalogops.WebsitesOperations(deps.Websites)},
		{"dns", surface.DNSOn(), catalogops.DNSOperations(deps.DNS)},
		{"ipns", surface.IPNSOn(), catalogops.IPNSOperations(deps.IPNS)},
		{"ens", surface.ENSOn(), catalogops.ENSOperations(deps.ENS)},
		{"operations", surface.OperationsOn(), catalogops.OperationsOperations(deps.Operations)},
		{"admin", surface.AdminOn(), catalogops.AdminOperations(deps.Admin)},
	}

	// An operation's Environment restricts which surfaces may register it.
	// Hosted mode is a Portal-embedded assembly whose catalog must never
	// advertise CLI-local complexity (EnvLocalOnly, EnvCLIOnly) — e.g.
	// auth_login/auth_logout, which mutate shared local config that a stateless
	// hosted server does not have. The hosted flag is declared explicitly by
	// the caller's construction path rather than inferred from surface equality
	// or CredentialResolver presence, so a local stdio server that disables
	// Vault/Admin is never misclassified as hosted. The CLI/local path
	// registers everything except hosted-only ops.
	for _, d := range domains {
		if !d.enabled {
			continue
		}
		ops := filterOpsForEnvironment(d.ops, hosted)
		if err := assembleCatalogOps(cat, ops); err != nil {
			return nil, fmt.Errorf("catalog assembly: register %s domain: %w", d.name, err)
		}
	}

	return cat, nil
}

// filterOpsForEnvironment drops operations that are not valid on the active
// surface. In hosted mode the EnvLocalOnly and EnvCLIOnly operations are
// excluded (they mutate or depend on shared local config / are CLI frontend
// only). In CLI/local mode every operation is kept except EnvHostedOnly, which
// none are declared to be today.
func filterOpsForEnvironment(ops []pinner.Operation, hosted bool) []pinner.Operation {
	return lo.Filter(ops, func(op pinner.Operation, _ int) bool {
		env := op.Environment()
		if hosted && (env == pinner.EnvCLIOnly || env == pinner.EnvLocalOnly) {
			return false
		}
		if !hosted && env == pinner.EnvHostedOnly {
			return false
		}
		return true
	})
}
