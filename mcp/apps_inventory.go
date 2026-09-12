package mcp

import (
	"fmt"
)

// installedAppsValidation outlines the guarantees Assemble applies to
// Config.InstalledApps before the guide reads it. The inventory is
// composition-root-supplied — the app registry itself sits behind the MCP SDK
// (see the Config NOTE) — so this method cannot diff the names against live
// registrations. Two assertions hold:
//
//   - entries must be unique (a duplicated name is always a wiring mistake);
//   - the composition root's registry-side assertion
//     (Config.VerifyInstalledApps) runs at assembly time whenever its seam
//     can see the registry it installed views through.
//
// The blessed composition pattern keeps the remaining registered-vs-listed
// guarantee structural: populate Config.InstalledApps from the launcher names
// the shared app-view wiring returns when views actually install — never from
// a hand-maintained list. Entries that name a launcher outside the app-view
// vocabulary are rejected by the vocabulary check layered on top by the
// appswire table PR.
func validateInstalledApps(installedApps []string) error {
	seen := map[string]bool{}
	for _, name := range installedApps {
		if seen[name] {
			return fmt.Errorf("mcp: InstalledApps entry %q is duplicated", name)
		}
		seen[name] = true
	}
	return nil
}
