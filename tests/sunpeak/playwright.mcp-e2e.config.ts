import { defineConfig } from 'sunpeak/test/config';
import { join } from 'path';
import { fileURLToPath } from 'url';

const __dirname = fileURLToPath(new URL('.', import.meta.url));

// Sunpeak MCP e2e suite driving REAL tool calls through pinner's own harness
// (cmd/mcpharness). The harness fakes every core service in-process and seeds a
// deterministic account/token (`--seed-email e2e@example.com` -> token
// token-e2e@example.com), so `account_info` returns real account data rather
// than "authentication required" — no separate fake API, no network.
//
// NOTE: this suite's seed-parity with the harness's in-process fakes (e.g. an
// empty initial pins store) is a follow-up still being reconciled; it is NOT
// wired into CI yet.
//
// Build the harness first:
//   go build -o ../../bin/mcpharness ./cmd/mcpharness
//
// Run: npx sunpeak test -c playwright.mcp-e2e.config.ts — or:
//   pnpm test:mcp-e2e

const FAKE_PORT = 8126;
const FIXTURE_HOME = join(__dirname, 'fixtures', 'pinner-home');

// Determinism, not speed, is the point of this suite. Every host worker talks
// to ONE shared fake-API server (127.0.0.1:8126) whose account store is
// mutable (account_update_email / account_update_password temporarily rewrite
// the shared seeded account). sunpeak's default runs `workers: 2` locally —
// two parallel workers then race against that single shared account and flake
// (a worker reading account_info can observe another worker's in-flight email
// mutation). CI already forces `workers: 1` via SUNPEAK/C.I. env; pin it here
// so LOCAL runs are deterministic and identical to CI. The suite is a 15s
// stdio-protocol run, so serializing it costs nothing.
export default {
  ...defineConfig({
    server: {
      command: '../../bin/mcpharness',
      args: ['--seed-email', 'e2e@example.com'],
    },
    testDir: 'mcp-e2e',
    timeout: 120_000,
  }),
  workers: 1,
  fullyParallel: false,
};
