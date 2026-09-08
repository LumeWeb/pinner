// Package pinner is the shared Pinner operation core ("pinnerops"): a typed,
// frontend-neutral operation registry plus the Pinner domain operation
// providers, extracted from pinner-cli.
//
// # Operation registry (root package)
//
// The root package holds the operation-descriptor model and registry that
// every frontend (CLI, MCP, desktop/server/mobile) derives from. It is an
// in-memory Go registry of Operation descriptors, not a wire format. Each
// Operation declares its Safety, Interaction, Visibility and typed
// OperationArg inputs, plus a Handler that runs against the host
// application's services. Invoke enforces the declared policy and
// NormalizeOperationInput coerces raw input into the declared shapes, so
// every surface delivers identical, correctly-typed input. Frontend compilers
// project the registry: Compile (or NewMCPCompiler /
// NewMCPCompilerForProfile) maps a Catalog to frontend-neutral
// []ToolDescriptor. Terminal (CLI) and MCP-server bindings stay out of this
// module; consumers wire the descriptors into their own surfaces.
//
// # Domain operations (catalogops)
//
// The catalogops subpackage declares the Pinner domain operation set —
// account, admin (billing, domains, quota, social providers, websites),
// apikeys, auth, dns, ens, ipns, pins, vault and websites — as catalog
// operations backed by the core service layer under core/. It defines those
// operations against the frontend-clean go.lumeweb.com/opmesh model; the
// frontend presentation metadata (MCP tool targets/descriptions, Environment
// surface carve-outs, per-arg agent/CLI metadata) lives on the catalogmcp and
// catalogmeta boundary packages, keyed by the operations' stable IDs.
//
// # Core services (core/)
//
// core/ holds the service implementations the operations run against
// (auth, config, dns, ipns, pinning, vault, websites, ...). It depends on
// go.lumeweb.com/portal-sdk and go.lumeweb.com/ipfs-sdk, never on any
// terminal-UI or CLI framework.
//
// # Example
//
// Declare a domain operation set, register it, and invoke an operation the
// way any frontend would:
//
//	cat := opmesh.NewCatalog()
//	ops := catalogops.AccountOperations(catalogops.AccountDeps{...})
//	for _, op := range ops {
//		_ = cat.Add(op)
//	}
//	input, err := opmesh.NormalizeOperationInput(ops[0],
//	    map[string]any{"limit": 10})
//	result, err := cat.Invoke(ctx, ops[0].Name(), input, opmesh.ActorModel)
package pinner
