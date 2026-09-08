package pinnerservices

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergedEnvVarsMergesCallerEnvVarsWithEnvFile(t *testing.T) {
	// Regression: backends that inline the resolved environment at install
	// time (launchd plist, Windows SCM registry) must MERGE Config.EnvVars
	// with the env file's values, not replace the caller's variables with the
	// file's contents. The env file wins on a key collision.
	envPath := filepath.Join(t.TempDir(), "mcp.env")
	require.NoError(t, os.WriteFile(envPath, []byte("ENV_ONE=from-file\nFILE_ONLY=1\n"), 0600))

	merged, err := mergedEnvVars(Config{
		EnvVars: map[string]string{"ENV_ONE": "from-caller", "CALLER_ONLY": "2"},
		EnvFile: envPath,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"ENV_ONE":     "from-file", // env file wins on collision
		"CALLER_ONLY": "2",         // caller-provided vars are kept
		"FILE_ONLY":   "1",         // env-file-only vars are added
	}, merged)
}

func TestMergedEnvVarsKeepsCallerMapUntouched(t *testing.T) {
	// The helper must resolve the union into a fresh map: the caller's
	// Config.EnvVars must never be mutated by a backend install.
	envPath := filepath.Join(t.TempDir(), "mcp.env")
	require.NoError(t, os.WriteFile(envPath, []byte("FILE_ONLY=1\n"), 0600))

	caller := map[string]string{"CALLER_ONLY": "2"}
	merged, err := mergedEnvVars(Config{EnvVars: caller, EnvFile: envPath})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"CALLER_ONLY": "2"}, caller)
	require.Equal(t, map[string]string{"CALLER_ONLY": "2", "FILE_ONLY": "1"}, merged)
	require.Len(t, merged, 2) // a real merge, not the same map aliased
}

func TestMergedEnvVarsNilEnvVars(t *testing.T) {
	// A nil EnvVars map with only an env file still resolves the file's values.
	envPath := filepath.Join(t.TempDir(), "mcp.env")
	require.NoError(t, os.WriteFile(envPath, []byte("FILE_ONLY=1\n"), 0600))

	merged, err := mergedEnvVars(Config{EnvFile: envPath})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"FILE_ONLY": "1"}, merged)
}

func TestMergedEnvVarsAbsentEnvFile(t *testing.T) {
	// LoadEnvironment tolerates a missing file, so a fresh, unprovisioned
	// install yields just the caller's variables.
	merged, err := mergedEnvVars(Config{
		EnvVars: map[string]string{"CALLER_ONLY": "2"},
		EnvFile: filepath.Join(t.TempDir(), "missing.env"),
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"CALLER_ONLY": "2"}, merged)

	merged, err = mergedEnvVars(Config{})
	require.NoError(t, err)
	require.Empty(t, merged)
}

func TestMergedEnvVarsPropagatesLoadError(t *testing.T) {
	// A real load failure (here: a directory instead of a file) must
	// propagate, not silently install with no credentials.
	merged, err := mergedEnvVars(Config{EnvFile: t.TempDir()})
	require.Error(t, err)
	require.Nil(t, merged)
}
