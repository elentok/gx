package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// startNotifyRecovery starts a server with a fake chat and one rule entry
// whose remedy is remedy; it returns the server and the chat bodies sent so far.
func startNotifyRecovery(t *testing.T, hold time.Duration, remedy recovery.Remedy) (*servertest.Harness, func() []string) {
	t.Helper()
	return startNotifyRecoveryWith(t, hold, recovery.Entry{
		ID: "TEST", Type: events.Paused, Kind: events.IterationError,
		Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Enabled: true, Remedy: remedy,
	})
}

func startNotifyRecoveryWith(t *testing.T, hold time.Duration, entry recovery.Entry) (*servertest.Harness, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Catalog{Enabled: true, Entries: []recovery.Entry{entry}}
		c.RecoverySettings.NotifyHold = hold
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	})
	return h, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

// pauseMessages are the sends that are about the pause, not the server starting.
func pauseMessages(sent []string) []string {
	return pauseMessagesOf(sent, events.IterationError)
}

func pauseMessagesOf(sent []string, kind events.Kind) []string {
	var out []string
	for _, b := range sent {
		if strings.Contains(b, string(kind)) {
			out = append(out, b)
		}
	}
	return out
}

// waitPauseMessages polls until a pause message of kind is sent, then waits long
// enough for a wrong second one to show.
func waitPauseMessages(t *testing.T, sent func() []string, kind events.Kind) []string {
	t.Helper()
	servertest.Poll(10*time.Second, func() bool { return len(pauseMessagesOf(sent(), kind)) > 0 })
	time.Sleep(1500 * time.Millisecond) // negative wait: a wrong second message would show by now
	return pauseMessagesOf(sent(), kind)
}

// resume moves ticket 01 on the way a remedy's relaunch would.
func resume(t *testing.T, h *servertest.Harness) {
	t.Helper()
	path := filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")
	if err := schema.UpdateTicket(path, func(tk *schema.Ticket) { tk.Status = schema.StatusClaimed }); err != nil {
		t.Error(err)
	}
}

// waitRecoveryState polls the snapshot until the paused ticket shows want.
func waitRecoveryState(t *testing.T, h *servertest.Harness, want string) {
	t.Helper()
	var got server.TicketInfo
	servertest.WaitFor(t, 10*time.Second, fmt.Sprintf("the ticket stuck with recovery %q", want), func() bool {
		snap, err := h.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, ti := range snap.Tickets {
			if ti.Address == "proj:epic-a/01" {
				got = ti
			}
		}
		return got.Status == "needs-repair" && got.Recovery == want
	})
}

func TestRecoveryNotify_SnapshotShowsTheHold(t *testing.T) {
	t.Parallel()
	t.Run("escalated after a failed remedy", func(t *testing.T) {
		release := make(chan struct{})
		h, _ := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
			<-release
			return errors.New("remedy refused")
		})
		if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		waitRecoveryState(t, h, server.RecoveryPending)
		close(release)
		waitRecoveryState(t, h, server.RecoveryEscalated)
	})
	t.Run("cleared when the hold expires", func(t *testing.T) {
		release := make(chan struct{})
		h, _ := startNotifyRecovery(t, 300*time.Millisecond, func(recovery.Failure, recovery.Verbs) error {
			<-release
			return nil
		})
		// Registered after the server, so it runs before the server's
		// cleanup waits for this remedy to end.
		t.Cleanup(func() { close(release) })
		if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		waitRecoveryState(t, h, "")
	})
}

func TestRecoveryState_FromInvestigations(t *testing.T) {
	t.Parallel()
	inv := func(status string) server.TicketInfo {
		return server.TicketInfo{Address: "p:e/01a", Type: "investigate", Status: status, Parent: "p:e/01"}
	}
	other := server.TicketInfo{Address: "p:e/01b", Type: "implement", Status: "claimed", Parent: "p:e/01"}
	for name, tc := range map[string]struct {
		held string
		all  []server.TicketInfo
		want string
	}{
		"none":              {"", []server.TicketInfo{other}, ""},
		"open":              {"", []server.TicketInfo{inv("open")}, server.RecoveryPending},
		"claimed":           {"", []server.TicketInfo{inv("claimed")}, server.RecoveryPending},
		"paused":            {"", []server.TicketInfo{inv("needs-repair")}, server.RecoveryEscalated},
		"landed":            {"", []server.TicketInfo{inv("done")}, ""},
		"open beats paused": {"", []server.TicketInfo{inv("needs-answer"), inv("claimed")}, server.RecoveryPending},
		"hold wins":         {server.RecoveryEscalated, []server.TicketInfo{inv("claimed")}, server.RecoveryEscalated},
	} {
		if got := server.RecoveryState(tc.held, "p:e/01", tc.all); got != tc.want {
			t.Errorf("%s: state = %q; want %q", name, got, tc.want)
		}
	}
}

// The investigation's own launch fails here (no herdr), which pauses it: that
// is recovery escalating. Its later merging leaves the pause's own state.
func TestRecoveryNotify_SnapshotFollowsTheInvestigation(t *testing.T) {
	t.Parallel()
	unmatched := recovery.Entry{ID: "TEST", Type: events.Paused, Kind: events.Spinning, Executor: recovery.ExecutorRule, Enabled: true}
	h, _ := startNotifyRecoveryWith(t, time.Minute, unmatched)
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	waitRecoveryState(t, h, server.RecoveryEscalated)
	child := filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01a-investigate.md")
	if err := schema.UpdateTicket(child, func(tk *schema.Ticket) { tk.Status = schema.StatusDone }); err != nil {
		t.Fatal(err)
	}
	waitRecoveryState(t, h, "")
}

func TestRecoveryNotify_RecoveredPauseSendsNothingAndIsCounted(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	var h *servertest.Harness
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
		resume(t, h)
		close(done)
		return nil
	})
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	<-done
	time.Sleep(1500 * time.Millisecond) // a wrongly sent message would show by now
	if got := pauseMessages(sent()); len(got) != 0 {
		t.Fatalf("a recovered pause sent %v", got)
	}
	if n := server.RecoveredCount(filepath.Join(h.TicketStore, "proj"), "epic-a"); n != 1 {
		t.Errorf("recovered count = %d, want 1", n)
	}
}

// A pause racing a merge finds the ticket done: the pause is refused and logged,
// and no chat message or recovery follows. The caller sees no error, since a
// done ticket has nothing left to pause.
func TestRecoveryNotify_PauseOfDoneTicketIsRefusedAndLogged(t *testing.T) {
	t.Parallel()
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
		t.Error("recovery ran for a refused pause")
		return nil
	})
	path := filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")
	if err := schema.UpdateTicket(path, func(tk *schema.Ticket) { tk.Status = schema.StatusDone }); err != nil {
		t.Fatal(err)
	}
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatalf("PauseTicket = %v, want nil", err)
	}
	time.Sleep(1500 * time.Millisecond) // a wrongly sent message would show by now
	if got := pauseMessages(sent()); len(got) != 0 {
		t.Errorf("a refused pause sent %v", got)
	}
	raw, _ := os.ReadFile(path)
	if tk, err := schema.ParseTicketFromRaw(string(raw), path); err != nil || tk.Status != schema.StatusDone {
		t.Errorf("ticket = %+v (%v), want still done", tk, err)
	}
	log, _ := os.ReadFile(server.LogPath(h.StateDir))
	if !strings.Contains(string(log), "proj:epic-a/01") || !strings.Contains(string(log), "done") {
		t.Errorf("server log has no refusal for proj:epic-a/01:\n%s", log)
	}
}

func TestRecoveryNotify_FailedRemedySendsOneMessageNamingTheEntry(t *testing.T) {
	t.Parallel()
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
		return errors.New("remedy refused")
	})
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	got := waitPauseMessages(t, sent, events.IterationError)
	if len(got) != 1 || !strings.Contains(got[0], "TEST") || !strings.Contains(got[0], "boom") {
		t.Fatalf("sends = %v, want one pause message naming entry TEST and the original reason", got)
	}
}

func TestRecoveryNotify_OkRemedyThatLeavesTheTicketPausedSendsTheMessage(t *testing.T) {
	t.Parallel()
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error { return nil })
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	got := waitPauseMessages(t, sent, events.IterationError)
	if len(got) != 1 || !strings.Contains(got[0], "TEST left it paused") || !strings.Contains(got[0], "boom") {
		t.Fatalf("sends = %v, want one pause message saying TEST left it paused", got)
	}
}

func TestRecoveryNotify_RecognizedPauseStillNotifiesAndIsNotCounted(t *testing.T) {
	t.Parallel()
	entries := recovery.Default().Entries
	r1 := entries[slices.IndexFunc(entries, func(e recovery.Entry) bool { return e.ID == "R1" })]
	h, sent := startNotifyRecoveryWith(t, time.Minute, r1)
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.Spinning, "paused and re-claimed 3 times within 5m0s"); err != nil {
		t.Fatal(err)
	}
	if got := waitPauseMessages(t, sent, events.Spinning); len(got) != 1 {
		t.Fatalf("sends = %v, want the spinning pause message", got)
	}
	if n := server.RecoveredCount(filepath.Join(h.TicketStore, "proj"), "epic-a"); n != 0 {
		t.Errorf("recovered count = %d, want 0", n)
	}
}

// escalationMessages are the sends that escalate a recovery.
func escalationMessages(sent []string) []string {
	var out []string
	for _, b := range sent {
		if strings.Contains(b, "recovery escalated") {
			out = append(out, b)
		}
	}
	return out
}

func TestRecoveryNotify_EveryEscalationSendsOneMessageNamingTheEntryAndTheReport(t *testing.T) {
	t.Parallel()
	rule := func(remedy recovery.Remedy) recovery.Entry {
		return recovery.Entry{
			ID: "TEST", Type: events.Paused, Kind: events.IterationError,
			Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Enabled: true, Remedy: remedy,
		}
	}
	ok := func(recovery.Failure, recovery.Verbs) error { return nil }
	person := rule(nil)
	person.Executor = recovery.ExecutorPerson
	unmatched := rule(ok)
	unmatched.Kind = events.Spinning
	for _, tc := range []struct {
		name  string
		entry recovery.Entry
		setup func(t *testing.T, h *servertest.Harness)
		want  string
	}{
		{"guard rail", rule(ok), func(t *testing.T, h *servertest.Harness) {
			ev := applied(events.IterationError)
			ev.Ticket = "01"
			if err := ralphloop.AppendEvent(filepath.Join(h.TicketStore, "proj"), "epic-a", ev); err != nil {
				t.Fatal(err)
			}
		}, "TEST"},
		{"person entry", person, nil, "TEST"},
		{"failed investigation", unmatched, func(t *testing.T, h *servertest.Harness) {
			// A map epic refuses the investigate ticket's queueing.
			testutil.WriteFile(t, filepath.Join(h.TicketStore, "proj", "epic-a"), "ticket.md", "---\nkind: map\nstatus: open\n---\n# map\n")
		}, "no match"},
		{"failed held remedy", rule(func(recovery.Failure, recovery.Verbs) error { return errors.New("remedy refused") }), nil, "TEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, sent := startNotifyRecoveryWith(t, time.Minute, tc.entry)
			if tc.setup != nil {
				tc.setup(t, h)
			}
			if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
				t.Fatal(err)
			}
			servertest.Poll(10*time.Second, func() bool { return len(escalationMessages(sent())) > 0 })
			time.Sleep(1500 * time.Millisecond) // a wrong second message would show by now
			got := escalationMessages(sent())
			if len(got) != 1 || !strings.Contains(got[0], "entry: "+tc.want) || !strings.Contains(got[0], "01-first.md") {
				t.Fatalf("sends = %v, want one escalation naming %q and the ticket's report", got, tc.want)
			}
		})
	}
}

func TestRecovery_AgentEntryMatchIsRecordedWithItsSignatureBeforeTheInvestigation(t *testing.T) {
	t.Parallel()
	entry := recovery.Entry{ID: "AGENT", Type: events.Paused, Kind: events.IterationError, Executor: recovery.ExecutorAgent, Enabled: true}
	h, _ := startNotifyRecoveryWith(t, time.Minute, entry)
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.TicketStore, "proj")
	var log []ralphloop.Event
	appliedAt := func() int {
		return slices.IndexFunc(log, func(ev ralphloop.Event) bool { return events.Type(ev.Type) == events.RecoveryApplied })
	}
	servertest.WaitFor(t, 10*time.Second, "recovery-applied", func() bool {
		log, _, _ = ralphloop.ReadEvents(dir, "epic-a")
		return appliedAt() >= 0
	})
	matchedAt := slices.IndexFunc(log, func(ev ralphloop.Event) bool { return events.Type(ev.Type) == events.RecoveryMatched })
	if matchedAt < 0 || matchedAt > appliedAt() {
		t.Fatalf("log = %+v, want recovery-matched before recovery-applied", log)
	}
	if m := log[matchedAt]; m.Reason != "AGENT" || m.Signature != entry.Signature() {
		t.Errorf("matched = %+v, want entry AGENT with signature %q", m, entry.Signature())
	}
}

func TestRecoveryNotify_SlowRecoverySendsTheOriginalPauseAfterTheHold(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	h, sent := startNotifyRecovery(t, 200*time.Millisecond, func(recovery.Failure, recovery.Verbs) error {
		<-release
		return nil
	})
	// Registered after the server, so it runs before the server's cleanup
	// waits for this remedy to end.
	t.Cleanup(func() { close(release) })
	if err := h.Server.PauseTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	var got []string
	servertest.Poll(10*time.Second, func() bool {
		got = pauseMessages(sent())
		return len(got) > 0
	})
	if len(got) != 1 || !strings.Contains(got[0], "boom") {
		t.Fatalf("sends = %v, want the original pause once the hold expired", got)
	}
}
