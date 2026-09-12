package transfer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mcptransfer "go.lumeweb.com/mcpplane/transfer"
)

// TestParsePresignTTL pins the ONE canonical presigned-PUT TTL parser shared
// by every presigned surface (upload_file remote mint, the open_upload_manager
// launcher, the upload App helpers):
//   - empty/whitespace input → mcpplane/transfer's default TTL;
//   - unparseable input → wrapped error with the stable wording
//     `invalid ttl "..."`;
//   - non-positive input (0s, negative) → default (NOT an error), matching the
//     coordinator-side clamping.
func TestParsePresignTTL(t *testing.T) {
	t.Run("empty defaults", func(t *testing.T) {
		d, err := ParsePresignTTL("")
		require.NoError(t, err)
		require.Equal(t, mcptransfer.DefaultHTTPUploadTTL, d)
	})
	t.Run("whitespace defaults", func(t *testing.T) {
		d, err := ParsePresignTTL("   ")
		require.NoError(t, err)
		require.Equal(t, mcptransfer.DefaultHTTPUploadTTL, d)
	})
	t.Run("valid duration passes through", func(t *testing.T) {
		d, err := ParsePresignTTL("5m")
		require.NoError(t, err)
		require.Equal(t, 5*time.Minute, d)
	})
	t.Run("padded duration passes through", func(t *testing.T) {
		d, err := ParsePresignTTL(" 5m ")
		require.NoError(t, err)
		require.Equal(t, 5*time.Minute, d)
	})
	t.Run("fractional passes through", func(t *testing.T) {
		d, err := ParsePresignTTL("90s")
		require.NoError(t, err)
		require.Equal(t, 90*time.Second, d)
	})
	t.Run("zero falls back to default", func(t *testing.T) {
		d, err := ParsePresignTTL("0s")
		require.NoError(t, err)
		require.Equal(t, mcptransfer.DefaultHTTPUploadTTL, d)
	})
	t.Run("negative falls back to default", func(t *testing.T) {
		d, err := ParsePresignTTL("-1m")
		require.NoError(t, err)
		require.Equal(t, mcptransfer.DefaultHTTPUploadTTL, d)
	})
	t.Run("unparseable yields stable invalid ttl wording", func(t *testing.T) {
		_, err := ParsePresignTTL("soon")
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid ttl "soon"`)
	})
}
