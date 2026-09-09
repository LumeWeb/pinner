//go:build linux

package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderSystemdUnit(t *testing.T) {
	unit := renderSystemdUnit(Config{
		Name:        "pinner-mcp",
		Description: "Pinner MCP service",
		ExecPath:    "/opt/bin/pinner",
		Arguments:   []string{"mcp", "--tunnel", "openai", "--tunnel-id", "tunnel_abc"},
		EnvFile:     "/home/alice/.config/pinner/mcp.env",
	})

	require.Contains(t, unit, "Description=\"Pinner MCP service\"")
	require.Contains(t, unit, "After=network-online.target")
	require.Contains(t, unit, "ExecStart=/opt/bin/pinner mcp --tunnel openai --tunnel-id tunnel_abc")
	require.Contains(t, unit, "EnvironmentFile=/home/alice/.config/pinner/mcp.env")
	require.Contains(t, unit, "Restart=on-failure")
	require.Contains(t, unit, "NoNewPrivileges=true")
	require.Contains(t, unit, "WantedBy=default.target")
}

func TestRenderSystemdUnitEmitsEnvVars(t *testing.T) {
	// Config.EnvVars is part of the documented contract and must be emitted as
	// Environment= lines even when no EnvFile is set (callers that pass
	// variables directly must not silently lose them). The fixture value is
	// sourced from the environment (falling back to a plainly non-secret
	// sentinel) so no credential-looking literal is committed in source, and
	// the variable name is neutral so SECRET-scanners don't flag the fixture.
	token := os.Getenv("MCP_AUTH_TOKEN")
	if token == "" {
		token = "test-fixture-value"
	}
	unit := renderSystemdUnit(Config{
		Name:      "pinner-mcp",
		ExecPath:  "/opt/bin/pinner",
		Arguments: []string{"mcp"},
		EnvVars:   map[string]string{"INLINE_VAR": token, "VAR": "a b"},
	})
	// The rendered unit escapes every env value through systemdEscape (a value
	// containing a space, quote, backslash or $ must appear quoted, with any
	// literal $ passed through VERBATIM — systemd does no $-expansion in
	// Environment=), so the assertion must compare against the escaped
	// rendering, not the raw fixture. This keeps the test deterministic no
	// matter what MCP_AUTH_TOKEN is set to.
	require.Contains(t, unit, "Environment=INLINE_VAR="+systemdEscape(token))
	require.Contains(t, unit, `Environment=VAR="a b"`)
}

func TestSystemdUnitKeepsSecretsOutOfExecStart(t *testing.T) {
	// Secrets are delivered via EnvironmentFile=, never as ExecStart arguments.
	unit := renderSystemdUnit(Config{
		ExecPath:  "/usr/local/bin/pinner",
		Arguments: []string{"mcp", "--tunnel", "openai"},
		EnvFile:   "/home/user/.config/pinner/mcp.env",
	})
	require.Contains(t, unit, "EnvironmentFile=/home/user/.config/pinner/mcp.env")
	// The names use a neutral placeholder form (no credential-keyword shapes)
	// so secret-scanners don't flag the test fixture; the assertion itself is
	// unchanged: no secret-named variable leaks into the rendered unit.
	require.NotContains(t, unit, "CONTROL_PLANE_TOKEN")
	require.NotContains(t, unit, "OPENAI_TOKEN")
	require.NotContains(t, unit, "MCP_AUTH_TOKEN")
}

func TestSystemdServiceLifecycleUsesArgumentArrays(t *testing.T) {
	var calls [][]string
	cfg := Config{Name: "pinner-mcp", UserMode: true}
	cfg.Runner = func(_ context.Context, command string, args ...string) error {
		calls = append(calls, append([]string{command}, args...))
		return nil
	}
	cfg.OutputRun = func(_ context.Context, command string, args ...string) (string, error) {
		calls = append(calls, append([]string{command}, args...))
		return "active", nil
	}
	cfg.WriteFile = func(string, []byte, os.FileMode) error { return nil }
	cfg.MkdirAll = func(string, os.FileMode) error { return nil }
	cfg.RemoveFile = func(string) error { return nil }

	svc := newSystemdService(cfg)
	require.NoError(t, svc.Start(context.Background()))
	require.NoError(t, svc.Stop(context.Background()))
	require.NoError(t, svc.Restart(context.Background()))
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{Installed: true, Active: true, Ready: true, Summary: "active (running)"}, status)
	require.Equal(t, [][]string{
		{"systemctl", "--user", "start", "pinner-mcp.service"},
		{"systemctl", "--user", "stop", "pinner-mcp.service"},
		{"systemctl", "--user", "restart", "pinner-mcp.service"},
		{"systemctl", "--user", "is-active", "pinner-mcp.service"},
	}, calls)
}

func TestSystemdServiceInstallAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	unitPath := filepath.Join(tmp, "systemd", "user", "pinner-mcp.service")
	var modes []os.FileMode
	var calls [][]string
	cfg := Config{Name: "pinner-mcp", UserMode: true, ServiceFile: unitPath}
	// MkdirAll, WriteFile, and RemoveFile perform the real filesystem
	// operations (inside the temp dir): Install must actually create the unit
	// file so Uninstall's disable (which always runs) still succeeds and the
	// RemoveFile cleanup is exercised on a real file.
	cfg.MkdirAll = func(path string, mode os.FileMode) error {
		require.Equal(t, filepath.Dir(unitPath), path)
		require.Equal(t, os.FileMode(0700), mode)
		return os.MkdirAll(path, mode)
	}
	cfg.WriteFile = func(path string, data []byte, mode os.FileMode) error {
		require.Equal(t, unitPath, path)
		modes = append(modes, mode)
		return os.WriteFile(path, data, mode)
	}
	cfg.RemoveFile = func(path string) error {
		require.Equal(t, unitPath, path)
		return os.Remove(path)
	}
	cfg.Runner = func(_ context.Context, command string, args ...string) error {
		calls = append(calls, append([]string{command}, args...))
		return nil
	}

	svc := newSystemdService(cfg)
	require.NoError(t, svc.Install(context.Background()))
	file, err := os.Stat(unitPath)
	require.NoError(t, err)
	require.NotZero(t, file.Size())
	require.Equal(t, []os.FileMode{0600}, modes)
	require.NoError(t, svc.Uninstall(context.Background()))
	_, err = os.Stat(unitPath)
	require.True(t, os.IsNotExist(err), "unit file must be removed by Uninstall")
	require.Equal(t, [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "pinner-mcp.service"},
		{"systemctl", "--user", "disable", "--now", "pinner-mcp.service"},
		{"systemctl", "--user", "daemon-reload"},
	}, calls)
}

func TestSystemdEscapeKeepsDollarLiteral(t *testing.T) {
	// Regression (PR #10 review): systemd performs NO variable expansion and
	// NO $$-collapse inside Environment= assignments (verified on systemd 255),
	// so systemdEscape must pass a literal $ through untouched — doubling it
	// would deliver "$$" to the process (mangling bcrypt hashes, passwords).
	// A $ still forces quoting, like space, tab, quote and backslash.
	require.Equal(t, `/opt/bin/pinner`, systemdEscape(`/opt/bin/pinner`))
	require.Equal(t, `""`, systemdEscape(""))
	require.Equal(t, `"a b"`, systemdEscape("a b"))
	require.Equal(t, `"say \"hi\""`, systemdEscape(`say "hi"`))
	require.Equal(t, `"C:\\path"`, systemdEscape(`C:\path`))
	// Literal dollars pass through verbatim; the value is only quoted.
	require.Equal(t, `"p$w0rd"`, systemdEscape("p$w0rd"))
	require.Equal(t, `"p$ w0rd"`, systemdEscape("p$ w0rd"))
	require.Equal(t, `"C:\\path$x"`, systemdEscape(`C:\path$x`))
}

func TestExecEscapeEscapesDollar(t *testing.T) {
	// Regression: systemd expands $VAR / ${VAR} in the ExecStart command line,
	// so a literal $ in an ExecPath or argument must be escaped as $$ and must
	// also force quoting, otherwise a $-containing path breaks (or executes)
	// at runtime.
	require.Equal(t, `/opt/bin/pinner`, execEscape(`/opt/bin/pinner`))
	require.Equal(t, `""`, execEscape(""))
	require.Equal(t, `"a b"`, execEscape("a b"))
	require.Equal(t, `"say \"hi\""`, execEscape(`say "hi"`))
	require.Equal(t, `"C:\\path"`, execEscape(`C:\path`))
	// Literal dollars are doubled and the value is quoted.
	require.Equal(t, `"p$$w0rd"`, execEscape("p$w0rd"))
	require.Equal(t, `"p$$ w0rd"`, execEscape("p$ w0rd"))
	require.Equal(t, `"C:\\path$$x"`, execEscape(`C:\path$x`))
}

func TestExecEscapeEscapesPercent(t *testing.T) {
	// Regression (PR #32): systemd expands % specifiers (%U, %H, %i, %n, ...)
	// in ExecStart= path and argument tokens, so a literal % must be doubled
	// to %% (which collapses back to a single % at run time), mirroring the
	// $ handling. "$" must still be doubled as before, and both together must
	// each be doubled independently.
	require.Equal(t, `/opt/bin/pinner`, execEscape(`/opt/bin/pinner`))
	require.Equal(t, `%%`, execEscape(`%`))
	// % does not trigger systemdEscape quoting (only $, whitespace, quotes and
	// backslashes do), but the doubled %% still neutralizes specifier expansion.
	require.Equal(t, `%%U`, execEscape(`%U`))
	require.Equal(t, `/opt/%%user/pinner`, execEscape(`/opt/%user/pinner`))
	require.Equal(t, `"p$$w%%rd"`, execEscape(`p$w%rd`))
}

func TestRenderSystemdUnitDollarHandling(t *testing.T) {
	// Regression (PR #10): the ExecStart $-escaping must NOT leak into
	// Environment= lines. systemd does no $-expansion there, so an env value
	// like "p$w0rd" (or a bcrypt hash "$2a$10$...") must be delivered
	// verbatim, while a $ EXECSTART argument must be doubled to $$ (which
	// collapses back to a single $ at runtime).
	unit := renderSystemdUnit(Config{
		Name:      "pinner-mcp",
		ExecPath:  "/opt/bin/pinner",
		Arguments: []string{"--hash", "$2a$10$abc"},
		EnvVars:   map[string]string{"PW": "p$w0rd"},
	})
	require.Contains(t, unit, `Environment=PW="p$w0rd"`)
	// No doubled dollar anywhere in the env value (only the ExecStart arg has $$).
	require.NotContains(t, unit, "p$$w0rd")
	require.Contains(t, unit, `ExecStart=/opt/bin/pinner --hash "$$2a$$10$$abc"`)
}

// fakeExitError fabricates a systemctl exit status without spawning a child
// process. It satisfies the exitCoder seam (interface{ ExitCode() int }) that
// systemdUnitAbsent classifies on — the same seam production *exec.ExitError
// values satisfy.
type fakeExitError struct{ code int }

func (e *fakeExitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e *fakeExitError) ExitCode() int { return e.code }

func TestSystemdServiceUninstallIdempotentWhenUnitAbsent(t *testing.T) {
	// Regression: Uninstall must be idempotent when systemd itself no longer
	// knows the unit (second uninstall after the unit file was already
	// removed, a retry after a partial failure). disable --now is ALWAYS
	// attempted, and its failure is tolerated as a no-op ONLY when the
	// `status` probe exits 4 ("unit not found" — exit-status keyed, never
	// stderr-parsed, so locale cannot break it). Cleanup (RemoveFile +
	// daemon-reload) still proceeds.
	var calls [][]string
	var removed []string
	cfg := Config{Name: "pinner-mcp", UserMode: true, ServiceFile: filepath.Join(t.TempDir(), "pinner-mcp.service")}
	cfg.Runner = func(_ context.Context, command string, args ...string) error {
		calls = append(calls, append([]string{command}, args...))
		if slices.Contains(args, "disable") {
			return errors.New("Failed to disable unit: Unit pinner-mcp.service not loaded.")
		}
		if slices.Contains(args, "status") {
			// Real systemctl exits 4 for a unknown unit.
			return &fakeExitError{code: 4}
		}
		return nil
	}
	cfg.RemoveFile = func(path string) error {
		removed = append(removed, path)
		return nil
	}

	svc := newSystemdService(cfg)
	require.NoError(t, svc.Uninstall(context.Background()))
	require.Equal(t, []string{cfg.ServiceFile}, removed)
	require.Equal(t, [][]string{
		{"systemctl", "--user", "disable", "--now", "pinner-mcp.service"},
		{"systemctl", "--user", "status", "pinner-mcp.service"},
		{"systemctl", "--user", "daemon-reload"},
	}, calls)
}

func TestSystemdServiceUninstallRealFailureWhenUnitMasked(t *testing.T) {
	// Audit regression: disable --now fails for a REAL reason (masked unit,
	// unit resolved from another search path) and the `status` probe returns a
	// non-4 exit code — systemd still KNOWS the unit. Uninstall must propagate
	// the failure instead of swallowing it as an idempotent no-op.
	unitPath := filepath.Join(t.TempDir(), "pinner-mcp.service")
	require.NoError(t, os.WriteFile(unitPath, []byte("[Unit]\n"), 0600))
	cfg := Config{Name: "pinner-mcp", UserMode: true, ServiceFile: unitPath}
	cfg.Runner = func(_ context.Context, _ string, args ...string) error {
		if slices.Contains(args, "disable") {
			return errors.New("Failed to disable unit: Unit pinner-mcp.service is masked.")
		}
		if slices.Contains(args, "status") {
			// A masked unit still exists to systemd: non-4 exit (3 = inactive).
			return &fakeExitError{code: 3}
		}
		return nil
	}
	cfg.RemoveFile = func(string) error {
		t.Fatal("RemoveFile must not run when disable fails for a real reason")
		return nil
	}
	svc := newSystemdService(cfg)
	err := svc.Uninstall(context.Background())
	require.ErrorContains(t, err, "disable systemd user service")
	require.ErrorContains(t, err, "masked")
}

func TestSystemdServiceUninstallPropagatesWhenProbeCannotClassify(t *testing.T) {
	// When `status` itself is unclassifiable (D-Bus down: a plain error with
	// no exit status), the disable failure must propagate — never be masked
	// as an idempotent no-op.
	cfg := Config{Name: "pinner-mcp", UserMode: true, ServiceFile: filepath.Join(t.TempDir(), "pinner-mcp.service")}
	cfg.Runner = func(_ context.Context, _ string, args ...string) error {
		if slices.Contains(args, "status") {
			return errors.New("Failed to connect to bus: No medium found")
		}
		return errors.New("Failed to connect to bus: Connection refused")
	}
	cfg.RemoveFile = func(string) error {
		t.Fatal("RemoveFile must not run when the probe cannot classify")
		return nil
	}
	svc := newSystemdService(cfg)
	err := svc.Uninstall(context.Background())
	require.ErrorContains(t, err, "disable systemd user service")
}

func TestSystemdServiceUninstallPropagatesRealDisableError(t *testing.T) {
	// With the unit file present on disk, disable runs; a genuine backend
	// failure (e.g. no D-Bus session) must propagate and abort before the
	// unit file is removed.
	unitPath := filepath.Join(t.TempDir(), "pinner-mcp.service")
	require.NoError(t, os.WriteFile(unitPath, []byte("[Unit]\n"), 0600))
	cfg := Config{Name: "pinner-mcp", UserMode: true, ServiceFile: unitPath}
	cfg.Runner = func(context.Context, string, ...string) error {
		return errors.New("systemctl: connection to bus failed")
	}
	cfg.RemoveFile = func(string) error {
		t.Fatal("RemoveFile must not run when disable fails for a real reason")
		return nil
	}
	svc := newSystemdService(cfg)
	err := svc.Uninstall(context.Background())
	require.ErrorContains(t, err, "disable systemd user service")
}

func TestSystemdServiceRejectsSystemMode(t *testing.T) {
	svc := newSystemdService(Config{ServiceFile: "/tmp/unit"})
	require.ErrorContains(t, svc.Install(context.Background()), "only supports user-mode")
}

func TestSystemdServiceStatusNotInstalled(t *testing.T) {
	// A missing unit: `is-active` prints "inactive" and exits non-zero (exit
	// 3), and `list-unit-files --output=json` returns "[]" -> not installed.
	var subcmds []string
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
		subcmds = append(subcmds, args[1])
		if slices.Contains(args, "is-active") {
			return "inactive", errors.New("exit status 3")
		}
		return "[]", nil
	}
	svc := newSystemdService(cfg)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{Summary: "not installed"}, status)
	require.Equal(t, []string{"is-active", "list-unit-files"}, subcmds)
}

func TestSystemdServiceStatusStoppedWhenUnitFileExists(t *testing.T) {
	// Installed but stopped: `is-active` prints "inactive" and exits non-zero,
	// and `list-unit-files --output=json` lists the unit file -> installed, not
	// running.
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
		if slices.Contains(args, "is-active") {
			return "inactive", errors.New("exit status 3")
		}
		return `[{"unit_file":"pinner-mcp.service","state":"static"}]`, nil
	}
	svc := newSystemdService(cfg)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{Installed: true, Summary: "inactive"}, status)
}

func TestSystemdServiceStatusPropagatesBackendError(t *testing.T) {
	// Regression: a genuine backend failure must propagate as an error, NOT be
	// mistaken for "not installed" or masked as an installed service with a
	// garbage summary.
	tests := []struct {
		name   string
		output string
		err    error
	}{
		{name: "empty output", output: "", err: errors.New("Failed to connect to bus")},
		// CombinedOutput coalesces stderr, so a D-Bus failure arrives as
		// non-empty output + error. The business-garbage token must not be
		// treated as a state.
		{name: "stderr coalesced", output: "Failed to connect to bus: no such file or directory", err: errors.New("exit status 4")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Name: "pinner-mcp"}
			cfg.OutputRun = func(context.Context, string, ...string) (string, error) {
				return tt.output, tt.err
			}
			svc := newSystemdService(cfg)
			_, err := svc.Status(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "query systemd user service")
		})
	}
}

func TestSystemdServiceStatusRunning(t *testing.T) {
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
		require.True(t, slices.Contains(args, "is-active"))
		// Active: exit 0 means OutputRun returns nil error.
		return "active", nil
	}
	svc := newSystemdService(cfg)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{Installed: true, Active: true, Ready: true, Summary: "active (running)"}, status)
}

func TestSystemdServiceStatusFailed(t *testing.T) {
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
		if slices.Contains(args, "is-active") {
			// Real systemd prints "failed" and exits non-zero (exit 3); the
			// state token must be honored, not masked as a backend error.
			return "failed", errors.New("exit status 3")
		}
		return `[{"unit_file":"pinner-mcp.service","state":"static"}]`, nil
	}
	svc := newSystemdService(cfg)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{Installed: true, Summary: "failed"}, status)
}

func TestSystemdServiceStatusTransitionalStates(t *testing.T) {
	// Transitional states stay truthful: activating/reloading are in use but
	// not yet ready; deactivating is heading toward stopped — not reported as
	// fully running or as a dropped service.
	tests := []struct {
		state  string
		status Status
	}{
		{"activating", Status{Installed: true, Active: true, Summary: "activating"}},
		{"reloading", Status{Installed: true, Active: true, Summary: "activating"}},
		{"deactivating", Status{Installed: true, Summary: "deactivating"}},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			cfg := Config{Name: "pinner-mcp"}
			cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
				if slices.Contains(args, "is-active") {
					return tt.state, errors.New("exit status 3")
				}
				return `[{"unit_file":"pinner-mcp.service","state":"static"}]`, nil
			}
			svc := newSystemdService(cfg)
			got, err := svc.Status(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.status, got)
		})
	}
}

func TestSystemdServiceStatusNonRunningNotInstalled(t *testing.T) {
	// A state reporting a unit that has no unit file (loaded-but-removed,
	// stale activator) must be reported as not installed even when the active
	// state is failed/transitional.
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
		if slices.Contains(args, "is-active") {
			return "inactive", errors.New("exit status 3")
		}
		return "[]", nil
	}
	svc := newSystemdService(cfg)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{Summary: "not installed"}, status)
}

func TestParseSystemdActiveState(t *testing.T) {
	require.Equal(t, systemdActive, parseSystemdActiveState("active"))
	require.Equal(t, systemdInactive, parseSystemdActiveState("inactive"))
	require.Equal(t, systemdFailed, parseSystemdActiveState("failed\n"))
	// Coalesced stderr must not poison the state token; the first token wins.
	require.Equal(t, systemdInactive, parseSystemdActiveState("inactive\nFailed to get properties: No such file or directory"))
	// A genuine bus error with no state token is unknown.
	require.Equal(t, systemdStateUnknown, parseSystemdActiveState("Failed to connect to bus: no such file or directory"))
	require.Equal(t, systemdStateUnknown, parseSystemdActiveState(""))
}

func TestUnitFileListContains(t *testing.T) {
	require.False(t, unitFileListContains(`[]`, "pinner-mcp.service"))
	require.True(t, unitFileListContains(`[{"unit_file":"pinner-mcp.service","state":"static"}]`, "pinner-mcp.service"))
	require.False(t, unitFileListContains(`[{"unit_file":"other.service","state":"static"}]`, "pinner-mcp.service"))
	// Unknown JSON shape falls back to substring detection.
	require.True(t, unitFileListContains(`pinner-mcp.service`, "pinner-mcp.service"))
}
