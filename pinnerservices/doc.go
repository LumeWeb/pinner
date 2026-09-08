// Package pinnerservices manages a configured process as an OS service
// (daemon) on the host's init system. It is a thin lifecycle adapter: it
// installs, starts, stops, and reports the status of a service that runs an
// external executable, without ever running the process in-process (no
// supervisor contract).
//
// The design is inspired by github.com/kardianos/service (Daniel Theophanes,
// zlib license). We borrow its shape — a per-platform System with a
// Detect/New probe and an ordered registry so a single New() picks the right
// backend — and its per-OS file split (systemd on Linux, launchd on macOS,
// Windows SCM), but reimplement it minimal and self-contained rather than
// depending on the library. It was extracted from pinner-cli's
// internal/service package; its behavior is intentionally unchanged.
//
// The package knows nothing about CLI frameworks, terminal rendering, or MCP:
// it exposes only the generic lifecycle surface (Service, System, Config,
// Status, Environment). Callers assemble the Config (executable path,
// arguments, env file) and wrap it in whatever product flow they have.
//
// Platform support:
//
//   - Linux: systemd per-user units (systemctl --user, Config.UserMode).
//   - macOS: launchd user LaunchAgents (launchctl load/unload).
//   - Windows: the Service Control Manager (system services).
//
// A minimal install-and-start flow looks like:
//
//	svc, err := pinnerservices.New(pinnerservices.Config{
//		Name:        "pinner-mcp",
//		Description: "Pinner MCP service",
//		ExecPath:    "/usr/local/bin/pinner",
//		Arguments:   []string{"mcp", "serve"},
//		UserMode:    true,
//		EnvFile:     "/home/alice/.config/pinner/mcp.env",
//	})
//	if err != nil { ... } // no init system detected
//	if err := svc.Install(ctx); err != nil { ... }
//	if err := svc.Start(ctx); err != nil { ... }
package pinnerservices
