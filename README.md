# pinner

`go.lumeweb.com/pinner` is the shared Pinner operation core (the "pinnerops"
role), extracted from [pinner-cli](https://github.com/LumeWeb/pinner-cli).

It contains:

- **Root package `pinner`** — the typed, frontend-neutral operation registry:
  `Operation` descriptors (`Safety`, `Interaction`, `Visibility`, typed
  `OperationArg`s, `Handler`), the concurrent `Catalog` registry, policy
  enforcement (`Invoke`), input normalization (`NormalizeOperationInput`),
  typed accessors, paging helpers and positional-arg mapping, plus a
  frontend-neutral compiler that projects a `Catalog` to `[]ToolDescriptor`.
- **`catalogops`** — the Pinner domain operation set (account, admin billing /
  domains / quota / social providers / websites, apikeys, auth, dns, ens,
  ipns, operations, pins, vault, websites) declared directly against the
  frontend-clean [go.lumeweb.com/opmesh](https://pkg.go.dev/go.lumeweb.com/opmesh)
  operation model (`opmesh.Operation`/`OperationSpec`). The core definitions
  carry only the frontend-neutral vocabulary; frontend presentation metadata
  lives on the boundary packages keyed by these operations' stable IDs.
- **`pinnerops`** — the operation ASSEMBLY layer: `AssembleCatalogOps` takes a
  `CatalogDepsBundle` (the lazy per-invocation dependency graph wired by the
  product's construction layer), a `Surface` (which operation domains the
  deployment registers — the zero value is the full surface; `HostedSurface`
  excludes the Sia vault and portal admin), and an explicit `hosted` flag that
  drops `EnvCLIOnly`/`EnvLocalOnly` operations from a Portal-embedded assembly —
  and produces one runnable `opmesh.Catalog`. Also carries the frontend-free
  `CredentialResolver` seam (with the config-backed `ConfigCredentialResolver`).
- **`converge`** — the opmesh convergence seam (now a projection bridge): a
  total, panic-free projection of the root package's (frontend-ful)
  `pinner.Operation` metadata onto the frontend-clean operation model of
  [go.lumeweb.com/opmesh](https://pkg.go.dev/go.lumeweb.com/opmesh), for
  consumers that still assemble from the root model. Since the opmesh
  convergence, catalogops defines its operations directly against opmesh and
  `pinnerops` assembles `opmesh.Catalog`s natively, so no core-path content
  flows through this seam anymore.
- **`catalogmeta`** — the stdlib-only frontend-metadata boundary: the
  `Environment` surface carve-outs (`EnvironmentOf`) and per-argument frontend
  metadata (`AgentHelp`, `AgentOnly`, `PositionalOnly`, `Sources`) that used to
  live inline on the operation definitions, keyed by stable operation ID
  (`ArgFrontendFor`).
- **`catalogmcp`** — the MCP boundary: the per-profile MCP tool target
  machinery (`Target`, `MCPTargets`, `TargetFor`, `Fallback`, `Hidden`,
  `FallbackFunc`, `TargetsOf`), the feature-gated description DSL
  (`FeatFileHostInput` and the `MCPProfile`/`ProfileFromHas`/`AdaptProfile`
  adapter), and a model-surface compiler (`NewCompilerForProfile`) mapping an
  `opmesh.Catalog` to `[]opmesh.ToolDescriptor` with target-resolved
  descriptions. This is the only in-module home of the
  [mcpforge](https://pkg.go.dev/go.lumeweb.com/mcpforge) dependency.
- **`transfer`** — the Pinner/IPFS upload & download EXECUTORS over the
  transport-neutral coordination layer of
  [go.lumeweb.com/mcpplane/transfer](https://pkg.go.dev/go.lumeweb.com/mcpplane/transfer)
  (`transfer.UploadHandler` / `IPFSDownloadHandler` contracts): the
  stream→upload executor (`StreamUpload` — temp-file buffering, HTML wrap-name
  sniffing, archive-convert extraction with an aggregate size cap over
  `core/uploads.Service`), the IPFS download executor (`StreamDownload` over
  `core/download.Service`), and the download-sink layer (`ExecuteLocalSink`
  with root-confined atomic local writes, `ExecuteDropSink` with pre-buffered
  one-time GET filedrops, `ResolveLocalOutputPath` containment), plus the
  archive toolkit (`ArchiveMode`/`SniffArchive`/`OpenArchiveFS`/`CheckTreeSize`)
  over [go.lumeweb.com/ipfs-content](https://pkg.go.dev/go.lumeweb.com/ipfs-content).
  It imports no CLI formatter, command framework, or MCP SDK package — tool
  descriptors and vault handling stay in pinner-cli per the package-boundaries
  doc.
- **`pinnermcp`** — the Pinner MCP PRESENTATION layer over the kernels above:
  `Assemble(Config) (*Server, error)` builds the complete, instance-scoped MCP
  presentation from a `Config` (surface, hosted flag, platform profile,
  transfer wiring, resource providers — no package globals, per fitness rule
  13): the catalog tool surface compiled via
  `catalogmcp.NewCompilerForProfile` and projected onto `model.ToolDescriptor`
  with safety-derived wire hints, curated stamping, and environment
  carve-out skips; the curated tools/list names (`CuratedToolNames`); the
  surface-gated prompt set (the embedded `prompttemplates/` —
  `website-onboarding`, `website-update`, `setup`, `ens-publish`); the
  `pinner://` resource descriptors with injected `ResourceProviders` (account,
  vault, DNS/website wizard status); the honest capabilities report
  (`CurrentCapabilities` + `NewCapabilitiesDescriptor`); the host-aware agent
  guide (`BuildAgentGuide` / `AgentGuideDescriptor`); and the transfer-tool
  descriptor halves for `upload_file` / `upload_data` / `download_file`
  (`NewUploadFileDescriptor` / `DataURIUploadDescriptor` /
  `NewDownloadFileDescriptor`) whose executor halves are injected as function
  types via `TransferDeps` (the coordinators stay with the composition root).
  Server orchestration (protocol wiring, dispatch, transports) stays with the
  composition root, which dispatches through `Server.Catalog().Invoke`. The
  package imports no CLI framework, terminal-UI library, or MCP SDK.
- **`core/`** — the service layer the operations run against (auth, config,
  dns, ipns, uploads, download, pinning, vault, websites, ...), built on
  [portal-sdk](https://pkg.go.dev/go.lumeweb.com/portal-sdk) and
  [ipfs-sdk](https://pkg.go.dev/go.lumeweb.com/ipfs-sdk).
- **`dnsutil`** — shared, dependency-free DNS validators.
- **`pinnerservices`** — the OS service-management machinery: install, start,
  stop, and status-report a configured process as an OS service (daemon).
  Thin lifecycle adapters over systemd per-user units (Linux), launchd
  LaunchAgents (macOS), and the Windows Service Control Manager (Windows),
  selected automatically by probe; plus the 0600 KEY=VALUE service
  environment-file helpers all backends share. It never runs the process
  in-process — it always points at an external executable.

The module carries **no terminal-UI, CLI-framework, or MCP-SDK imports**:
pterm/urfave rendering and the MCP-server bindings stay in pinner-cli, which
consumes this module and compiles the shared descriptors into its own
surfaces.

## Usage

```go
import (
    "context"

    "go.lumeweb.com/opmesh"
    "go.lumeweb.com/pinner/catalogmcp"
    "go.lumeweb.com/pinner/catalogops"
    "go.lumeweb.com/pinner/core/auth"
    "go.lumeweb.com/pinner/core/config"
)

var cfgMgr config.Manager // your configmanager-backed Manager

cat := opmesh.NewCatalog()

// Declare (or receive) the domain operations with their service deps.
// Deps are lazy getters resolved per invocation, so live config changes
// (including auth-token edits) are honored without restart:
ops := catalogops.AccountOperations(catalogops.AccountDeps{
    CfgMgr: func() config.Manager { return cfgMgr },
    AuthService: func(m config.Manager, token string) auth.AuthService {
        return auth.NewAuthService(m, "https://api.example.com", nil)
    },
})
for _, op := range ops {
    if err := cat.Add(op); err != nil {
        panic(err)
    }
}

// Normalize raw input exactly the way every frontend surface does:
input, err := opmesh.NormalizeOperationInput(ops[0], map[string]any{})
if err != nil {
    panic(err)
}

// Invoke with policy enforcement against the acting actor:
if _, err := cat.Invoke(context.Background(), ops[0].Name(), input, opmesh.ActorModel); err != nil {
    panic(err)
}

// Project to a frontend-neutral tool surface:
tools, err := catalogmcp.NewCompiler().Compile(cat)
if err != nil {
    panic(err)
}
_ = tools // hand []opmesh.ToolDescriptor to your MCP server
```

Install and run a built binary as an OS service (backend picked automatically
per platform — systemd user unit, launchd LaunchAgent, or Windows SCM):

```go
import "go.lumeweb.com/pinner/pinnerservices"

svc, err := pinnerservices.New(pinnerservices.Config{
	Name:        "pinner-mcp",
	Description: "Pinner MCP service",
	ExecPath:    "/usr/local/bin/pinner",
	Arguments:   []string{"mcp", "serve"},
	UserMode:    true,
	EnvFile:     "/home/alice/.config/pinner/mcp.env",
})
if err != nil {
	// no supported init system on this host
	return err
}
if err := svc.Install(ctx); err != nil {
	return err // registers and enables the service
}
return svc.Start(ctx)
```

Assemble the whole catalogops surface into one runnable catalog for a chosen
product surface (hosted assemblies drop `EnvCLIOnly`/`EnvLocalOnly` operations
explicitly — never inferred from the surface or the credential seam):

```go
import "go.lumeweb.com/pinner/pinnerops"

bundle := &pinnerops.CatalogDepsBundle{
    CfgMgr:             func() config.Manager { return cfgMgr },
    CredentialResolver: pinnerops.ConfigCredentialResolver{
        AuthToken: func() (string, error) { return cfgAuthToken(), nil },
    },
    Auth:    catalogops.AuthDeps{CfgMgr: func() config.Manager { return cfgMgr }},
    Account: catalogops.AccountDeps{CfgMgr: func() config.Manager { return cfgMgr }},
    // ... remaining domains: leave nil to degrade to "service unavailable" ops
}

// Local/CLI assembly over the FULL surface; hosted (Portal-embedded) would use
// pinnerops.HostedSurface and hosted=true:
cat, err := pinnerops.AssembleCatalogOps(bundle, pinnerops.FullSurface, false)
if err != nil {
    panic(err)
}
```

Project the assembled (or any) operations onto the frontend-clean
[go.lumeweb.com/opmesh](https://pkg.go.dev/go.lumeweb.com/opmesh) operation
model — register, discover, normalize, and dispatch them through an
`opmesh.Catalog` while the CLI/MCP metadata stays behind in this module:

```go
import (
    "go.lumeweb.com/opmesh"
    "go.lumeweb.com/pinner/converge"
)

ops := catalogops.OperationsOperations(catalogops.OperationsDeps{Service: svc})
opCat := opmesh.NewCatalog()
if err := converge.RegisterAll(opCat, ops...); err != nil {
    return err // projected under the same stable operation IDs
}

// discovery against the opmesh model (same titles, inputs, effect classes)
if desc, ok := opCat.Describe("operations_list", opmesh.ActorModel); ok {
    _ = desc // opmesh.ToolDescriptor with input schema
}
```

Wire the Pinner/IPFS transfer executors into an mcpplane/transfer server (or
any consumer that programs against the `transfer.UploadHandler` /
`IPFSDownloadHandler` contracts). A concrete, compiling sketch:

```go
import (
    "context"
    "strings"

    "go.lumeweb.com/pinner/core/uploads"
    "go.lumeweb.com/pinner/transfer"
)

var uploadSvc uploads.Service // your concrete SDK-backed implementation

// An archive/website upload through the executor:
upload := transfer.StreamUpload(uploadSvc, 1<<30) // 1 GiB cap
result, err := upload(context.Background(), strings.NewReader("<h1>hi</h1>"), 13, "", true, string(transfer.ArchiveConvert), true)
if err != nil {
    return err
}
_ = result.(*uploads.UploadResult) // the uploaded CID / size / duration
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
