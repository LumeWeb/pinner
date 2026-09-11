package hosted

import (
	"fmt"
	"reflect"
)

// normalizeCredentialResolver resolves the ONE effective per-request
// credential resolver for a hosted assembly so the operation-catalog dispatch
// and the HTTP/transfer path can never disagree about identity:
//
//   - nil explicit resolver defers to a resolver already supplied on the
//     catalog-deps bundle (the "bundle-only" hosted setup): that resolver
//     then drives BOTH the bundle seeding and the credential middleware.
//   - both supplied and identical (pointer/value-equal) is accepted.
//   - both supplied and DIFFERENT is a construction wiring conflict — two
//     resolvers would let compiled operations and the transfer path
//     authenticate under different identities — and fails closed.
//
// Uncomparable (closure-backed) resolver implementations cannot be proven
// value-identical by reflection, but that is NOT evidence of a conflict: a
// closure wired the same to both places is a valid (and common) hosted setup.
// Closures may therefore opt in to the proof with the
// IdentifiableCredentialResolver interface (a stable ResolverIdentity for the
// underlying credential source): equal identities are accepted as THE SAME
// resolver. Anything that proves neither value-equality nor a shared identity
// fails closed.
func normalizeCredentialResolver(explicit, fromBundle CredentialResolver) (CredentialResolver, error) {
	switch {
	case explicit == nil:
		return fromBundle, nil
	case fromBundle == nil:
		return explicit, nil
	}
	if sameCredentialResolver(explicit, fromBundle) {
		return explicit, nil
	}
	return nil, fmt.Errorf("hosted MCP server: conflicting credential resolvers: the explicit resolver and the catalog-deps bundle's resolver must be the same resolver")
}

// NormalizeCredentialResolvers folds the pairwise normalizeCredentialResolver
// rule across the explicit resolver and EVERY sampled construction-time bundle
// resolver, so a non-uniform catalog-deps factory (e.g. nil on the first
// invocation, a resolver afterwards) normalizes to the non-nil resolver for
// every boundary instead of leaving one boundary resolver-less, and two
// observed-but-disagreeing resolvers fail construction closed (the same
// conflict rule the pairwise check enforces).
func NormalizeCredentialResolvers(explicit CredentialResolver, sampled []CredentialResolver) (CredentialResolver, error) {
	effective := explicit
	for _, r := range sampled {
		var err error
		effective, err = normalizeCredentialResolver(effective, r)
		if err != nil {
			return nil, err
		}
	}
	return effective, nil
}

// sameCredentialResolver reports whether a and b provably resolve through the
// SAME credential source: reflect value-equality when the concrete types are
// comparable, or — for uncomparable closure-backed resolvers — equal
// IdentifiableCredentialResolver identities. Inability to prove equality is a
// conflict (fail closed), never an accepted-equality shortcut.
func sameCredentialResolver(a, b CredentialResolver) bool {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.Comparable() && bv.Comparable() {
		return av.Equal(bv)
	}
	ia, aok := a.(IdentifiableCredentialResolver)
	ib, bok := b.(IdentifiableCredentialResolver)
	if !aok || !bok {
		// Cannot prove the two closures are the same wired resolver; a
		// one-sided identity proves nothing.
		return false
	}
	iaID, ibID := ia.ResolverIdentity(), ib.ResolverIdentity()
	return iaID != "" && ibID != "" && iaID == ibID
}
