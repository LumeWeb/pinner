package assembly

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/catalogmeta"
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
	// CfgMgr returns a live config manager for the current invocation. It is
	// not read by AssembleCatalogOps directly; the product wiring layer uses
	// it to build the per-domain deps below (e.g. AuthDeps/ApiKeysDeps that
	// need a live config accessor), resolved per invocation so a global/test
	// override stays live.
	CfgMgr func() config.Manager

	// CredentialResolver resolves the Portal API token for the authenticated
	// request. It is a wiring-layer input, not read by AssembleCatalogOps
	// directly: the product wiring threads it into the per-domain deps that
	// need it (e.g. AuthDeps.CredentialResolver, whose handlers surface
	// needs_auth). When nil the CLI/local default (read the bearer token from
	// config) is used; a hosted server supplies a Portal-auth mapping.
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
func assembleCatalogOps(cat opmesh.Catalog, ops []opmesh.Operation) error {
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
// scope controls which domains are registered. A domain whose scope flag
// is disabled is simply not added to the catalog, so a restricted scope
// (e.g. hosted mode, which excludes the Sia vault and portal admin) never
// advertises — or can invoke — those operations. The zero scope is the full
// scope.
//
// hosted declares whether this is a hosted (Portal-embedded) assembly. It is
// passed explicitly by the caller's server-construction path rather than
// inferred from scope equality or the presence of a CredentialResolver,
// which are orthogonal to deployment context. When hosted, operations whose
// Environment is EnvCLIOnly or EnvLocalOnly (e.g. auth_login/auth_logout,
// which mutate shared local config a stateless hosted server does not have)
// are excluded.
//
// A nil bundle is a wiring bug and is rejected here.
//
// The assembled catalog is the opmesh.Catalog interface (Add for
// registration; Search/Get/Describe/Invoke for consumption), built from the
// catalogops operation definitions. Their Environment metadata (the
// carve-outs) is consumed from the catalogmeta boundary package, keyed by
// the stable operation ID.
func AssembleCatalogOps(deps *CatalogDepsBundle, scope DomainScope, hosted bool) (opmesh.Catalog, error) {
	if deps == nil {
		return nil, fmt.Errorf("catalog assembly: nil catalog deps bundle")
	}

	// Hosted plugin surfaces must not carry subscription/plan-management
	// deep-links or subscribe prompting in operation results (platform
	// commerce policy); the account handlers gate on this stamped flag and
	// CLI/local assemblies keep the zero value (full deep-link UX).
	deps.Account.HostedPlugin = hosted

	cat := opmesh.NewCatalog()

	// Map each catalogops domain to its scope flag. A disabled domain's
	// operations are never produced, so they are absent from search/describe/
	// invoke and their hand-off/setup handlers are never reachable.
	domains := []struct {
		name    string
		enabled bool
		ops     []opmesh.Operation
	}{
		{"auth", scope.AccountOn(), catalogops.AuthOperations(deps.Auth)},
		{"account", scope.AccountOn(), catalogops.AccountOperations(deps.Account)},
		{"api-keys", scope.AccountOn(), catalogops.APIKeysOperations(deps.APIKeys)},
		{"vault-setup", scope.VaultOn(), catalogops.VaultSetupOperations(deps.VaultSetup)},
		{"vault", scope.VaultOn(), catalogops.VaultOperations(deps.Vault)},
		{"pins", scope.PinsOn(), catalogops.PinsOperations(deps.Pins)},
		{"websites", scope.WebsitesOn(), catalogops.WebsitesOperations(deps.Websites)},
		{"dns", scope.DNSOn(), catalogops.DNSOperations(deps.DNS)},
		{"ipns", scope.IPNSOn(), catalogops.IPNSOperations(deps.IPNS)},
		{"ens", scope.ENSOn(), catalogops.ENSOperations(deps.ENS)},
		{"operations", scope.OperationsOn(), catalogops.OperationsOperations(deps.Operations)},
		{"admin", scope.AdminOn(), catalogops.AdminOperations(deps.Admin)},
	}

	// An operation's Environment restricts which scopes may register it.
	// The Environment carve-outs are frontend metadata
	// (catalogmeta.EnvironmentOf, keyed by the stable operation ID) — the
	// opmesh core model deliberately carries none. Hosted mode is a
	// Portal-embedded assembly whose catalog must never
	// advertise CLI-local complexity (EnvLocalOnly, EnvCLIOnly) — e.g.
	// auth_login/auth_logout, which mutate shared local config that a stateless
	// hosted server does not have. The hosted flag is declared explicitly by
	// the caller's construction path rather than inferred from scope equality
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
// scope. In hosted mode the EnvLocalOnly and EnvCLIOnly operations are
// excluded (they mutate or depend on shared local config / are CLI frontend
// only). In CLI/local mode every operation is kept except EnvHostedOnly, which
// none are declared to be today.
func filterOpsForEnvironment(ops []opmesh.Operation, hosted bool) []opmesh.Operation {
	return lo.Filter(ops, func(op opmesh.Operation, _ int) bool {
		env := catalogmeta.EnvironmentOf(op.Name())
		if hosted && (env == catalogmeta.EnvCLIOnly || env == catalogmeta.EnvLocalOnly) {
			return false
		}
		if !hosted && env == catalogmeta.EnvHostedOnly {
			return false
		}
		return true
	})
}
