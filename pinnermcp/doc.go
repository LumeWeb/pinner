// Package pinnermcp is the Pinner MCP PRESENTATION layer: Pinner prompts,
// resources, curated tool names, capabilities, the agent guide, and the
// transfer-tool (upload_file / upload_data / download_file) descriptors.
//
// Per 00-package-boundaries.md §4, pinnermcp sits ABOVE the seeded
// description engine (catalogmcp) and adds everything catalogmcp excludes:
// prompts, resources, curated names, capabilities, agent guide, and transfer
// tool descriptors. It depends on go.lumeweb.com/pinner/{pinnerops,
// catalogmcp, catalogmeta, catalogops, transfer} plus the plane
// libraries (mcpplane, canimcp, mcpforge, opmesh) — never a CLI framework, a
// terminal-UI library, or an MCP SDK.
//
// Fitness rule 13 (no package globals) applies with full force: every piece
// of instance state — the surface, the hosted flag, the platform profile,
// the transfer wiring, the resource providers — is carried on an assembled
// Server through its Config. The historic pinner-cli package-global setters
// (SetSurface/SetHosted/SetTransportFlags and friends) are deliberately
// superseded by Config fields.
//
// Assemble is the single construction seam: it accepts explicit dependencies
// (a CatalogDepsBundle or a pre-assembled opmesh.Catalog), never a
// *cli.Command, and never calls a CLI service factory. Server orchestration
// (protocol wiring, dispatch, transports) stays with the composition root.
package pinnermcp
