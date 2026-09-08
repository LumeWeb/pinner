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

- new `pinnermcp` subpackage: the Pinner MCP PRESENTATION layer over
  `pinnerops` + `catalogmcp` (prompts, resources, curated tool names,
  capabilities, agent guide, transfer-tool descriptors), dependent only on
  the plane libraries (mcpplane, canimcp, mcpforge, opmesh) and this module —
  no CLI framework, terminal-UI library, or MCP SDK imports. `Assemble(Config)`
  is the single construction seam producing an instance-scoped `Server`
  (fitness rule 13: surface, hosted flag, platform profile, transfer wiring,
  and resource providers are all Config fields; the historic pinner-cli
  package-global setters are superseded, with no equivalent setters). It
  compiles the catalog tool surface via `catalogmcp.NewCompilerForProfile`
  and projects it to `model.ToolDescriptor` presentations (safety-derived wire
  hints, curated stamping via `CuratedToolNames`,
  `catalogmeta.EnvironmentOf` carve-out skips), plus the direct-only tools
  outside the catalog: agent_guide (`BuildAgentGuide`/`AgentGuideDescriptor`
  with host-profile fragments), the honest capabilities report
  (`CurrentCapabilities`/`NewCapabilitiesDescriptor` matched to the tools/list
  registration flags), and the transfer-tool descriptor halves
  (`NewUploadFileDescriptor`/`DataURIUploadDescriptor`/
  `NewDownloadFileDescriptor`) with executor/coordinator function types
  injected via `TransferDeps`. The surface-gated prompt set renders the
  embedded `prompttemplates/` (`website-onboarding`, `website-update`,
  `setup`, `ens-publish`) and the `pinner://` resources flow through the
  injected `ResourceProviders` (zero value = descriptors with a clear
  "provider not configured" read failure, matching the source's nil-provider
  handling). There is deliberately no AppRegistry field: mcpplane.apps sits
  behind the MCP SDK, so the Apps registry remains a composition-root seam.
  Server orchestration (protocol wiring, dispatch, transports) stays with the
  composition root, which dispatches through `Server.Catalog().Invoke`.
- new `pinnertransfer` subpackage: the Pinner/IPFS upload & download
  EXECUTORS over [go.lumeweb.com/mcpplane/transfer](https://pkg.go.dev/go.lumeweb.com/mcpplane/transfer)
  (per the package-boundaries doc §4) — the stream→upload executor
  (`StreamUpload` over a new `core/uploads.Service`: temp-file buffering,
  HTML wrap-name sniffing, archive-convert extraction with an aggregate
  tree-size cap via the ipfs-content archive toolkit), the IPFS download
  executor (`StreamDownload` over `core/download.Service`), and the
  download-sink execution layer (`ExecuteLocalSink`/`ExecuteDropSink`,
  root-confined atomic local writes via `ResolveLocalOutputPath`/
  `WriteLocalDownload`, one-time GET filedrops with real-size pre-buffering,
  `DownloadResult`, `DownloadSinksAllowed`, `SinkDefaultName`,
  `ResolveDownloadRoot`), plus the archive port (`ArchiveMode`,
  `ParseArchiveMode`, `SniffArchive`, `OpenArchiveFS`, `CheckTreeSize`). The
  package imports only stdlib + mcpplane/transfer + ipfs-content + the
  module's own `core/{uploads,download,config}` — no pterm/urfave/MCP-SDK or
  pinner-cli imports. Tool descriptors (`upload_file`/`download_file`/
  `upload_data`) and the vault transfer stay in pinner-cli for future
  pinnermcp/CLI homes.
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
