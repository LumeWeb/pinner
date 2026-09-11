package hosted

import (
	"go.lumeweb.com/pinner/assembly"
)

// Normalized is the construction-time plan a host applies to its protocol
// wiring after Normalize. It carries exactly the values that must be shared —
// and therefore must not disagree — across every boundary the host wires: the
// resolved domain surface, the per-invocation catalog-deps factory, and the
// single effective per-request credential resolver.
type Normalized struct {
	// DomainScope is the effective hosted surface: the explicit Config
	// scope when set, else HostedDomainScope.
	DomainScope assembly.DomainScope

	// CatalogDeps is the per-invocation catalog-deps factory from Config,
	// preserved for materialization.
	CatalogDeps func() *assembly.CatalogDepsBundle

	// CredentialResolver is the ONE effective per-request credential resolver:
	// Config.CredentialResolver reconciled against the resolver sampled from
	// the catalog-deps bundle per NormalizeCredentialResolvers, so the HTTP
	// boundary and catalog dispatch authenticate under a single identity.
	CredentialResolver CredentialResolver
}

// Normalize is the SDK-free hosted-construction orchestration step. It
// validates the Config, resolves the effective hosted surface, samples the
// catalog-deps factory once to reconcile its resolver against the explicit
// CredentialResolver, and returns the construction plan. The sample is a
// construction-time reconciliation only; the factory is preserved on
// Normalized for per-invocation materialization, so a live token/config edit
// stays honored.
func Normalize(cfg Config) (Normalized, error) {
	if err := cfg.Validate(); err != nil {
		return Normalized{}, err
	}

	scope := cfg.HostedDomainScope()

	var sampled []CredentialResolver
	if bundle := cfg.CatalogDeps(); bundle != nil && bundle.CredentialResolver != nil {
		sampled = append(sampled, bundle.CredentialResolver)
	}
	effective, err := NormalizeCredentialResolvers(cfg.CredentialResolver, sampled)
	if err != nil {
		return Normalized{}, err
	}

	return Normalized{
		DomainScope:        scope,
		CatalogDeps:        cfg.CatalogDeps,
		CredentialResolver: effective,
	}, nil
}
