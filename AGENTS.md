# AGENTS.md

This file provides development guidelines and architectural documentation for
the pinner project.

## Common Commands

### Building
```bash
# Build all packages
go build -v ./...
```

### Testing
```bash
# Run all tests with race detection and coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Run tests for the root package only
go test -v -race .

# View coverage report
go tool cover -func=coverage.out
```

### Mock Generation

```bash
# Generate mocks for interfaces (uses .mockery.yaml; mockery is
# pre-installed at $HOME/go/bin/mockery — never reinstall it)
$HOME/go/bin/mockery
```

### Dependency Management
```bash
go mod download
go mod verify
go mod tidy
```

## Project Overview

pinner is the shared Pinner operation core: a typed, frontend-neutral operation
registry plus the Pinner domain operation set (pinnerops), extracted from
pinner-cli. Frontends (CLI, MCP servers, apps) declare operations once and
derive their command/tool surfaces from the same descriptors.

## Architecture

### Package Structure
- **Module path**: `go.lumeweb.com/pinner`
- **Root package** (`pinner`): the operation-descriptor model and registry —
  `Operation`, `OperationArg`, `Catalog`, `Invoke` policy enforcement, `Invoke`
  normalization (`NormalizeOperationInput`), accessors, paging, positional
  mapping, and the frontend-neutral `Compiler[ToolDescriptor]`. `doc.go` holds
  the package documentation; `errors.go` the sentinel errors.
- **`catalogops/`**: the Pinner domain operation providers — every operation is
  declared as data over the injected core services and never renders terminals
  or MCP itself.
- **`core/`**: the service layer the operations run against (`auth`, `config`,
  `dns`, `ipns`, `pinning`, `vault`, `websites`, ...). Generated mocks for the
  config manager live in `core/config/mocks`.
- **`dnsutil/`**: pure DNS string validators shared with consumers.

### Hard Boundaries (do not violate)
- **No `pterm`** — terminal rendering stays in pinner-cli.
- **No `urfave/cli`** — command compilation stays in pinner-cli
  (the CLI compiler was intentionally left behind there).
- **No MCP SDKs and no pinner-cli `internal/mcp` glue** — this module only
  emits frontend-neutral `ToolDescriptor`s and description DSL segments
  (via `go.lumeweb.com/mcpforge`). Host detection (pinner-cli's hostenv)
  stays out; consumers pass their own feature carrier
  (`pinner` accepts any `mcpforge.FeatureCarrier`).
- **No pinner-cli imports** — this module is upstream and standalone;
  pinner-cli depends on it, never the reverse.

### Design
- Operations are plain data (`pinner.OperationSpec`) plus a `Handler` executed
  against injected per-invocation service getters; services are resolved at
  request time so live config/auth-token changes are honored without restart.
- `Invoke` enforces declared `Safety` / `Interaction` / `Visibility` policy via
  `Actor`; callers gate on the sentinel errors in `errors.go` (`errors.Is`).
- Error message strings are behavior: tests pin them — do not reword.

### Testing Conventions
- Characterization tests moved from pinner-cli pin existing behavior; when
  changing behavior, update tests deliberately, never silently.
- Tests never touch the network or the real Portal; services are faked behind
  their interfaces (hand fakes + generated mocks under `core/config/mocks`).
- Do not add a README/board copyright header to source files; attribution
  lives only in the LICENSE file.
