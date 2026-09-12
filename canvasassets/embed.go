package canvasassets

// This package transitively embeds the MCP App JS bundles, the generated
// mcpcanvas manifest, and the compiled Tailwind stylesheet (via Assets /
// ThemeCSS above), so `go generate ./canvasassets` regenerates them before any
// `go build`/`go test`. go:generate runs with this package as its working
// directory; `..` is the repo root where the Makefile lives. The `canvasassets`
// target installs the templ CLI, regenerates the templ files, builds the app
// bundles, generates the manifest, and compiles the CSS.
//go:generate make -C .. canvasassets
