package hosted

import (
	"net/http"

	"go.lumeweb.com/pinner/assembly"
)

// CredentialResolver resolves the Portal API token for the authenticated
// principal of the current request. It is the seam that lets a hosted
// (Portal-embedded) server route the MCP OAuth backend to its own OAuth
// library/IdP — which has already validated the caller and established a user —
// instead of forcing CLI config-token assumptions.
//
// This is the same contract assembly.CredentialResolver owns; it is aliased
// here so a hosted-construction consumer can name the seam from the package
// that assembles the server rather than reaching into the catalog-assembly
// package. TokenForRequest returns assembly.ErrNotAuthenticated when there is
// no authenticated caller.
type CredentialResolver = assembly.CredentialResolver

// IdentifiableCredentialResolver is an optional interface a CredentialResolver
// may implement to expose a STABLE identity for its underlying credential
// source. Go cannot compare closures/functions (reflect: funcs are
// non-comparable), so two resolvers wiring the same closure-backed source can
// never be proven value-identical. An implementation whose identity method
// returns an equal value for the same underlying source (e.g. a user-scoped
// resolver instance ID, constant per closure) opts in to that proof: THE SAME
// such closure wired to two boundaries is then accepted instead of rejected as
// a conflict. Two implementations whose identities disagree are still a
// construction wiring conflict, and any resolver that proves neither
// value-equality nor a shared identity fails construction closed.
type IdentifiableCredentialResolver interface {
	// ResolverIdentity returns a stable identifier for the resolver's
	// UNDERLYING credential source, not per invocation: two closures wrapping
	// the same credential source must return the same string (and different
	// sources must not). Deterministic across calls on one value.
	ResolverIdentity() string
}

// OAuthHandler protects the hosted MCP HTTP endpoint with OAuth. It is the
// surface-agnostic seam between the MCP implementation and an authorization
// server:
//
//   - CLI mode: implemented by the CLI's own OAuth AS (go.lumeweb.com/oauth,
//     login page, dynamic client registration).
//   - Hosted mode: implemented by the Portal plugin, which delegates to the
//     Portal's OAuthProviderService (ValidateAccessToken, RFC 8414/9728) — the
//     hosted MCP server never needs to know OAuth exists.
type OAuthHandler interface {
	// WrapHTTP wraps the /mcp streamable-HTTP handler with OAuth enforcement
	// (validate Authorization: Bearer, emit 401 + WWW-Authenticate pointing at
	// the protected-resource metadata when invalid). next is the authenticated-
	// session downstream handler.
	WrapHTTP(next http.Handler) http.Handler
}
