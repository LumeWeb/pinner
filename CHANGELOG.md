# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Breaking changes

- **opmesh convergence — catalogops rewritten over `go.lumeweb.com/opmesh`**:
  every `catalogops` domain provider now returns `[]opmesh.Operation`
  (frontend-clean, defined directly against the opmesh model) instead of
  `[]pinner.Operation`, and `pinnerops.AssembleCatalogOps` now returns an
  `opmesh.Catalog` instead of a `pinner.Catalog`. Consumers that registered the
  domain providers into a `pinner.Catalog` must register into an
  `opmesh.Catalog` instead. Operation IDs, args, defaults, enums, selection
  groups, safety/interaction/visibility, actor+confirmation policy, and
  handler behavior are unchanged; the affected frontend metadata moved to the
  new boundary packages below (catalogmeta, catalogmcp).

### Features

- new `catalogmeta` subpackage (stdlib-only): the frontend-metadata boundary
  keyed by stable operation ID — `EnvironmentOf` for the surface carve-outs
  (`EnvBoth`/`EnvCLIOnly`/`EnvLocalOnly`/`EnvHostedOnly`, relocated verbatim
  from the former inline `pinner.Environment` declarations) and
  `ArgFrontendFor`/`ArgFrontendForArg` for the per-argument frontend metadata
  previously templated on the core definitions (`AgentHelp`, `AgentOnly`,
  `PositionalOnly`, `Sources`, preserved verbatim).
- new `catalogmcp` subpackage (the MCP boundary, the module's only
  `mcpforge` consumer): the relocated per-profile MCP tool machinery —
  `Target`/`MCPTargets`/`TargetFor`/`Fallback`/`Hidden`/`FallbackFunc`,
  `TargetsOf` (per-operation MCP targets extracted verbatim from the former
  inline declarations), the feature-gated description DSL
  (`FeatFileHostInput`, the websites_create `DescBuilder` description and its
  `MCPProfile`/`ForgeFeatureCarrier`/`ProfileFromHas`/`AdaptProfile`/
  `ProfileAdapterGap` adapter), and a model-surface compiler
  (`NewCompiler`/`NewCompilerForProfile`) that maps an `opmesh.Catalog` to
  `[]opmesh.ToolDescriptor` with target-resolved descriptions (matching the
  pre-migration root-pinner compiler contract).
- `converge` is repurposed as the projection bridge for the root-package
  bridging surface: catalogops no longer produces `pinner.Operation` values,
  so no core-path content flows through the seam; its projection contract,
  tests, and import-isolation boundary are unchanged.

- new `converge` subpackage: the FIRST STEP of opmesh convergence — a total,
  panic-free PROJECTION SEAM from this module's frontend-ful
  `pinner.Operation` model onto the frontend-clean operation model of
  [go.lumeweb.com/opmesh](https://go.lumeweb.com/opmesh), now a direct
  dependency (`Project`/`ProjectSpec`/`ProjectAll`/`ProjectArgs` plus the
  enum converters and `RegisterAll` into a real `opmesh.Catalog`). The
  projection carries exactly the opmesh-owned vocabulary — stable operation
  IDs, typed args (codecs, defaults, enums, flexible IDs, selection groups,
  raw schemas, sensitivity), read/mutate/destructive effect classification,
  actor interaction/visibility policy, and handler dispatch — and drops the
  frontend-only fields (`Environment`, `MCPTargets`/`Target`/`DescFunc`,
  arg `AgentHelp`/`AgentOnly`/`PositionalOnly`/`Sources`) for the CLI/MCP
  boundary adapters. No existing behavior changed; the root `pinner`,
  `catalogops`, `pinnerops`, and `pinnerservices` packages are untouched.

- initial extraction of the pinner operation set (pinnerops role) from
  pinner-cli: the typed operation-descriptor registry and the Pinner domain
  operation providers (account, admin, apikeys, auth, dns, ens, ipns, list,
  meta, operations, pins, vault, websites) plus their core service layer.
- new `pinnerops` subpackage: the Pinner operation ASSEMBLY layer extracted
  from pinner-cli's internal/mcp (catalogassembly.go, CatalogDepsBundle, and
  the frontend-free CredentialResolver seam). `AssembleCatalogOps(deps,
  surface, hosted)` builds one runnable `pinner.Catalog` from the catalogops
  domain providers, gated by a `Surface` (zero value = full surface;
  `FullSurface`/`HostedSurface` presets) and an explicit hosted flag that
  drops `EnvCLIOnly`/`EnvLocalOnly` operations in hosted mode. Behavior is
  unchanged from the original; no pterm/urfave/MCP/pinner-cli dependencies.
- new `pinnerservices` subpackage: the OS service-management machinery
  (thin install/start/stop/status/connect adapter over systemd user units,
  launchd LaunchAgents, and the Windows SCM) extracted from pinner-cli's
  `internal/service`. Includes the KEY=VALUE service environment file
  helpers (`ParseEnvironment`/`LoadEnvironment`/`WriteEnvironment`) and the
  backend-independent `Status` report. Behavior is unchanged from the
  original; no pterm/urfave/MCP/pinner-cli dependencies.
