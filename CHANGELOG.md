# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Features

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
