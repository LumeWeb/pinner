// Package canvas owns the Pinner web UI view contracts: the stable,
// versioned set of view identifiers the Pinner MCP surface serves as ui://
// MCP App resources, together with each view's typed payload shapes, the
// tool invocations (actions) a view can trigger, and its client-side UI
// state vocabulary.
//
// The contracts integrate with the frontend runtime contract of
// go.lumeweb.com/mcpcanvas: mcpcanvas owns the host-rendered UI shell, the
// MCP Apps bridge, and the asset manifest that resolves a view ID to hashed
// bundle bytes, while this package owns the Pinner-specific nouns (which
// views exist and what data and verbs each one speaks). Both composition
// roots described by the Pinner extraction plan — the CLI's `mcp` surface
// and the hosted product — serve the same views through different asset
// adapters; neither may fork the screen set, the payload schemas, or the
// action/state vocabulary.
//
// The package is frontend-neutral and server-neutral. It consumes no Go
// server packages and knows nothing about CLI versus hosted routing: server
// packages provide data and actions, frontend packages own rendering.
//
// Rendering itself lives here as a thin seam over go.lumeweb.com/mcpcanvas,
// which owns the app-document shell and the verified-bundle module script.
// This package owns the Pinner-specific nouns — the view set, the screen
// bodies (templ markup), and the payload/action contracts — and injects
// asset delivery and theme CSS via composition roots (Renderer). It imports
// no server packages and no pinner-cli code.
//
// Every compatibility surface defined here — view IDs, payload field sets,
// and the action/state vocabulary — is versioned (ContractVersion) and
// receives contract tests: a screen added server-side without a contract,
// or a contract without a screen, fails the contract suite.
package canvas
