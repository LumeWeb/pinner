package services

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Service controls the lifecycle of a configured service (daemon) on the host's
// init system. Implementations are per-platform (systemd, launchd, Windows SCM,
// ...) and are selected automatically via the System registry in system.go.
//
// The service points at a real executable (Config.ExecPath + Config.Arguments):
// install/control only — we never run the process in-process. This keeps the
// service layer a thin lifecycle adapter over the host init system.
type Service interface {
	// Install registers the service with the init system and enables it so it
	// starts on boot.
	Install(ctx context.Context) error
	// Uninstall removes the service from the init system.
	Uninstall(ctx context.Context) error
	// Start starts the service now.
	Start(ctx context.Context) error
	// Stop stops the service now.
	Stop(ctx context.Context) error
	// Restart stops then starts the service.
	Restart(ctx context.Context) error
	// Status reports the current service state.
	Status(ctx context.Context) (Status, error)
	// Logs tails the service's logs. follow=true streams until the context is
	// cancelled or the command exits.
	Logs(ctx context.Context, follow bool) error
}

// ErrServiceAlreadyExists is returned by Install when the service is already
// registered with the host init system and cannot be re-registered. Only the
// Windows SCM backend returns it (CreateService refuses an existing service);
// systemd and launchd reinstall idempotently and never return it.
var ErrServiceAlreadyExists = errors.New("service already exists")

// Status is the backend-independent service state exposed to callers.
type Status struct {
	Installed bool
	Active    bool
	Ready     bool
	Summary   string
}

// Config describes the process and environment a service backend manages.
type Config struct {
	// Name is the service identifier (no spaces; e.g. "pinner-mcp"). Required.
	Name string
	// Description is human-facing metadata rendered into the init system's
	// unit/plist/registry where supported.
	Description string
	// ExecPath is the absolute path to the executable the service runs.
	ExecPath string
	// Arguments are appended to ExecPath in the generated unit/plist/SCM entry.
	Arguments []string
	// EnvVars are injected into the service's environment by the init system
	// (systemd Environment=, launchd EnvironmentVariables, Windows SCM registry
	// Environment). Empty by default.
	EnvVars map[string]string
	// EnvFile is an absolute path to a KEY=VALUE environment file the init
	// system loads at runtime. On systemd this is rendered as EnvironmentFile=
	// so secrets stay in a private file rather than the unit. Empty by default.
	EnvFile string
	// UserMode installs a per-user service (systemd user unit, LaunchAgent)
	// instead of a system-wide one. Not meaningful on Windows SCM, where
	// services are always system services.
	UserMode bool
	// ServiceFile is the exact path the backend writes its unit/plist/SCM
	// definition to. Empty lets the backend choose a default for the platform.
	ServiceFile string

	// Command exec seams take the place of the real system binaries so tests
	// can drive a backend without a live init system. Defaults to the host
	// commands when nil.
	Runner     CommandRunner
	OutputRun  CommandOutputRunner
	WriteFile  func(string, []byte, os.FileMode) error
	RemoveFile func(string) error
	MkdirAll   func(string, os.FileMode) error
}

// mergedEnvVars returns the union of Config.EnvVars and the EnvFile's values,
// used by backends that resolve the environment at install time. It starts
// from a copy of Config.EnvVars (so the caller's map is never mutated) and
// overlays the env file's values, which win on a key collision (the env file
// is the secret source), matching the systemd renderer's behavior of emitting
// both Environment= and EnvironmentFile=. An absent env file is tolerated.
func mergedEnvVars(cfg Config) (map[string]string, error) {
	envVars := make(map[string]string, len(cfg.EnvVars))
	for k, v := range cfg.EnvVars {
		envVars[k] = v
	}
	if cfg.EnvFile != "" {
		env, err := LoadEnvironment(cfg.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("load service environment %q: %w", cfg.EnvFile, err)
		}
		for k, v := range env {
			envVars[k] = v
		}
	}
	return envVars, nil
}
