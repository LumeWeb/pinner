package appswire

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/pinner/canvas"
)

// The table is the shared contract between every composition that installs
// MCP Apps: launchers are unique, screens derive from launcher names, URIs
// resolve to real canvas views, and the hosted/CLI mask split matches the
// deployment-capability facts (no OOB/vault views on hosted).

func TestAllViewsUnique(t *testing.T) {
	seenLauncher := map[string]bool{}
	seenURI := map[string]bool{}
	seenResource := map[string]bool{}
	for _, v := range All() {
		require.False(t, seenLauncher[v.Launcher], "duplicate launcher %s", v.Launcher)
		require.False(t, seenURI[v.URI], "duplicate URI %s", v.URI)
		require.False(t, seenResource[v.ResourceName], "duplicate resource %s", v.ResourceName)
		seenLauncher[v.Launcher] = true
		seenURI[v.URI] = true
		seenResource[v.ResourceName] = true

		require.True(t, strings.HasPrefix(v.Launcher, AppLauncherToolPrefix),
			"launcher %s must carry the %s prefix", v.Launcher, AppLauncherToolPrefix)
		require.NotEmpty(t, v.Screen())
		require.True(t, strings.HasPrefix(v.URI, "ui://"), "view %s: URI %s must be ui://", v.Launcher, v.URI)
		require.NotEmpty(t, v.ResourceTitle)
		require.NotEmpty(t, v.ResourceDescription)
		require.NotEmpty(t, v.AppPhrase)
		require.NotEmpty(t, v.HumanPurpose)
		require.NotEmpty(t, v.Headless)
		// Description skeleton must compose: every field it references is set.
		require.Contains(t, v.OpenLauncherDescriptionFor(), v.Headless)
	}
}

func TestAllViewsCoverCanvasScreenVocabulary(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range All() {
		require.Equal(t, v.Launcher[len(AppLauncherToolPrefix):], v.Screen())
		require.Contains(t, canvas.AllViews(), v.View,
			"view %s references canvas screen %q outside the contract vocabulary", v.Launcher, v.View)
		if seen[string(v.View)] {
			t.Errorf("canvas view %s claimed twice", v.View)
		}
		seen[string(v.View)] = true
	}
	require.Len(t, seen, len(canvas.AllViews()),
		"the table must cover every canvas screen exactly once")
}

func TestCapabilityRequirementsAreEncoded(t *testing.T) {
	// OOB handoff views require the OOB coordinators; the Sia vault views
	// require the vault surface; everything else needs no named capability
	// (its dependencies ride the installer). These are wiring requirements —
	// the table never excludes a deployment kind.
	for _, v := range All() {
		switch v.Launcher {
		case LauncherSSOSignin, LauncherAccountPassword, LauncherAccountEmail:
			require.Equalf(t, CapOOB, v.Requires, "%s must require OOB", v.Launcher)
		case LauncherVaultCreate, LauncherVaultRestore, LauncherVaultBrowser, LauncherVaultManager, LauncherVaultDownloadMgr:
			require.Equalf(t, CapVault, v.Requires, "%s must require the vault surface", v.Launcher)
		default:
			require.Zerof(t, v.Requires, "%s must be capability-free", v.Launcher)
		}
	}
}

func TestSelectableFiltersByCapsAndDeps(t *testing.T) {
	// A deployment that wires nothing beyond the base capabilities only sees
	// the capability-free rows; wiring OOB and the vault surface admits every
	// row (a nil available takes the rest as wired).
	basic := Selectable(CapHosted|CapOOB|CapVault, nil)
	require.Len(t, basic, len(All()))

	noVault := Selectable(CapHosted|CapOOB, nil)
	require.Len(t, noVault, len(All())-5, "vault views drop when the vault surface is not wired")

	// Dependencies filter within the capability coverage: only the pin list
	// row reports wired here.
	only := Selectable(CapHosted|CapOOB|CapVault, func(v ViewSpec) bool { return v.Launcher == LauncherPinList })
	require.Len(t, only, 1)
	require.Equal(t, LauncherPinList, only[0].Launcher)
}

func TestLauncherConstantsResolve(t *testing.T) {
	for name, c := range map[string]string{
		"pin_creator":   LauncherPinCreator,
		"sso_signin":    LauncherSSOSignin,
		"acct_pass":     LauncherAccountPassword,
		"acct_email":    LauncherAccountEmail,
		"account":       LauncherAccount,
		"vault_create":  LauncherVaultCreate,
		"vault_restore": LauncherVaultRestore,
		"vault_browser": LauncherVaultBrowser,
		"pin_list":      LauncherPinList,
		"upload":        LauncherUploadManager,
		"vault_upload":  LauncherVaultManager,
		"download":      LauncherDownloadManager,
		"vault_dl":      LauncherVaultDownloadMgr,
	} {
		v, ok := SpecForLauncher(c)
		require.Truef(t, ok, "launcher constant %s (%q) must resolve to a table row", name, c)
		require.Equalf(t, c, v.Launcher, "constant %s must round-trip to itself", name)
	}
}

func TestSpecForLauncher(t *testing.T) {
	v, ok := SpecForLauncher(LauncherPinCreator)
	require.True(t, ok)
	require.Equal(t, canvas.ViewPin, v.View)
	v, ok = SpecForLauncher("open_nonexistent")
	require.False(t, ok)
	require.Zero(t, v.Launcher)
}
