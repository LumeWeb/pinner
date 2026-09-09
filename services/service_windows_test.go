//go:build windows

package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"
)

func TestWindowsServiceNameDerivation(t *testing.T) {
	if got := newWindowsService(Config{Name: "pinner-mcp"}).name; got != "pinner-mcp" {
		t.Fatalf("name = %q, want pinner-mcp", got)
	}
	if got := newWindowsService(Config{}).name; got != defaultServiceName {
		t.Fatalf("default name = %q, want %q", got, defaultServiceName)
	}
}

func TestWindowsSystemDetects(t *testing.T) {
	sys := windowsSystem{}
	require.True(t, sys.Detect())
	svc := sys.New(Config{Name: "pinner-mcp"})
	_, ok := svc.(*windowsService)
	require.True(t, ok, "New should return a *windowsService")
}

func TestSCMStateToStatus(t *testing.T) {
	running := Status{Installed: true, Active: true, Ready: true, Summary: "active (running)"}
	inactive := Status{Installed: true, Summary: "inactive"}

	require.Equal(t, running, scmStateToStatus(svc.Running))
	require.Equal(t, running, scmStateToStatus(svc.StartPending))
	require.Equal(t, inactive, scmStateToStatus(svc.Stopped))
	require.Equal(t, inactive, scmStateToStatus(svc.StopPending))
	require.Equal(t, inactive, scmStateToStatus(svc.Paused))
	require.Equal(t, inactive, scmStateToStatus(svc.PausePending))
	require.Equal(t, inactive, scmStateToStatus(svc.ContinuePending))
}

func TestWindowsLogsXPathEscapesSourceName(t *testing.T) {
	// The service name is embedded in the /q: XPath [@Name='...'] literal.
	// A name containing a single quote must be escaped (doubled) so the query
	// stays a valid literal and still matches the real source; other characters
	// pass through verbatim (never mangled).
	cases := []struct {
		name, wantQuery string
	}{
		{"pinner-mcp", `/q:*[System[Provider[@Name='pinner-mcp']]]`},
		{"a]b'c", `/q:*[System[Provider[@Name='a]b''c']]]`},
		{"pin'er", `/q:*[System[Provider[@Name='pin''er']]]`},
	}
	for _, tc := range cases {
		var gotQuery string
		cfg := Config{Name: tc.name}
		cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
			for _, a := range args {
				if strings.HasPrefix(a, "/q:") {
					gotQuery = a
				}
			}
			return "", nil
		}
		require.NoError(t, newWindowsService(cfg).Logs(context.Background(), false))
		require.Equal(t, tc.wantQuery, gotQuery, "name %q", tc.name)
	}
}

func TestWindowsSetEnvInRegistryFailsOnCorruptEnvFile(t *testing.T) {
	// SCM has no runtime env-file fallback, so a corrupt env file must make
	// install fail before any registry write, rather than silently installing a
	// service with no credentials.
	dir := t.TempDir()
	envFile := filepath.Join(dir, "mcp.env")
	require.NoError(t, os.WriteFile(envFile, []byte("NOT A KEY VALUE LINE\n"), 0600))
	svc := newWindowsService(Config{Name: "pinner-mcp", EnvFile: envFile})
	err := svc.setEnvInRegistry()
	require.Error(t, err)
	require.Contains(t, err.Error(), "load service environment")
}

func TestWindowsSetEnvInRegistryToleratesMissingEnvFile(t *testing.T) {
	// A fresh install may legitimately have no env file yet; that must not
	// error (nothing is written to the registry).
	svc := newWindowsService(Config{Name: "pinner-mcp", EnvFile: filepath.Join(t.TempDir(), "gone.env")})
	require.NoError(t, svc.setEnvInRegistry())
}

func TestWindowsNewServiceDefaultsCommandSeam(t *testing.T) {
	// newWindowsService must default the OutputRun seam so Logs (the only
	// Windows command path) never invokes a nil function in production.
	svc := newWindowsService(Config{Name: "pinner-mcp"})
	require.NotNil(t, svc.cfg.OutputRun)
	// A caller-provided seam is preserved, not overwritten: exercising it
	// proves it is the same function (function values can't be == compared).
	sentinel := "custom-output"
	want := func(context.Context, string, ...string) (string, error) { return sentinel, nil }
	svc2 := newWindowsService(Config{Name: "pinner-mcp", OutputRun: want})
	out, err := svc2.cfg.OutputRun(context.Background(), "wevtutil", "qe")
	require.NoError(t, err)
	require.Equal(t, sentinel, out)
}

func TestWindowsMergedEnvVars(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "mcp.env")
	require.NoError(t, os.WriteFile(envFile, []byte("SHARED=file-value\nONLY_FILE=fs\n"), 0600))
	svc := newWindowsService(Config{
		Name:    "pinner-mcp",
		EnvVars: map[string]string{"SHARED": "vars-value", "ONLY_VARS": "vs"},
		EnvFile: envFile,
	})
	env, err := svc.mergedEnvVars()
	require.NoError(t, err)
	// Env file wins on collision; both sources contribute.
	require.Equal(t, "file-value", env["SHARED"])
	require.Equal(t, "fs", env["ONLY_FILE"])
	require.Equal(t, "vs", env["ONLY_VARS"])
}

func TestWindowsMergedEnvVarsRejectsCorruptEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "mcp.env")
	require.NoError(t, os.WriteFile(envFile, []byte("NOT A KEY VALUE LINE\n"), 0600))
	svc := newWindowsService(Config{Name: "pinner-mcp", EnvFile: envFile})
	_, err := svc.mergedEnvVars()
	require.ErrorContains(t, err, "load service environment")
}

func TestWindowsLogsFollowBailsOnPersistentFailure(t *testing.T) {
	// A permanently-failing wevtutil poll (missing wevtutil, access denied,
	// nonexistent source) must not spin on the 2s loop forever looking like an
	// idle tail. After maxEventPollFailures consecutive errors, Logs must
	// return the error instead of silent-looping.
	oldInterval, oldMax := eventPollInterval, maxEventPollFailures
	eventPollInterval, maxEventPollFailures = time.Millisecond, 3
	defer func() { eventPollInterval, maxEventPollFailures = oldInterval, oldMax }()

	cfg := Config{Name: "pinner-mcp"}
	polls := 0
	cfg.OutputRun = func(_ context.Context, _ string, _ ...string) (string, error) {
		polls++
		return "", errors.New("missing wevtutil")
	}

	err := newWindowsService(cfg).Logs(context.Background(), true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing wevtutil")
	require.Equal(t, maxEventPollFailures, polls, "must fail fast after the capped consecutive errors")
}

func TestWindowsLogsRoutesThroughOutputRun(t *testing.T) {
	// Logs must invoke wevtutil through the OutputRun seam (so a caller can
	// intercept/test it), with the /q: switch (wevtutil rejects a bare
	// positional XPath), a filter to the service's event source, newest-first,
	// bounded to the recent window.
	var calls [][]string
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, command string, args ...string) (string, error) {
		calls = append(calls, append([]string{command}, args...))
		return "", nil
	}
	require.NoError(t, newWindowsService(cfg).Logs(context.Background(), false))
	require.Len(t, calls, 1)
	a := calls[0]
	require.Equal(t, "wevtutil", a[0])
	require.Equal(t, "qe", a[1])
	require.Equal(t, "Application", a[2])
	require.Equal(t, `/q:*[System[Provider[@Name='pinner-mcp']]]`, a[3])
	require.Contains(t, a, "/f:text")
	require.Contains(t, a, "/rd:true")
	require.Contains(t, a, "/c:50")
}

func TestWindowsLogsFollowRecoversAfterLogRollover(t *testing.T) {
	// Regression (PR #32, round 2): the Application event log is circular.
	// After a rollover EventRecordIDs restart at values BELOW the pinned
	// cursor, so — exactly like real wevtutil — a follow query filtered by
	// `EventRecordID > cursor` matches NOTHING (the fake here honors the
	// XPath filter; the round-1 fake did not, so it could exercise only a
	// path real wevtutil never reaches). Because no returned batch can ever
	// reveal the wrap, the follow loop detects the stale cursor
	// independently: after maxEventPollEmpties consecutive empty polls it
	// probes the log's newest record id WITHOUT the boundary filter, and when
	// that id is lower than the cursor it resets the cursor so the next
	// (capped) poll re-anchors on the post-rollover events. Without that
	// reset the tail stays silent forever while returning success.
	oldInterval, oldMaxFail, oldMaxEmpty := eventPollInterval, maxEventPollFailures, maxEventPollEmpties
	eventPollInterval, maxEventPollFailures, maxEventPollEmpties = time.Millisecond, 10, 3
	defer func() { eventPollInterval, maxEventPollFailures, maxEventPollEmpties = oldInterval, oldMaxFail, oldMaxEmpty }()

	type event struct {
		id  int
		msg string
	}
	events := []event{{500, "seed"}}
	var pollQueries []string // main polls: EventRecordID > boundary
	var probeQueries []string
	cfg := Config{Name: "pinner-mcp"}
	cfg.OutputRun = func(_ context.Context, _ string, args ...string) (string, error) {
		query := ""
		for _, a := range args {
			if strings.HasPrefix(a, "/q:") {
				query = a
			}
		}
		if !strings.Contains(query, "EventRecordID > ") {
			// Unfiltered newest-record probe (/c:1): the newest id of the
			// current (possibly wrapped) event list.
			probeQueries = append(probeQueries, query)
			newest := events[len(events)-1]
			return winEventXML(newest.id, newest.msg), nil
		}
		pollQueries = append(pollQueries, query)
		if len(pollQueries) == 2 {
			// After poll 1 seeds the cursor at 500, the circular log wraps:
			// records restart below the pinned cursor, identical to a real
			// post-rollover Application log.
			events = []event{{41, "post-roll-a"}, {42, "post-roll-b"}}
		}
		// Honor the XPath boundary: return ONLY events with id > boundary
		// (an empty batch when every id is <= boundary), like real wevtutil.
		rest, _ := strings.CutPrefix(query, "/q:*[System[Provider[@Name='pinner-mcp'] and EventRecordID > ")
		boundary, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(rest), "]]"))
		if err != nil {
			return "", fmt.Errorf("unparsable query %q: %w", query, err)
		}
		var out strings.Builder
		for _, e := range events {
			if e.id > boundary {
				out.WriteString(winEventXML(e.id, e.msg))
			}
		}
		return out.String(), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	printed := captureStdout(t, func() {
		require.NoError(t, newWindowsService(cfg).Logs(ctx, true))
	})

	// Poll 1 seeds the cursor at 500; polls 2..4 hit the stale boundary and
	// must return empty (the fake honors the filter) until the empty-poll
	// probe fires and resets the cursor, after which a capped poll re-anchors.
	require.Contains(t, pollQueries[0], "EventRecordID > 0")
	require.Contains(t, pollQueries[1], "EventRecordID > 500")
	require.True(t, len(probeQueries) > 0, "empty-poll rollover probe must run")
	// The reset must drop the query boundary back to 0 (re-anchor) and then
	// advance to 42 (the new newest id) on the poll after it.
	reAnchor := -1
	for i, q := range pollQueries {
		if strings.Contains(q, "EventRecordID > 0") && i > 0 {
			reAnchor = i
			break
		}
	}
	require.NotEqual(t, -1, reAnchor, "cursor must reset to 0 after the rollover probe (got boundaries: %v)", pollQueries)
	require.Contains(t, pollQueries[reAnchor+1], "EventRecordID > 42",
		"the re-anchor poll must advance the cursor to the post-rollover newest id 42")
	// The post-rollover events must actually be emitted, not just re-queried.
	require.Contains(t, printed, "[41] post-roll-a")
	require.Contains(t, printed, "[42] post-roll-b")
}

// captureStdout runs fn with os.Stdout redirected into a pipe and returns what
// it printed. Logs emits through fmt.Print* directly, so the pipe is the only
// seam. (Tests never run in parallel in this package, so swapping os.Stdout is
// race-free.)
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = old }()
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	require.NoError(t, w.Close())
	return <-done
}

// winEventXML builds one wevtutil /f:xml event block carrying the given
// record id and single Data payload.
func winEventXML(id int, msg string) string {
	return fmt.Sprintf(
		`<Event><System><EventRecordID>%d</EventRecordID></System><EventData><Data>%s</Data></EventData></Event>`,
		id, msg)
}

func TestPrintNewEventsAdvancesCursorByID(t *testing.T) {
	// Two identical-message events with different record ids must both be
	// printed (dedup is by record id, never by content) and the cursor must
	// advance to the newest id — so a follow poll never re-prints and always
	// moves forward.
	out := `<Event><System><EventRecordID>100</EventRecordID></System><EventData><Data>heartbeat</Data></EventData></Event>` +
		`<Event><System><EventRecordID>101</EventRecordID></System><EventData><Data>heartbeat</Data></EventData></Event>`
	require.Equal(t, 101, printNewEvents(out))
	// Newer id with a multi-data payload advances further.
	out = `<Event xmlns='x'><System><EventRecordID> 300 </EventRecordID></System><EventData><Data>err</Data><Data>detail</Data></EventData></Event>`
	require.Equal(t, 300, printNewEvents(out))
	// Empty output keeps the cursor where it is.
	require.Equal(t, 0, printNewEvents(""))
}
