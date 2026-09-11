package hosted

import "go.lumeweb.com/pinner/assembly"

// HostedDomainScope is the standard hosted surface: account/subscription plus
// the full IPFS/websites/DNS/IPNS/ENS/operations set (no Sia vault, no portal
// admin). It aliases assembly.HostedDomainScope — the single source of truth
// for the hosted domain surface — so a hosted construction consumer names the
// surface from the package that assembles the server while the shape stays
// owned by the catalog-assembly layer.
var HostedDomainScope = assembly.HostedDomainScope

// DomainScope is the hosted surface type consumed by Config. It is the
// assembly.DomainScope shape; a zero value is the full surface, and hosted
// deployments should opt in explicitly via HostedDomainScope.
type DomainScope = assembly.DomainScope
