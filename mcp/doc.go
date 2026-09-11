// Package mcp is the MCP presentation and assembly layer for the Pinner
// operation set: the Pinner prompts, resources, direct tool names,
// capabilities, agent guide, and transfer-tool (upload_file / upload_data /
// download_file) descriptors, plus the compiled catalog-surface projection.
//
// It sits ABOVE the seeded description engine (catalogmcp) and adds
// everything catalogmcp excludes: prompts, resources, direct names,
// capabilities, the agent guide, and transfer tool descriptors. It depends on
// go.lumeweb.com/pinner/{assembly, catalogmcp, catalogmeta, catalogops,
// transfer} plus the plane libraries (mcpplane, canimcp, mcpforge, opmesh) —
// never a terminal-UI library or an MCP SDK.
//
// No package globals: every piece of instance state — the domain scope, the
// hosted flag, the platform profile, the transfer wiring, the resource
// providers — is carried on an assembled Server through its Config, and no
// package-global mutator exists.
//
// Assemble is the single construction seam: it accepts explicit dependencies
// (a CatalogDepsBundle or a pre-assembled opmesh.Catalog). Server
// orchestration (protocol wiring, dispatch, transports) stays with the
// composition root.
package mcp
