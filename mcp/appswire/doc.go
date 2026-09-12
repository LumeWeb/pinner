// Package appswire is the shared composition-root seam for MCP Apps: the one
// declarative table of the Pinner app views (the ui:// interactive screens +
// their open_* launcher tools), the open_* launcher descriptor builder, and
// the generic view installer that both the CLI's local server assembly and a
// hosted (Portal-embedded) assembly iterate so a deployment cannot wire one
// surface's app inventory without the other deriving from the same facts.
//
// The package sits BEHIND the same boundary as the app registry itself: it
// imports go.lumeweb.com/mcpplane/apps and go.lumeweb.com/mcpplane/sdk (the
// registry and server types view registration physically needs). The core
// go.lumeweb.com/pinner/mcp package must never import appswire — that would
// pull the MCP SDK into every consumer of the descriptor model; composition
// roots (pinner-cli's internal/mcp, a portal MCP plugin) reference this
// package directly and echo results back into the SDK-free seams (the agent
// guide's Config.InstalledApps inventory, capability reports) as plain data.
//
// Views are declared once in All with a capability Requirements mask: a view
// needing out-of-band handoff coordinators or the local Sia vault surface names
// that capability, and a deployment selects the subset whose requirements its
// wiring covers AND whose dependencies it can actually supply. Capabilities
// are wiring facts, not deployment-kind facts — a hosted assembly that wires
// its own OOB browser-form coordinators gets the sign-in card with no table
// change, and a vault-enabling custom scope gets the vault views. Missing
// dependencies skip a view, never fail the assembly, and the installer
// returns the launcher names that DID install so guidance prose can enumerate
// only what exists.
package appswire
