package hosted

import (
	"fmt"

	"go.lumeweb.com/pinner/assembly"
	"go.lumeweb.com/pinner/mcp"
)

// Config is the explicit construction contract for a hosted (Portal-embedded)
// Pinner MCP server. Every dependency the assembly needs is declared here as
// an instance field — the domain scope, the catalog-deps factory, the
// per-request credential resolver, the transfer wiring, the OAuth handler, and
// the hosting environment (base URL, localhost-protection policy) — so a host
// assembles its server entirely from explicit values, never from hidden
// package state and never by importing pinner-cli.
//
// The zero value is not a valid hosted assembly: a hosted server must declare
// a domain surface (typically HostedDomainScope) and a catalog-deps factory
// (or a pre-assembled catalog, via the Catalog field) — the catalog is the
// source of the compiled tool surface.
type Config struct {
	// DomainScope declares which operation domains/tool families this hosted
	// server exposes. The zero value is the full surface; a hosted deployment
	// should opt in explicitly, typically with HostedDomainScope (which omits
	// the Sia vault and portal admin).
	DomainScope assembly.DomainScope

	// CatalogDeps supplies the operation-catalog dependency bundle for this
	// hosted server, resolved lazily per invocation so a live token/config
	// edit stays honored. It is the hosted equivalent of the CLI's local
	// bundle construction: the Portal API endpoint and per-request credential
	// resolution are threaded in explicitly (see CredentialResolver).
	// Invoked once per Catalog materialization/seed; the returned bundle's
	// closures re-read config and resolve services lazily.
	CatalogDeps func() *assembly.CatalogDepsBundle

	// CredentialResolver maps the OAuth-authenticated caller of a request onto
	// the Portal API token used to serve that request. It is threaded through
	// the operation dispatch so every hosted operation authenticates as the
	// calling user instead of a shared config token. When nil, ops fall back
	// to their config-token source.
	CredentialResolver CredentialResolver

	// OAuthHandler protects the /mcp endpoint with OAuth. When nil, the
	// handler is served unauthenticated (the caller is responsible for any
	// upstream auth, e.g. Portal middleware).
	OAuthHandler OAuthHandler

	// DisableLocalhostProtection disables the Streamable-HTTP localhost
	// (DNS-rebinding) protection. Required when the handler is served behind a
	// proxy/tunnel that presents a non-loopback Origin.
	DisableLocalhostProtection bool

	// Transfer carries the hosted IPFS-only transfer wiring: executor fns and
	// coordinator pointers handed in by the composition root. The vault is
	// never wired here. Zero fields mean the corresponding transfer tool is
	// simply not registered.
	Transfer mcp.TransferDeps

	// BaseURL is the externally reachable origin of this hosted server (e.g.
	// https://pinner.xyz). It is used to mint reachable presigned upload PUT
	// and filedrop GET URLs for the IPFS byte-route coordinators. When empty,
	// the coordinators fall back to their loopback-derived origin.
	BaseURL string
}

// HostedDomainScope returns the effective hosted domain surface: the explicit
// DomainScope when set, else HostedDomainScope. A config that left DomainScope
// at its zero value (the implicit full surface, which would admit the Sia
// vault and portal admin) is defaulted to the restricted hosted surface, so a
// hosted assembly never accidentally exposes CLI-local domains.
func (c Config) HostedDomainScope() DomainScope {
	if c.DomainScope.IsZero() {
		return HostedDomainScope
	}
	return c.DomainScope
}

// Validate checks that the config is a usable hosted construction: a
// catalog-deps factory (or an explicit catalog) must be present, since the
// catalog is the source of the compiled tool surface. It returns nil for a
// valid construction and a descriptive error otherwise.
func (c Config) Validate() error {
	if c.CatalogDeps == nil {
		return fmt.Errorf("hosted MCP server: no catalog deps: set Config.CatalogDeps (or supply a pre-assembled catalog)")
	}
	return nil
}
