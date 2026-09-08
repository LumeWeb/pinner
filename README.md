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
  ipns, operations, pins, vault, websites) declared as catalog operations.
- **`pinnerops`** — the operation ASSEMBLY layer: `AssembleCatalogOps` takes a
  `CatalogDepsBundle` (the lazy per-invocation dependency graph wired by the
  product's construction layer), a `Surface` (which operation domains the
  deployment registers — the zero value is the full surface; `HostedSurface`
  excludes the Sia vault and portal admin), and an explicit `hosted` flag that
  drops `EnvCLIOnly`/`EnvLocalOnly` operations from a Portal-embedded assembly —
  and produces one runnable `pinner.Catalog`. Also carries the frontend-free
  `CredentialResolver` seam (with the config-backed `ConfigCredentialResolver`).
- **`converge`** — the opmesh convergence seam: a total, panic-free projection
  of this module's (frontend-ful) `pinner.Operation` metadata onto the
  frontend-clean operation model of
  [go.lumeweb.com/opmesh](https://pkg.go.dev/go.lumeweb.com/opmesh) — stable
  operation IDs, typed args with codecs/defaults/enums, read/mutate/destructive
  effect classification, actor interaction/visibility policy, and handler
  dispatch through a real `opmesh.Catalog`. Frontend-only metadata
  (`Environment`, `MCPTargets`/`Target`/`DescFunc`, arg `AgentHelp`/`AgentOnly`/
  `PositionalOnly`/`Sources`) is intentionally NOT projected — it stays behind
  for the CLI/MCP boundary adapters.
- **`core/`** — the service layer the operations run against (auth, config,
  dns, ipns, pinning, vault, websites, ...), built on
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

    "go.lumeweb.com/pinner"
    "go.lumeweb.com/pinner/catalogops"
    "go.lumeweb.com/pinner/core/auth"
    "go.lumeweb.com/pinner/core/config"
)

var cfgMgr config.Manager // your configmanager-backed Manager

cat := pinner.NewCatalog()

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
input, err := pinner.NormalizeOperationInput(ops[0], map[string]any{})
if err != nil {
    panic(err)
}

// Invoke with policy enforcement against the acting actor:
if _, err := cat.Invoke(context.Background(), ops[0].Name(), input, pinner.ActorModel); err != nil {
    panic(err)
}

// Project to a frontend-neutral tool surface:
tools, err := pinner.NewMCPCompiler().Compile(cat)
if err != nil {
    panic(err)
}
_ = tools // hand []pinner.ToolDescriptor to your MCP server
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

## License

Apache-2.0 — see [LICENSE](LICENSE).
