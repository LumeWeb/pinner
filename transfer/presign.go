package transfer

import (
	"fmt"
	"strings"
	"time"

	mcptransfer "go.lumeweb.com/mcpplane/transfer"
)

// ParsePresignTTL is the ONE canonical presigned-PUT TTL parser shared by
// every presigned-upload surface in this module: upload_file's remote mint
// branch, the open_upload_manager launcher, and the upload App's helper
// tools. Using a single parser keeps the accepted wire format, the default,
// the non-positive handling, and the error wording identical across surfaces:
//
//   - empty (or whitespace) input yields mcpplane/transfer's default TTL;
//   - unparseable input yields a wrapped time.ParseDuration error with the
//     stable wording `invalid ttl "..."`;
//   - a parsed non-positive duration is NOT an error: it falls back to the
//     default, matching the coordinator-side clamping (an explicit "0s" or
//     "-1m" gets the documented default lifetime rather than an unusable
//     endpoint).
func ParsePresignTTL(raw string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return mcptransfer.DefaultHTTPUploadTTL, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid ttl %q: %w", raw, err)
	}
	if d <= 0 {
		return mcptransfer.DefaultHTTPUploadTTL, nil
	}
	return d, nil
}
