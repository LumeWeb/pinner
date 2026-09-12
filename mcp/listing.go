package mcp

// This file owns the SHARED tool-listing policy: the tools/list
// materialization strategy (progressive vs flat) and the host selector that
// decides which connected MCP hosts bypass progressive discovery. Both Pinner
// MCP consumers resolve through this one seam:
//
//   - the self-hosted CLI assembly (stdio / HTTP / embedded OpenAI tunnel,
//     with per-host reassembly over the HTTP mux), and
//   - a hosted (Portal-embedded) assembly, whose composition root declares
//     the policy for its audience (see Config.Listing).
//
// Ownership boundaries:
//
//   - canimcp answers only "what can the connected client do" (host +
//     transport + wire capability features resolved to a Profile). The
//     decision "flat vs progressive for THIS host" is a Pinner
//     product/listing decision, so it lives here — keyed on the same
//     canimcp HostType/TransportKind vocabulary canimcp detects. It is
//     deliberately NOT a canimcp Feature or a disclosure enum on canimcp's
//     Profile: canimcp stays host-only.
//
//   - The consumer-only listing axes (the onboarding recommendation set) are
//     carried as plain data so the full policy value can travel across the
//     module boundary; their evaluation (which names are "start here") stays
//     with the consumer that owns the primary-tool vocabulary.

import (
	"fmt"

	"go.lumeweb.com/canimcp"
)

// ToolListingStrategy names the tools/list materialization policy. The zero
// value is progressive (the default for every host that is not a flat-listing
// web host).
type ToolListingStrategy uint8

const (
	// ListingProgressive materializes only the direct set plus the
	// progressive-disclosure meta-tools on tools/list; the full catalog stays
	// reachable via search_tools → describe_tool → invoke_*.
	ListingProgressive ToolListingStrategy = iota
	// ListingFlat materializes every agent-safe enabled op directly on
	// tools/list. The progressive-disclosure meta-tools stay available unless
	// the consumer explicitly opts out (IncludeMetaOnFlat: *false) — the SAFE
	// default keeps them, so the gated entries flat deliberately excludes
	// (admin/wizard/interactive) remain reachable via the discovery
	// meta-tools.
	ListingFlat
)

// String returns a human-readable name for the strategy. Unknown values fall
// back to a generic "<strategy N>" so formatting never panics.
func (s ToolListingStrategy) String() string {
	switch s {
	case ListingProgressive:
		return "progressive"
	case ListingFlat:
		return "flat"
	}
	return fmt.Sprintf("<strategy %d>", uint8(s))
}

// Valid reports whether s is a supported listing strategy. It is the single
// gate for rejecting unsupported/unknown policy values at construction.
func (s ToolListingStrategy) Valid() bool {
	switch s {
	case ListingProgressive, ListingFlat:
		return true
	}
	return false
}

// ListingPolicy is the server-construction input that selects tools/list
// listing behavior. It carries ONLY the listing axes — the materialization
// strategy, the meta-on-flat switch, and the onboarding (direct) selection —
// and deliberately carries NO deployment axes (DomainScope/Hosted): those
// remain separate construction inputs so a partial listing policy can never
// overwrite a deployment surface or hosted flag.
type ListingPolicy struct {
	// Strategy is the tools/list materialization strategy. The zero value
	// (ListingProgressive) is the default and current behavior for every host
	// that is not a flat-listing web host.
	Strategy ToolListingStrategy
	// IncludeMetaOnFlat, when Strategy == ListingFlat, keeps the
	// progressive-disclosure meta-tools on tools/list alongside the direct
	// surface. It is inert under ListingProgressive.
	//
	// It is a *bool so "unset" is representable separately from "explicitly
	// false": a nil pointer (the zero value / omitted field) means the SAFE
	// default — flat keeps the discovery meta-tools, so the gated entries
	// (admin/wizard-category/interactive) that flat excludes from direct
	// registration stay reachable via search_tools/describe_tool/invoke_*
	// (see ResolveIncludeMetaOnFlat). A non-nil pointer to true keeps them;
	// a non-nil pointer to false is the explicit opt-out that hides those
	// gated operations behind the direct surface entirely.
	//
	// The host selector (PolicyForHost) DOES set this field for the flat web
	// cloud hosts: a dashboards-first web audience materializes the full
	// agent-safe surface directly and has no use for discovery machinery, so
	// the selector opts those hosts out explicitly. A composed policy the
	// caller built (not via the selector) still defaults to retaining them.
	IncludeMetaOnFlat *bool
	// Onboarding is an optional "start here" recommendation override, as
	// plain data. When empty, consumers defer to their own primary-tool
	// predicate; when non-empty it is a membership set independent of direct
	// status. It is never set by the host selector (PolicyForHost) — the
	// selector decides the strategy and the web-host meta opt-out only.
	Onboarding []string
}

// ResolveIncludeMetaOnFlat returns the effective meta-on-flat setting for the
// policy: TRUE when IncludeMetaOnFlat is nil (unset) — the safe invariant
// that a flat server keeps the discovery meta-tools on tools/list by default,
// so the gated entries flat excludes from direct registration stay reachable
// — and the explicit value otherwise.
func (p ListingPolicy) ResolveIncludeMetaOnFlat() bool {
	if p.IncludeMetaOnFlat == nil {
		return true
	}
	return *p.IncludeMetaOnFlat
}

// Validate reports whether the policy holds supported values. It rejects an
// unknown/unsupported listing strategy at construction time so an unsupported
// policy value fails loudly instead of silently falling back.
func (p ListingPolicy) Validate() error {
	if !p.Strategy.Valid() {
		return fmt.Errorf("mcp: unsupported listing strategy %v", p.Strategy)
	}
	return nil
}

// DefaultPolicy returns the default listing policy: progressive listing with
// the safe meta-on-flat default (nil IncludeMetaOnFlat resolves true).
func DefaultPolicy() ListingPolicy {
	return ListingPolicy{
		Strategy: ListingProgressive,
	}
}

// ---
// Host selector: which connected hosts bypass progressive discovery.
// ---

// StrategyForHost resolves the shared tools/list policy for a detected host
// on the declared transport. Claude Web, ChatGPT Web, Grok Web, and the
// Manufact Cloud dashboard use flat listings because MCP publishing
// requirements and host behavior make progressive discovery unreliable or
// unacceptable on cloud-hosted clients.
// Other hosts retain progressive discovery by default.
//
// This selector does not materialize tools/list itself. The composition root
// must apply the returned policy when registering tools and honor
// IncludeMetaOnFlat; Server.ListingPolicy exposes the resolved policy for
// consumers that assemble their own MCP server.
func StrategyForHost(host canimcp.HostType, transport canimcp.TransportKind) ToolListingStrategy {
	switch {
	case host == canimcp.HostClaude && transport == canimcp.TransportHTTP,
		host == canimcp.HostGrok && transport == canimcp.TransportHTTP,
		// Manufact Cloud is a remote web-dashboard client (HTTP-only profile);
		// exclude it from progressive discovery alongside the other web hosts.
		host == canimcp.HostManufact && transport == canimcp.TransportHTTP,
		(host == canimcp.HostOpenAI || host == canimcp.HostChatGPT) &&
			(transport == canimcp.TransportHTTP || transport == canimcp.TransportOpenAI):
		return ListingFlat
	default:
		return ListingProgressive
	}
}

// IsFlatListingHost reports whether the detected host on the declared
// transport bypasses progressive discovery (StrategyForHost == ListingFlat).
func IsFlatListingHost(host canimcp.HostType, transport canimcp.TransportKind) bool {
	return StrategyForHost(host, transport) == ListingFlat
}

// PolicyForHost resolves the listing policy selected by the shared host
// selector: DefaultPolicy with the strategy overridden to flat for the
// flat-listing web hosts. A flat web cloud host (Claude Web, Grok Web,
// ChatGPT/OpenAI Web, the Manufact Cloud dashboard) additionally opts out of
// the discovery meta-tools explicitly: these hosts are served the full
// agent-safe surface directly on tools/list and gain nothing from
// search_tools/describe_tool/invoke_* — the gated entries flat excludes
// (admin/wizard-category/interactive) are deliberately withheld from them,
// so keeping the meta-tools would only advertise machinery that resolves to
// nothing. An arbitrary flat policy a consumer composed itself (not via this
// selector) still retains the meta-tools under the safe unset default.
// Onboarding is left unset — consumers owning the "start here" vocabulary
// layer their own recommendation axes on top.
func PolicyForHost(host canimcp.HostType, transport canimcp.TransportKind) ListingPolicy {
	p := DefaultPolicy()
	p.Strategy = StrategyForHost(host, transport)
	if p.Strategy == ListingFlat {
		p.IncludeMetaOnFlat = &metaOff
	}
	return p
}

// metaOff backs the flat web-host opt-out in PolicyForHost. The pointer target
// must never be written; the selector hands out the immutable address.
var metaOff = false
