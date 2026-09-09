package config

import (
	"testing"
	"time"

	z "github.com/Oudwins/zog"
	"github.com/stretchr/testify/assert"
)

// validateConfigSchema runs the Config's zog struct schema in Validate mode.
// configmanager validates registered structs by invoking this same method on
// the decoded *Config, so the schema sees time.Duration field values.
func validateConfigSchema(t *testing.T, cfg *Config) z.ZogIssueList {
	t.Helper()
	v, ok := cfg.Schema().(interface {
		Validate(any, ...z.ExecOption) z.ZogIssueList
	})
	if !ok {
		t.Fatal("schema does not expose Validate")
	}
	return v.Validate(cfg)
}

// TestTimeoutSchemaDurUnits pins that the timeout fields are validated in
// time.Duration units (not bare integer seconds): in-range durations pass,
// out-of-range durations fail, and the zero value is accepted as "unset" so
// the getters' default fallback keeps working.
func TestTimeoutSchemaDurUnits(t *testing.T) {
	t.Run("valid defaults and configured values pass", func(t *testing.T) {
		cfg := &Config{
			DefaultTimeout: time.Duration(DefaultTimeoutSeconds) * time.Second,
			UploadTimeout:  time.Duration(DefaultUploadTimeoutSeconds) * time.Second,
			SyncTimeout:    time.Duration(DefaultSyncTimeoutSeconds) * time.Second,
		}
		assert.Empty(t, validateConfigSchema(t, cfg))

		// boundary values are valid (parse also happens in duration units)
		cfg = &Config{DefaultTimeout: time.Second, UploadTimeout: 3600 * time.Second}
		assert.Empty(t, validateConfigSchema(t, cfg))
	})

	t.Run("above the 1h maximum is invalid", func(t *testing.T) {
		cfg := &Config{DefaultTimeout: 3600*time.Second + time.Nanosecond}
		assert.NotEmpty(t, validateConfigSchema(t, cfg))

		cfg = &Config{UploadTimeout: 7200 * time.Second}
		assert.NotEmpty(t, validateConfigSchema(t, cfg))
	})

	t.Run("sub-second and negative values are invalid", func(t *testing.T) {
		cfg := &Config{DefaultTimeout: 500 * time.Millisecond}
		assert.NotEmpty(t, validateConfigSchema(t, cfg))

		cfg = &Config{SyncTimeout: -time.Second}
		assert.NotEmpty(t, validateConfigSchema(t, cfg))
	})

	t.Run("zero is treated as unset and passes", func(t *testing.T) {
		cfg := &Config{}
		assert.Empty(t, validateConfigSchema(t, cfg))
	})
}

func TestGetDefaultTimeout(t *testing.T) {
	c := &Config{}
	assert.Equal(t, time.Duration(DefaultTimeoutSeconds)*time.Second, c.GetDefaultTimeout())

	c = &Config{DefaultTimeout: 45 * time.Second}
	assert.Equal(t, 45*time.Second, c.GetDefaultTimeout())
}

func TestGetUploadTimeout(t *testing.T) {
	c := &Config{}
	assert.Equal(t, time.Duration(DefaultUploadTimeoutSeconds)*time.Second, c.GetUploadTimeout())

	c = &Config{UploadTimeout: 10 * time.Minute}
	assert.Equal(t, 10*time.Minute, c.GetUploadTimeout())
}

func TestGetSyncTimeout(t *testing.T) {
	c := &Config{}
	assert.Equal(t, time.Duration(DefaultSyncTimeoutSeconds)*time.Second, c.GetSyncTimeout())

	c = &Config{SyncTimeout: 2 * time.Minute}
	assert.Equal(t, 2*time.Minute, c.GetSyncTimeout())
}
