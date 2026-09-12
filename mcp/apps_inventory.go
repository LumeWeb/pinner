package mcp

import (
	"fmt"

	"go.lumeweb.com/pinner/mcp/appswire"
)

// validateInstalledApps checks Config.InstalledApps against the shared
// app-view table before the assembled server reads it. The inventory is
// composition-root-supplied — the app registry itself sits behind the MCP SDK
// (see the Config NOTE), so this method cannot diff the names against live
// registrations — but an entry that is not a declared app-view launcher is a
// provable lie: the guide would enumerate it in {{APPS}} prose and accept it
// as an open_app name, advertising a view tools/list cannot carry. Unknown
// names fail the assembly loudly, as do duplicates.
//
// The blessed composition pattern keeps the remaining (registered-vs-listed)
// half of the guarantee structural: populate Config.InstalledApps from the
// slice appswire/install's Install returns — the launcher names whose views
// actually registered — never from a hand-maintained list. A composition root
// can additionally assert its registry-side state via
// Config.VerifyInstalledApps when its assembly seam makes that possible.
func validateInstalledApps(installedApps []string) error {
	seen := map[string]bool{}
	for _, name := range installedApps {
		if _, ok := appswire.SpecForLauncher(name); !ok {
			return fmt.Errorf("mcp: InstalledApps entry %q is not a declared app-view launcher", name)
		}
		if seen[name] {
			return fmt.Errorf("mcp: InstalledApps entry %q is duplicated", name)
		}
		seen[name] = true
	}
	return nil
}
