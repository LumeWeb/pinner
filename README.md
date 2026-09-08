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
- **`core/`** — the service layer the operations run against (auth, config,
  dns, ipns, pinning, vault, websites, ...), built on
  [portal-sdk](https://pkg.go.dev/go.lumeweb.com/portal-sdk) and
  [ipfs-sdk](https://pkg.go.dev/go.lumeweb.com/ipfs-sdk).
- **`dnsutil`** — shared, dependency-free DNS validators.

The module carries **no terminal-UI, CLI-framework, or MCP-SDK imports**:
pterm/urfave rendering and the MCP-server bindings stay in pinner-cli, which
consumes this module and compiles the shared descriptors into its own
surfaces.

## Usage

```go
import (
    "go.lumeweb.com/pinner"
    "go.lumeweb.com/pinner/catalogops"
    account "go.lumeweb.com/portal-sdk"
)

cat := pinner.NewCatalog()

// Declare (or receive) the domain operations with their service deps.
// Deps are lazy getters resolved per invocation, so live config changes
// (including auth-token edits) are honored without restart:
ops := catalogops.AccountOperations(catalogops.AccountDeps{
    CfgMgr: func() config.Manager { return cfgMgr },
    AuthService: func(cfgMgr config.Manager, token string) auth.AuthService {
        return auth.NewService(cfgMgr, token)
    },
})
for _, op := range ops {
    if err := cat.Add(op); err != nil {
        panic(err)
    }
}

// Normalize raw input exactly the way every frontend surface does:
input, err := pinner.NormalizeOperationInput(ops[0], map[string]any{"limit": 10})

// Invoke with policy enforcement against the acting actor:
result, err := cat.Invoke(ctx, ops[0].Name(), input, pinner.ActorModel)

// Project to a frontend-neutral tool surface:
tools, err := pinner.NewMCPCompiler().Compile(cat)
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
