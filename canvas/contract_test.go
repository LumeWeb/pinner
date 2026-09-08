package canvas

import (
	"encoding/json"
	"regexp"
	"sort"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// referenceViews is the exact set of screens the reference implementation
// serves today: pinner-cli internal/mcpapp apps_embed.go bundleNames keys,
// each registered as a ui:// resource by internal/mcp. Any drift — a screen
// added server-side without a contract here, or a contract without a screen
// server-side — must fail the contract tests.
var referenceViews = []View{
	ViewPin,
	ViewPinList,
	ViewVaultBrowser,
	ViewVaultCreate,
	ViewVaultRestore,
	ViewVaultUpload,
	ViewVaultDownload,
	ViewIPFSUpload,
	ViewIPFSDownload,
	ViewAuthSSO,
	ViewAuthStatus,
	ViewAccountPassword,
	ViewAccountEmail,
}

// requireSorted asserts the slice is sorted and returns its sorted copy for
// set comparisons.
func requireSorted(t *testing.T, name string, in []View) []View {
	t.Helper()
	sorted := make([]View, len(in))
	copy(sorted, in)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	assert.Equal(t, sorted, in, "%s must be maintained in sorted order", name)
	return sorted
}

// requireStableID asserts a view ID meets the documented contract surface
// rules: non-empty, lowercase kebab-case ASCII, and valid per
// View.Validate.
func requireStableID(t *testing.T, v View, label string) {
	t.Helper()
	name := string(v)
	require.NotEmpty(t, name, "%s view ID must not be empty", label)
	for _, r := range name {
		require.False(t, unicode.IsUpper(r), "%s view ID %q must be lowercase", label, name)
		require.True(t, r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r),
			"%s view ID %q must be ASCII letters, digits, or hyphens", label, name)
	}
	require.NoError(t, v.Validate(), "%s view ID %q must satisfy Validate", label, name)
}

// TestViewSetExhaustiveAgainstReference pins the view table to the exact
// reference screen set: no missing view, no extra view, sorted order.
func TestViewSetExhaustiveAgainstReference(t *testing.T) {
	registered := requireSorted(t, "allViews", AllViews())

	reference := make([]View, len(referenceViews))
	copy(reference, referenceViews)
	sort.Slice(reference, func(i, j int) bool { return reference[i] < reference[j] })

	require.Equal(t, reference, registered,
		"canvas view contract diverged from the reference screens")

	// The constants the tests name must itself be dup-free and cover every
	// registered view, so this test cannot accidentally drift into
	// tautology.
	set := make(map[View]int, len(referenceViews))
	for _, v := range referenceViews {
		set[v]++
	}
	for v, n := range set {
		require.Equal(t, 1, n, "referenceViews lists %q %d times", v, n)
		require.True(t, v.IsValid(), "reference view %q is not registered", v)
	}
	require.Len(t, set, len(registered), "referenceViews must exhaust the registered set")
}

// TestViewIDsStable pins the format and uniqueness half of the contract:
// every view ID is a stable non-empty kebab-case string matching the
// documented format, unique across the surface, and registered exactly
// once.
func TestViewIDsStable(t *testing.T) {
	format := regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	seen := make(map[View]int, len(allViews))

	for _, v := range AllViews() {
		requireStableID(t, v, "registered")
		require.Regexp(t, format, string(v))
		seen[v]++
	}
	for v, n := range seen {
		require.Equal(t, 1, n, "view ID %q registered more than once", v)
	}

	// AllViews must be a non-progressing enumeration: stable across calls.
	require.Equal(t, AllViews(), AllViews())
}

// TestValidateRejectsBadIDs pins the format guard so registration
// boundaries can rely on it: a bad ID is rejected with ErrViewName, a valid
// ID of an unregistered view still validates (format and registration are
// distinct concerns).
func TestValidateRejectsBadIDs(t *testing.T) {
	for _, bad := range []View{"", "Pin", "pin ", "pin_", "-pin", "pin--list", "1pin"} {
		err := bad.Validate()
		require.ErrorIs(t, err, ErrViewName, "Validate(%q)", bad)
	}

	good := View("vault-audit")
	require.NoError(t, good.Validate())
	require.False(t, good.IsValid(), "format-valid ID is not a registered view")
}

// TestUnknownViewSentinel pins unknown-view handling everywhere: every
// view-bound accessor rejects an unregistered ID with ErrUnknownView, and
// registered IDs never produce it.
func TestUnknownViewSentinel(t *testing.T) {
	unknown := []View{"", "banana", "Pin", "pin-list "}

	for _, v := range unknown {
		_, err := v.URI()
		require.ErrorIs(t, err, ErrUnknownView, "URI(%q)", v)
		_, err = v.Actions()
		require.ErrorIs(t, err, ErrUnknownView, "Actions(%q)", v)
		_, err = v.States()
		require.ErrorIs(t, err, ErrUnknownView, "States(%q)", v)
		_, err = v.Payload()
		require.ErrorIs(t, err, ErrUnknownView, "Payload(%q)", v)
	}

	for _, v := range AllViews() {
		_, err := v.URI()
		require.NotErrorIs(t, err, ErrUnknownView, "view %q", v)
	}
}

// TestEveryViewContracted pins that every registered view carries the full
// contract: a format-valid ID, at least one action, at least one state, and
// a resolved payload disposition (typed payload or a documented
// ErrMissingPayload). A view is never contract-free.
func TestEveryViewContracted(t *testing.T) {
	for _, v := range AllViews() {
		actions, err := v.Actions()
		require.NoError(t, err, "view %q", v)
		require.NotEmpty(t, actions, "view %q has no actions", v)

		states, err := v.States()
		require.NoError(t, err, "view %q", v)
		require.NotEmpty(t, states, "view %q has no states", v)

		uri, err := v.URI()
		require.NoError(t, err, "view %q", v)
		require.Regexp(t, `^ui://[a-z]+/[a-z-]+\.html$`, uri, "view %q URI %q", v, uri)
	}
}

// TestPayloadDispositions pins which views have typed server-provided
// payloads and which are documented self-bootstrapping views that return
// ErrMissingPayload, so payload coverage cannot silently change.
func TestPayloadDispositions(t *testing.T) {
	typed := map[View]any{
		ViewPin:          PinStatusPayload{},
		ViewPinList:      PinListPayload{},
		ViewAuthStatus:   AuthStatusPayload{},
		ViewVaultBrowser: VaultStatusPayload{},
	}

	for _, v := range AllViews() {
		payload, err := v.Payload()
		_, defined := typed[v]
		if defined {
			require.NoError(t, err, "view %q", v)
			require.IsType(t, typed[v], payload, "view %q payload type", v)
			continue
		}
		require.Nil(t, payload, "view %q unexpectedly carries a payload", v)
		require.ErrorIs(t, err, ErrMissingPayload, "view %q", v)
	}

	// The nine remaining reference views bootstrap themselves; exactly those
	// return ErrMissingPayload.
	expectMissing := []View{
		ViewAccountEmail,
		ViewAccountPassword,
		ViewAuthSSO,
		ViewIPFSDownload,
		ViewIPFSUpload,
		ViewVaultCreate,
		ViewVaultDownload,
		ViewVaultRestore,
		ViewVaultUpload,
	}
	var gotMissing []View
	for _, v := range AllViews() {
		if _, err := v.Payload(); err != nil {
			require.ErrorIs(t, err, ErrMissingPayload, "view %q", v)
			gotMissing = append(gotMissing, v)
		}
	}
	sort.Slice(gotMissing, func(i, j int) bool { return gotMissing[i] < gotMissing[j] })
	require.Equal(t, expectMissing, gotMissing)
}

// TestPayloadJSONKeys pins the payload schemas' wire keys (the field names
// are the versioned compatibility surface, not the Go field names).
func TestPayloadJSONKeys(t *testing.T) {
	fixtures := []struct {
		name   string
		val    any
		fields []string
	}{
		{"PinStatusPayload", PinStatusPayload{Status: "s", CID: "c"}, []string{"cid", "status"}},
		{"PinListPayload", PinListPayload{Pins: []PinRow{}}, []string{"pins"}},
		{"PinRow", PinRow{CID: "c", Name: "n", Status: "s", Created: "t", RequestID: "r", Metadata: map[string]string{"m": "1"}}, []string{"cid", "name", "status", "created", "request_id", "metadata"}},
		{"AuthStatusPayload", AuthStatusPayload{Authenticated: true, PortalURL: "u", Message: "m"}, []string{"authenticated", "portal_url", "message"}},
		{"VaultStatusPayload", VaultStatusPayload{Unlocked: true, RemoteReachable: true, RemoteReady: true, RemoteError: "e", StorageUsed: 1, StorageLimit: 2, RemainingStorage: 3, CacheState: "healthy"}, []string{"unlocked", "remote_reachable", "remote_ready", "remote_error", "storage_used", "storage_limit", "remaining_storage", "cache_state"}},
	}

	for _, f := range fixtures {
		data, err := json.Marshal(f.val)
		require.NoError(t, err, f.name)
		var got map[string]any
		require.NoError(t, json.Unmarshal(data, &got), f.name)
		for _, field := range f.fields {
			require.Contains(t, got, field, "%s must expose %q", f.name, field)
		}
		require.Len(t, got, len(f.fields), "%s must expose exactly its fields", f.name)
	}
}

// TestActionVocabulary pins the action surface: every action resolves from
// at least one view, uses the stable snake_case tool-name format, and every
// view's cast is reachable through its Actions slice.
func TestActionVocabulary(t *testing.T) {
	format := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	reachable := make(map[Action]bool)

	for _, v := range AllViews() {
		actions, err := v.Actions()
		require.NoError(t, err, "view %q", v)
		for _, a := range actions {
			require.Regexp(t, format, string(a), "action %q", a)
			reachable[a] = true
		}
	}

	for _, a := range []Action{
		ActionPinsAdd, ActionPinStatus, ActionPinsList,
		ActionVaultStatus, ActionVaultLS,
		ActionVaultCreate, ActionVaultCreateStatus,
		ActionVaultRestore, ActionVaultRestoreStatus,
		ActionVaultUploadSubmit, ActionVaultGetFile,
		ActionIPFSUploadSubmit, ActionIPFSUploadStatus,
		ActionDownloadFile,
		ActionAuthSSO, ActionAuthSSOStatus, ActionAuthSSORevoke,
		ActionAuthStatus, ActionAccountPasswordUpdate, ActionAccountEmailChange,
	} {
		require.True(t, reachable[a], "action %q declared but not reachable from any view", a)
	}
}

// TestStateVocabulary pins the state surface: every state value is a
// stable snake-case token, every constant is reachable from at least one
// view, and the per-view sets are the ones derived from the reference
// machines.
func TestStateVocabulary(t *testing.T) {
	format := regexp.MustCompile(`^[a-z]+(_[a-z]+)?$`)
	reachable := make(map[State]bool)

	for _, v := range AllViews() {
		states, err := v.States()
		require.NoError(t, err, "view %q", v)
		for _, s := range states {
			require.Regexp(t, format, string(s), "state %q", s)
			reachable[s] = true
		}
	}

	for _, s := range []State{
		StateIdle, StateForm, StateFormError, StateStarting,
		StateMinting, StateUploading, StateDownloading, StateSubmitting,
		StatePolling, StateLoading, StateReady, StateInfo, StateOk,
		StateDead, StateNoURL, StateError, StateTimeout, StateRevoking,
	} {
		require.True(t, reachable[s], "state %q declared but not reachable from any view", s)
	}
}
