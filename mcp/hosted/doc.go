// Package hosted carries the reusable, SDK-independent construction contracts
// for assembling a hosted (Portal-embedded) Pinner MCP server. It is the thin
// public layer that lets a product like the LumeWeb Portal's MCP plugin bind to
// pinner directly — declaring its explicit credentials, resolvers, catalog
// dependencies, domain scope, and transfer/base-URL wiring — without importing
// pinner-cli or any MCP protocol SDK.
//
// The contracts here are deliberately SDK-free. The MCP protocol server wiring
// (dispatch, Apps, materialization, the streamable-HTTP handler) stays with the
// composition root; this package owns only the seam and configuration shapes
// that every hosted assembly shares: the auth seams (CredentialResolver,
// OAuthHandler) a host implements against its own IdP, the hosted domain
// surface (HostedDomainScope over assembly.DomainScope), the explicit
// construction Config, and the credential-resolver reconciliation rule
// (NormalizeCredentialResolvers) that keeps the HTTP boundary and catalog
// dispatch from authenticating a request under divergent identities.
//
// None of these types import pinner-cli. They depend only on
// go.lumeweb.com/pinner/{assembly, mcp} and the Go standard library, so a
// host can adopt them the moment this module publishes them, and the CLI's
// local composition and the plugin's hosted composition can both shoulder the
// same reconciliation semantics.
package hosted
