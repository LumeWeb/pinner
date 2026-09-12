import { defineConfig } from '@playwright/test';

// Raw HTTP tests for the pinner MCP server harness OAuth transport
// (mcpharness --http --oauth). Plain Playwright (not the sunpeak fixtures)
// because the OAuth-protected /mcp endpoint requires a full interactive-
// capable handshake that the headless sunpeak inspector cannot complete; these
// tests drive the entire RFC 9728 flow over raw HTTP: discovery -> dynamic
// client registration -> authorization-code + PKCE exchange -> token ->
// protected /mcp call. The harness ships a minimal test-only authorization
// server (cmd/mcpharness/oauth.go) for exactly this suite.
//
// Server: `mcpharness --http --oauth` on 127.0.0.1:8124 (secret is the OAuth
// resource-owner password). Build first:
//   go build -o ../../bin/mcpharness ./cmd/mcpharness

const PORT = 8124;
const SECRET = process.env.PINNER_TEST_SECRET ?? 'sunpeak-test-secret';

export default defineConfig({
  testDir: 'oauth',
  timeout: 60_000,
  use: { trace: 'on-first-retry' },
  webServer: {
    command: `../../bin/mcpharness --http --oauth --host 127.0.0.1 --port ${PORT} --auth-token ${SECRET}`,
    url: `http://127.0.0.1:${PORT}/healthz`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
