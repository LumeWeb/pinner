package canvasassets

// This package transitively embeds the MCP App JS bundles, the generated
// mcpcanvas manifest, and the compiled Tailwind stylesheet (via Assets /
// ThemeCSS above).
//
// The embed inputs are NOT git-tracked (they are build-time outputs), so any
// build of this package must first ensure they exist at the location Go
// resolves canvasassets from. When compiling inside this module, `go generate
// ./canvasassets` (or `make assets`) regenerates them in the working tree.
// Consumers that build canvasassets from a pinned module version instead run
// scripts/ensure-canvasassets.sh — the shared DRY staging script that
// regenerates the same inputs in place inside the resolved checkout or module
// cache before `go build`. Both routes run the identical `make assets`
// pipeline (jsbuild + cssbuild + genappmanifest).
//go:generate make -C .. canvasassets
