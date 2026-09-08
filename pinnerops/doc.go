// Package pinnerops is the Pinner operation ASSEMBLY layer: it takes the
// domain operation providers (catalogops), a dependency graph
// (CatalogDepsBundle), and a product surface (Surface) and produces one
// runnable pinner.Catalog.
//
// Assembly, not execution: the root pinner package owns the operation model
// (Operation descriptors, the Catalog registry, Invoke policy enforcement) and
// catalogops owns the per-domain operation providers. pinnerops is the glue
// that decides WHICH of those operations a given deployment gets:
//
//   - Surface gates whole operation domains (account, vault, DNS, ...) in or
//     out of a server's catalog; the zero value is the full surface.
//   - The explicit hosted flag drops operations whose Environment is
//     EnvCLIOnly or EnvLocalOnly from a hosted (Portal-embedded) assembly,
//     without ever being inferred from the surface or credential seam.
//   - CatalogDepsBundle carries the concrete, lazy per-invocation dependency
//     graph every provider gets.
//
// The package is deliberately frontend-neutral: it imports only stdlib,
// samber/lo, and in-module packages (pinner, catalogops, core/config). It
// must never import pterm, urfave/cli, MCP SDKs, or any frontend package —
// the CLI and MCP compilers that consume the assembled catalog stay in their
// own product modules.
package pinnerops
