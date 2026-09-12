import { defineConfig } from 'sunpeak/test/config';

// Canonical sunpeak config for pinner's own MCP server harness: a
// self-contained tests/sunpeak/ suite auto-discovered by `sunpeak test`.
//
// Server: the in-module harness `cmd/mcpharness` over stdio. It stands up the
// complete pinner MCP surface (progressive-disclosure meta-tools, compiled
// catalog tools, direct tools, and ui:// MCP Apps backed by real canvasassets
// documents) over FAKED core services — no network, no external API. Build it
// first with `go build -o bin/mcpharness ./cmd/mcpharness`.
//
// The suite covers two fixture levels:
//   - `mcp`        protocol primitives (listTools / callTool / listResources)
//   - `inspector`  host-iframe rendering of the ui:// MCP Apps (real browser)
//
// The raw-HTTP transport suites live in tests/sunpeak/http and tests/sunpeak/oauth
// with their own plain-Playwright configs; they are OUTSIDE this testDir so
// `sunpeak test` runs only the stdio protocol + render suite.
export default defineConfig({
  server: {
    command: '../../bin/mcpharness',
    args: [],
  },
  timeout: 120_000,
  testDir: 'tests',
});
