package mcp

import (
	"fmt"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/model"
)

// The platform-profile vocabulary of the mcp package.
//
// The DSL context needed by the guide/descriptor fragments must carry
// mcpforge's FeatureSet plus the host/transport/hosted facts the prose
// predicates gate on. HostProfile fills that role with canimcp types —
// byte-identical feature strings (kept in lockstep with canimcp.Feature) so
// remote profiles need no translation.

// The package feature vocabulary, expressed as mcpforge.Feature. The string
// values are identical to canimcp.Feature's wire values, so adapting a
// canimcp.Profile or an mcpplane model.Profile to HostProfile is a pure map
// copy, never a string rewrite.
const (
	// FeatFileHostInput gates the host-provided `file` parameter handoff
	// ({download_url, file_id}; OpenAI/ChatGPT hosts).
	FeatFileHostInput = mcpforge.Feature("file-host-input")
	// FeatSourcePath gates the co-located filesystem source (stdio).
	FeatSourcePath = mcpforge.Feature("source-path")
	// FeatSourceMint gates the presigned HTTP PUT source (HTTP transport).
	FeatSourceMint = mcpforge.Feature("source-mint")
	// FeatSourceURL gates the server-fetchable HTTPS URL relay (OpenAI tunnel).
	FeatSourceURL = mcpforge.Feature("source-url")
	// FeatSourceData gates the RFC 2397 data-URI relay (OpenAI tunnel).
	FeatSourceData = mcpforge.Feature("source-data")
	// FeatXMcpFile gates the draft x-mcp-file schema metadata.
	FeatXMcpFile = mcpforge.Feature("x-mcp-file")
	// FeatSinkLocal gates the host-side disk write sink.
	FeatSinkLocal = mcpforge.Feature("sink-local")
	// FeatSinkDrop gates the one-time HTTP GET filedrop sink.
	FeatSinkDrop = mcpforge.Feature("sink-drop")
	// FeatMCPApps gates MCP Apps (ui:// interactive views).
	FeatMCPApps = mcpforge.Feature("mcp-apps-ui")
	// FeatElicitation gates form/URL elicitation.
	FeatElicitation = mcpforge.Feature("elicitation")
	// FeatRemoteAccess marks a server reachable over HTTP from the client.
	FeatRemoteAccess = mcpforge.Feature("remote-access")
	// FeatCoLocated marks a server sharing the client's filesystem.
	FeatCoLocated = mcpforge.Feature("co-located")
)

// HostProfile is the platform context every description-DSL
// fragment resolves against. It carries exactly what the prose predicates
// need — the mcpforge feature set plus the host, transport, and hosted facts —
// and satisfies mcpforge.FeatureCarrier so it plugs into the shared DSL the
// tool schemas use. Unlike the other builders (whose C is inferable), the
// fragments here resolve against this concrete type via nearby constructors.
type HostProfile struct {
	// Features is the resolved capability feature set. A nil map reads as an
	// empty set: every gate is omitted (the documented profile-less case).
	Features mcpforge.FeatureSet
	// Transport is the MCP transport the server runs under.
	Transport canimcp.TransportKind
	// Host is the detected MCP client platform (HostGeneric when unknown).
	Host canimcp.HostType
	// Hosted reports whether this is a hosted (Portal-embedded) deployment.
	// It is a server-construction-time property overlaid by Config.Hosted,
	// never inferred from a per-request wire signal.
	Hosted bool
}

// FeatureSet implements mcpforge.FeatureCarrier.
func (p HostProfile) FeatureSet() mcpforge.FeatureSet { return p.Features }

// Has reports whether the resolved feature set declares f. A nil map reads as
// absent, so a zero HostProfile gates every conditional segment off.
func (p HostProfile) Has(f mcpforge.Feature) bool { return p.Features.Has(f) }

// CloneFeatures returns a copy of p with a cloned feature set, so overlays
// (e.g. clearing a feature for description resolution) never mutate a shared
// underlying map.
func (p HostProfile) CloneFeatures() HostProfile {
	out := p
	out.Features = p.Features.Clone()
	return out
}

// ClearFeature removes f from the profile's (cloned) feature set, returning
// the adjusted copy. Callers that adapt descriptions to registration-time
// wiring (e.g. capabilities without any file-capable tool wired) use it so
// the advertised prose matches the report.
func (p HostProfile) ClearFeature(f mcpforge.Feature) HostProfile {
	out := p.CloneFeatures()
	delete(out.Features, f)
	return out
}

// SetFeature adds f to the profile's (cloned) feature set, returning the
// adjusted copy. Used by the wiring-derived download profile (sink-drop only
// when a reachable mux exists).
func (p HostProfile) SetFeature(f mcpforge.Feature) HostProfile {
	out := p.CloneFeatures()
	out.Features[f] = true
	return out
}

// ---
// Predicates over HostProfile. mcpforge predicates are consumer-supplied, so
// the host/transport/hosted predicate constructors are declared here,
// generic over the DSL context's concrete type.
// ---

// HostIs matches profiles connected from the given host.
func HostIs(h canimcp.HostType) mcpforge.Predicate[HostProfile] {
	return func(p HostProfile) bool { return p.Host == h }
}

// HostedIs matches profiles from a hosted (Portal-embedded) deployment.
func HostedIs(hosted bool) mcpforge.Predicate[HostProfile] {
	return func(p HostProfile) bool { return p.Hosted == hosted }
}

// TransportIs matches profiles running on the given transport.
func TransportIs(t canimcp.TransportKind) mcpforge.Predicate[HostProfile] {
	return func(p HostProfile) bool { return p.Transport == t }
}

// transportFeaturesFor returns the mechanism feature set each transport
// implies: which source/sink modes file upload/download can actually serve,
// plus the reachability facts. It mirrors cans/mcpforge semantics (an HTTP
// host always gets mint, stdio path, the OpenAI tunnel url/data) and is the
// mechanism half every HostProfile is composed from.
func transportFeaturesFor(t canimcp.TransportKind) mcpforge.FeatureSet {
	switch t {
	case canimcp.TransportStdio:
		return mcpforge.FeatureSet{
			FeatSourcePath: true, FeatSinkLocal: true,
			FeatSinkDrop: true, FeatCoLocated: true,
		}
	case canimcp.TransportHTTP:
		return mcpforge.FeatureSet{
			FeatSourceMint: true, FeatSinkLocal: true,
			FeatSinkDrop: true, FeatRemoteAccess: true,
		}
	case canimcp.TransportOpenAI:
		return mcpforge.FeatureSet{
			FeatSourceURL: true, FeatSourceData: true, FeatSinkLocal: true,
		}
	default:
		return mcpforge.FeatureSet{}
	}
}

// profileForTransport returns the transport-generic HostProfile for t,
// composed from that transport's mechanism features, sans any host-specific
// caps.
func profileForTransport(t canimcp.TransportKind) HostProfile {
	return HostProfile{
		Features:  transportFeaturesFor(t).Clone(),
		Transport: t,
		Host:      canimcp.HostGeneric,
	}
}

// openAITunnelHostFeatures returns the host-capability features the embedded
// OpenAI tunnel declares on top of the transport mechanism set: the ChatGPT
// file-reference handoff (FeatFileHostInput), the draft x-mcp-file metadata
// (FeatXMcpFile), MCP Apps (FeatMCPApps), and elicitation (FeatElicitation).
// These are host capabilities, not transport mechanisms, but the embedded
// OpenAI tunnel is the one transport whose startup-effective profile is
// host-specific (it only ever serves ChatGPT/OpenAI hosts), so the effective
// startup fallback derives these caps straight from
// ProfileForTransport(TransportOpenAI). Any assembly-time fallback that
// composes the tunnel's effective features must merge them in — a tunnel that
// publishes only the mechanism set (no host-file schema property, no ChatGPT
// metadata, no host-file description segments) under-reports the host's
// actual surface, the regression class this helper exists to prevent.
func openAITunnelHostFeatures() mcpforge.FeatureSet {
	return mcpforge.FeatureSet{
		FeatFileHostInput: true,
		FeatXMcpFile:      true,
		FeatMCPApps:       true,
		FeatElicitation:   true,
	}
}

// transportStartupFeatures returns the registration-time fallback feature set
// for a transport when the wiring carries no explicit RelayFeatures. It is the
// transport's mechanism set, with the OpenAI tunnel's ChatGPT host
// capabilities merged in.
func transportStartupFeatures(t canimcp.TransportKind) mcpforge.FeatureSet {
	features := transportFeaturesFor(t).Clone()
	if t == canimcp.TransportOpenAI {
		for f, on := range openAITunnelHostFeatures() {
			features[f] = on
		}
	}
	return features
}

// ---
// Profile adaptation across the module boundary.
// ---

// ProfileAdapterError reports a Config.Profile shape this package cannot
// adapt. Construction fails loudly (never silently featureless), mirroring
// catalogmcp.ProfileAdapterError, so integration mistakes surface as errors.
type ProfileAdapterError struct {
	// ProfileType is the Go type of the unadapted profile (fmt %T).
	ProfileType string
}

// Error implements error.
func (e *ProfileAdapterError) Error() string {
	return "mcp: profile of type " + e.ProfileType +
		" cannot be adapted; accepted shapes are nil (intentionally profile-less), " +
		"a catalogmcp.ForgeFeatureCarrier (FeatureSet() mcpforge.FeatureSet), " +
		"a *canimcp.Profile, or a *model.Profile"
}

// AdaptHostProfile converts an opaque configured profile into a HostProfile,
// so the presentation fragments (guide, capabilities, transfer descriptors)
// and the catalog compiler share one adaptation path at the any-typed
// Config.Profile boundary. Accepted shapes, all documented and meaningful:
//
//   - nil — intentionally profile-less: every feature gate resolves as
//     omitted (the base description; HostGeneric/Transport ""/not hosted).
//   - a catalogmcp.ForgeFeatureCarrier — its mcpforge.FeatureSet is adopted.
//   - a *canimcp.Profile — feature strings are byte-identical, so the set
//     copies across unchanged; host and transport copy directly.
//   - a *model.Profile (mcpplane's SDK-neutral mirror) — likewise copied.
//
// Anything else is an adapter gap and returns a *ProfileAdapterError.
func AdaptHostProfile(p any) (HostProfile, error) {
	switch {
	case p == nil:
		return HostProfile{}, nil
	default:
		if c, ok := p.(catalogmcpForgeCarrier); ok {
			// Covers any mcpforge.FeatureCarrier, including the zero-config
			// HostProfile and catalogmcp.MCPProfile / ProfileFromHas shapes.
			if isNilCarrier(p) {
				// A typed-nil carrier (an interface holding a nil pointer/
				// map/... implementing FeatureSet) reads as intentionally
				// profile-less, matching the nil *canimcp.Profile and
				// *model.Profile branches. Calling FeatureSet() on a nil
				// receiver whose implementation dereferences it would panic.
				return HostProfile{}, nil
			}
			if hp, ok := p.(HostProfile); ok {
				return hp.CloneFeatures(), nil
			}
			fs := c.FeatureSet()
			if fs == nil {
				return HostProfile{}, nil
			}
			return HostProfile{Features: fs.Clone()}, nil
		}
		if prof, ok := p.(*canimcp.Profile); ok {
			if prof == nil {
				return HostProfile{}, nil
			}
			return HostProfile{
				Features:  forgeFeatures(prof.Features),
				Transport: prof.Transport,
				Host:      prof.HostType,
			}, nil
		}
		if prof, ok := p.(*model.Profile); ok {
			if prof == nil {
				return HostProfile{}, nil
			}
			return HostProfile{
				Features:  forgeFeatures(modelFeatures(prof)),
				Transport: canimcp.TransportKind(prof.Transport),
				Host:      canimcp.HostType(prof.HostType),
				Hosted:    prof.Hosted,
			}, nil
		}
		return HostProfile{}, &ProfileAdapterError{ProfileType: fmt.Sprintf("%T", p)}
	}
}

// catalogmcpForgeCarrier duplicates the catalogmcp.ForgeFeatureCarrier
// method set locally so this package does not have to name its interface type
// in an import-heavy position. (The import of catalogmcp itself is allowed,
// but the Adapters below must accept any implementation, not just
// catalogmcp's aliasing shape.)
type catalogmcpForgeCarrier interface {
	FeatureSet() mcpforge.FeatureSet
}

// isNilCarrier reports whether p is a nil interface or an interface holding
// a typed nil (nil pointer/map/... value). A plain `p == nil` comparison
// misses the typed-nil shape: an interface variable carrying a nil concrete
// value still matches the catalogmcpForgeCarrier assertion but the carrier
// method may be unsafe on a nil receiver (mirrors isNilCatalog's handling of
// typed-nil catalogs).
func isNilCarrier(p any) bool {
	return isNilValue(p)
}

// profileFromRequest extracts the HostProfile for a per-request tool
// invocation. It adapts the *model.Profile the per-request Caps carry; a nil
// Caps or nil Profile (tests invoking handlers directly) falls back to the
// transport-generic stdio profile.
// hosted/surface overlays are the caller's (Config-owned) instance state and
// are applied separately by the assembled handlers.
func profileFromRequest(req model.ToolRequest) HostProfile {
	if req.Caps != nil && req.Caps.Profile != nil {
		if hp, err := AdaptHostProfile(req.Caps.Profile); err == nil {
			return hp
		}
		// An in-request adaptation gap cannot error the tool call; mirror
		// catalogmcp's resolvers and fall back to the generic stdio profile.
		return profileForTransport(canimcp.TransportStdio)
	}
	return profileForTransport(canimcp.TransportStdio)
}

// --- feature-set conversions ---

// forgeFeatures converts a canimcp.FeatureSet into an mcpforge.FeatureSet.
// The wire strings are identical, so this is a pure map transcription.
func forgeFeatures(fs canimcp.FeatureSet) mcpforge.FeatureSet {
	if fs == nil {
		return nil
	}
	out := make(mcpforge.FeatureSet, len(fs))
	for f, on := range fs {
		out[forgeFeatureOf(string(f))] = on
	}
	return out
}

// modelFeatures flattens an mcpplane model.Profile's FeatureSet into the
// canimcp string vocabulary for subsequent conversion.
func modelFeatures(p *model.Profile) canimcp.FeatureSet {
	if p == nil {
		return nil
	}
	out := make(canimcp.FeatureSet, len(p.Features))
	for f, on := range p.Features {
		out[canimcp.Feature(string(f))] = on
	}
	return out
}

// forgeFeatureOf converts a shared feature string into the package's
// mcpforge.Feature constant when one is declared, echoing the raw string
// otherwise — so an unknown feature never silently aliases a different gate.
func forgeFeatureOf(name string) mcpforge.Feature {
	switch name {
	case string(FeatFileHostInput):
		return FeatFileHostInput
	case string(FeatSourcePath):
		return FeatSourcePath
	case string(FeatSourceMint):
		return FeatSourceMint
	case string(FeatSourceURL):
		return FeatSourceURL
	case string(FeatSourceData):
		return FeatSourceData
	case string(FeatXMcpFile):
		return FeatXMcpFile
	case string(FeatSinkLocal):
		return FeatSinkLocal
	case string(FeatSinkDrop):
		return FeatSinkDrop
	case string(FeatMCPApps):
		return FeatMCPApps
	case string(FeatElicitation):
		return FeatElicitation
	case string(FeatRemoteAccess):
		return FeatRemoteAccess
	case string(FeatCoLocated):
		return FeatCoLocated
	default:
		return mcpforge.Feature(name)
	}
}

// modelFeatureSet converts the package feature vocabulary into the
// mcpplane model.FeatureSet the transfer selectors expect
// (transport.SourceModeEnumFromFeatures / TransportKindFromFeatures).
// The wire strings are identical, so this is a pure map transcription.
func modelFeatureSet(fs mcpforge.FeatureSet) model.FeatureSet {
	if fs == nil {
		return nil
	}
	out := make(model.FeatureSet, len(fs))
	for f, on := range fs {
		out[model.Feature(string(f))] = on
	}
	return out
}
