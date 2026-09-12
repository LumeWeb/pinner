package catalogmcp

import (
	"strings"
	"testing"
)

// TestTargetsCarryNoMalformedPortalPlaceholder pins the plugin-guideline
// accuracy contract: a tool description must never document a destination via
// an unresolved hostname placeholder ("account.<portal>"), which is neither
// clear nor actionable. The real deep-link URL is returned as data in the
// tool result (e.g. account_subscription's web_url field), not published in
// the description.
func TestTargetsCarryNoMalformedPortalPlaceholder(t *testing.T) {
	for opID, targets := range opTargets {
		for _, tgt := range targets {
			if !strings.Contains(tgt.Description, "account.<portal>") {
				continue
			}
			t.Errorf("op %s: description contains the unresolved placeholder \"account.<portal>\": %q",
				opID, tgt.Description)
		}
	}
}
