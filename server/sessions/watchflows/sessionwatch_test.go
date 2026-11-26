package watchflows_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// report sends a web page's focus report and expects it to be accepted.
func (f *subWatch) report(t *testing.T, r wire.FocusReport) {
	t.Helper()
	body, _ := json.Marshal(r)
	code, res := servertest.WebPostJSON(t, f.base+"/v1/sessions/focus", string(body), f.cookie, "", servertest.JSONPost(f.base))
	var out wire.SessionResult
	if code != http.StatusOK || json.Unmarshal([]byte(res), &out) != nil || out.Refused {
		t.Fatalf("focus report = %d %s", code, res)
	}
}

// watching is the report of a page of client showing the session's working card.
func (f *subWatch) watching(client string, seq int64) wire.FocusReport {
	return wire.FocusReport{Client: client, Focused: true, Card: true, Session: f.id, Seq: seq}
}

// openWatching opens client's stream and reports its card as shown.
func (f *subWatch) openWatching(t *testing.T, client string) (closeTab func()) {
	t.Helper()
	closeTab = f.openTab(t, client)
	f.report(t, f.watching(client, 1))
	return closeTab
}

// sessionEvents counts, per client, the session watch events that arrive until
// done says so or d passes.
func (f *subWatch) sessionEvents(d time.Duration, done func(map[string]int) bool) map[string]int {
	got := map[string]int{}
	deadline := time.After(d)
	for !done(got) {
		select {
		case ev := <-f.evs:
			if ev.Type == wire.EventSessionSubagentChanged && ev.Address == f.id && ev.Call == "" {
				got[ev.Client]++
			}
		case <-deadline:
			return got
		}
	}
	return got
}

// wantEvent waits for one session watch event for each client.
func (f *subWatch) wantEvent(t *testing.T, what string, clients ...string) {
	t.Helper()
	all := func(got map[string]int) bool {
		for _, c := range clients {
			if got[c] == 0 {
				return false
			}
		}
		return true
	}
	if got := f.sessionEvents(5*time.Second, all); !all(got) {
		t.Fatalf("%s: events %v, want one for each of %v", what, got, clients)
	}
}

// wantQuiet fails on any session watch event in several ticks: no event marks
// the absence of a change.
func (f *subWatch) wantQuiet(t *testing.T, what string) {
	t.Helper()
	if got := f.sessionEvents(10*chatPollTestInterval, func(map[string]int) bool { return false }); len(got) > 0 {
		t.Fatalf("%s: events %v, want none", what, got)
	}
}

func twoClaudeCalls(now string) []string {
	return append(claudeCall(now), fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"a2","name":"Agent","input":{"description":"two","prompt":"find bar"}}]}}`, now))
}

func TestSessionWatch_NoWatchingClientSendsNothing(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	path := f.writeSubagent(t)
	f.openTab(t, "page-1")
	servertest.AppendLines(t, path, subFileLine)
	f.wantQuiet(t, "growth with no watching client")
}

func TestSessionWatch_StartSendsOneEventAndRepeatedReportNone(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	f.writeSubagent(t)
	f.openWatching(t, "page-1")
	f.wantEvent(t, "watch start", "page-1")
	f.wantQuiet(t, "after the start event")
	f.report(t, f.watching("page-1", 2))
	f.wantQuiet(t, "repeated report")
}

// secondSession creates another Claude session of f's server with a running
// subagent.
func (f *subWatch) secondSession(t *testing.T) *subWatch {
	t.Helper()
	res, err := f.h.Client.Sessions.Create(t.Context(), wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	other := *f
	other.id = res.Session.ID
	conv, _ := readMeta(t, f.h, other.id)["conversation_id"].(string)
	other.parent = filepath.Join(filepath.Dir(f.parent), conv+".jsonl")
	servertest.WriteTranscript(t, other.parent, claudeCall(time.Now().UTC().Format(time.RFC3339Nano))...)
	return &other
}

// blockTranscript turns the parent transcript into a pipe, so a read of it
// hangs until the test ends. blocked returns once a reader is hanging.
func (f *subWatch) blockTranscript(t *testing.T) (blocked func()) {
	t.Helper()
	if err := os.Remove(f.parent); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(f.parent, 0o644); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		var w *os.File
		// A non-blocking write open succeeds only once a reader has the pipe.
		servertest.WaitFor(t, 5*time.Second, "a read of the blocked transcript", func() bool {
			var err error
			w, err = os.OpenFile(f.parent, os.O_WRONLY|syscall.O_NONBLOCK, 0)
			return err == nil
		})
		t.Cleanup(func() {
			// Later opens must find a file, or they would hang on the pipe.
			empty := f.parent + ".empty"
			if err := os.WriteFile(empty, nil, 0o644); err != nil {
				t.Error(err)
			}
			if err := os.Rename(empty, f.parent); err != nil {
				t.Error(err)
			}
			if err := w.Close(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestSessionWatch_BlockedStartDoesNotDelayAnotherClient(t *testing.T) {
	t.Parallel()
	slow := startSubWatch(t, false, claudeCall)
	other := slow.secondSession(t)
	slow.openTab(t, "page-1")
	other.openTab(t, "page-2")
	blocked := slow.blockTranscript(t)

	var reports sync.WaitGroup
	// Registered before the pipe's release, so it runs after it.
	t.Cleanup(reports.Wait)
	reports.Go(func() { slow.report(t, slow.watching("page-1", 1)) })
	blocked()

	reports.Go(func() { other.report(t, other.watching("page-2", 1)) })
	other.wantEvent(t, "watch start while another client's start is blocked", "page-2")
}

func TestSessionWatch_StartWithNothingRunningSendsNothing(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCallDone)
	path := f.writeSubagent(t)
	f.openWatching(t, "page-1")
	f.wantQuiet(t, "start with a finished subagent")
	servertest.AppendLines(t, path, subFileLine)
	f.wantQuiet(t, "growth of a finished subagent's file")
}

func TestSessionWatch_GrowthSendsOneEventPerClientPerTick(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, twoClaudeCalls)
	f.openWatching(t, "page-1")
	f.openWatching(t, "page-2")
	f.wantEvent(t, "watch starts", "page-1", "page-2")
	f.wantQuiet(t, "no files yet")

	// One rename makes both files change in the same instant, so one tick sees
	// both.
	staged := *f
	staged.dir = filepath.Join(filepath.Dir(f.dir), "staged")
	one := staged.writeSubagent(t)
	staged.writeSubagentOf(t, "a2", "ag2")
	if err := os.Rename(staged.dir, f.dir); err != nil {
		t.Fatal(err)
	}
	got := f.sessionEvents(10*chatPollTestInterval, func(map[string]int) bool { return false })
	if got["page-1"] != 1 || got["page-2"] != 1 {
		t.Fatalf("events for two files at once = %v, want one for each client", got)
	}

	servertest.AppendLines(t, filepath.Join(f.dir, filepath.Base(one)), subFileLine)
	f.wantEvent(t, "growth", "page-1", "page-2")
}

func TestSessionWatch_ClaudeFileAppearingSendsAnEvent(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	f.openWatching(t, "page-1")
	f.wantEvent(t, "watch start with a running subagent and no file", "page-1")
	f.writeSubagent(t)
	f.wantEvent(t, "the file appeared", "page-1")
}

func TestSessionWatch_CodexRolloutAppearingSendsAnEvent(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, true, codexCall)
	f.openWatching(t, "page-1")
	f.wantEvent(t, "watch start with a running child and no rollout", "page-1")
	servertest.WriteTranscript(t, f.child, codexChildMsg)
	f.wantEvent(t, "the rollout appeared", "page-1")
}

func TestSessionWatch_SubagentStartingLaterIsPickedUp(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, func(string) []string { return nil })
	f.openWatching(t, "page-1")
	f.wantQuiet(t, "start with nothing running")
	servertest.AppendLines(t, f.parent, claudeCall(time.Now().UTC().Format(time.RFC3339Nano))...)
	f.writeSubagent(t)
	f.wantEvent(t, "a later subagent", "page-1")
}

func TestSessionWatch_EndsWhenTheClientStopsWatching(t *testing.T) {
	t.Parallel()
	for name, end := range map[string]func(f *subWatch, t *testing.T, closeTab func()){
		"not focused": func(f *subWatch, t *testing.T, _ func()) {
			r := f.watching("page-1", 2)
			r.Focused = false
			f.report(t, r)
		},
		"no card": func(f *subWatch, t *testing.T, _ func()) {
			r := f.watching("page-1", 2)
			r.Card = false
			f.report(t, r)
		},
		"closed stream": func(f *subWatch, t *testing.T, closeTab func()) {
			closeTab()
			servertest.WaitFor(t, 5*time.Second, "the stream's client to be gone", func() bool {
				return f.open(t, "a1").Reason == wire.ReasonSubagentWatchUnknownClient
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startSubWatch(t, false, claudeCall)
			path := f.writeSubagent(t)
			closeTab := f.openWatching(t, "page-1")
			f.wantEvent(t, "watch start", "page-1")
			end(f, t, closeTab)
			f.discardQueued()
			servertest.AppendLines(t, path, subFileLine)
			f.wantQuiet(t, "growth after the watch ended")
		})
	}
}

func TestSessionWatch_StaleReportNeitherStartsNorStops(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	path := f.writeSubagent(t)
	f.openTab(t, "page-1")
	hidden := f.watching("page-1", 5)
	hidden.Card = false
	f.report(t, hidden)
	f.report(t, f.watching("page-1", 4))
	f.wantQuiet(t, "stale report after a card-less one")

	f.report(t, f.watching("page-1", 6))
	f.wantEvent(t, "watch start", "page-1")
	hidden.Seq = 5
	f.report(t, hidden)
	servertest.AppendLines(t, path, subFileLine)
	f.wantEvent(t, "growth after a stale card-less report", "page-1")
}
