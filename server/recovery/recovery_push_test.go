package recovery_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// ticketPushes are the pushes about tickets and epics, not sessions.
func ticketPushes(h *servertest.Harness) []servertest.Push {
	var out []servertest.Push
	for _, p := range h.Pushes.Sent() {
		if strings.HasPrefix(p.Tag, "ticket:") || strings.HasPrefix(p.Tag, "epic:") {
			out = append(out, p)
		}
	}
	return out
}

// waitTicketPushes polls for want pushes, then waits for a wrong extra one.
func waitTicketPushes(t *testing.T, h *servertest.Harness, want int) []servertest.Push {
	t.Helper()
	servertest.Poll(10*time.Second, func() bool { return len(ticketPushes(h)) >= want })
	if want == 0 || len(ticketPushes(h)) == want {
		time.Sleep(500 * time.Millisecond) // negative wait: a wrong extra push would show by now
	}
	return ticketPushes(h)
}

func startPushRecovery(t *testing.T, hold time.Duration, entry recovery.Entry, chat ...func(*server.Config)) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Catalog{Enabled: true, Entries: []recovery.Entry{entry}}
		c.RecoverySettings.NotifyHold = hold
		for _, opt := range chat {
			if opt != nil {
				opt(c)
			}
		}
	})
}

func personEntry(kind events.Kind) recovery.Entry {
	return recovery.Entry{ID: "TEST", Type: events.Paused, Kind: kind, Executor: recovery.ExecutorPerson, Enabled: true}
}

func TestRecoveryPush_PauseSendsChatWordsWithOrWithoutChat(t *testing.T) {
	t.Parallel()
	unmatched := recovery.Entry{ID: "TEST", Type: events.Paused, Kind: events.Spinning, Executor: recovery.ExecutorRule, Enabled: true}
	for name, chat := range map[string]func(*server.Config){
		"no chat": nil,
		"chat": func(c *server.Config) {
			c.Chat = notify.ServerChatConfig{SlackWebhookURL: "http://127.0.0.1:1", GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
		},
		"muted chat": func(c *server.Config) {
			gate := filepath.Join(t.TempDir(), "gate.json")
			if err := os.WriteFile(gate, []byte(`{"transports":{"slack":{"muted":true}}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			c.Chat = notify.ServerChatConfig{SlackWebhookURL: "http://127.0.0.1:1", GateStatePath: gate, FlushInterval: testFlushInterval}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := startPushRecovery(t, 0, unmatched, chat)
			if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
				t.Fatal(err)
			}
			got := waitTicketPushes(t, h, 1)
			if len(got) != 1 {
				t.Fatalf("pushes = %+v, want one", got)
			}
			path := filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")
			title, body := notify.PauseText("epic-a", path, "01", "needs-repair", ticketwrite.PauseChatDetail(path, events.IterationError, "boom"))
			want := servertest.Push{Title: title, Body: body, Path: "/t/proj:epic-a/01", Tag: "ticket:proj:epic-a/01", Urgent: false}
			if got[0] != want {
				t.Errorf("push = %+v, want %+v", got[0], want)
			}
			if !strings.Contains(got[0].Body, "boom") || !strings.Contains(got[0].Title, "epic-a/01 first") {
				t.Errorf("push = %+v, want Chat's sentence and the ticket title", got[0])
			}
		})
	}
}

func TestRecoveryPush_HeldPauseWaitsAndRecoveredPauseSendsNone(t *testing.T) {
	t.Parallel()
	t.Run("held then expired", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		entry := personEntry(events.IterationError)
		entry.Executor, entry.Authority = recovery.ExecutorRule, recovery.AuthorityLow
		entry.Remedy = func(recovery.Failure, recovery.Verbs) error { <-release; return nil }
		h := startPushRecovery(t, 600*time.Millisecond, entry, nil)
		t.Cleanup(func() { close(release) })
		if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		if got := ticketPushes(h); len(got) != 0 {
			t.Fatalf("pushes during the hold = %+v, want none", got)
		}
		if got := waitTicketPushes(t, h, 1); len(got) != 1 {
			t.Fatalf("pushes after the hold = %+v, want one", got)
		}
	})
	t.Run("recovered", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		var h *servertest.Harness
		entry := personEntry(events.IterationError)
		entry.Executor, entry.Authority = recovery.ExecutorRule, recovery.AuthorityLow
		entry.Remedy = func(recovery.Failure, recovery.Verbs) error { resume(t, h); close(done); return nil }
		h = startPushRecovery(t, time.Minute, entry, nil)
		if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		<-done
		if got := waitTicketPushes(t, h, 0); len(got) != 0 {
			t.Fatalf("a recovered pause pushed %+v", got)
		}
	})
}

func TestRecoveryPush_EscalationsSendHighUrgency(t *testing.T) {
	t.Parallel()
	t.Run("ticket", func(t *testing.T) {
		t.Parallel()
		h := startPushRecovery(t, time.Minute, personEntry(events.IterationError), nil)
		if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		// A person's entry releases the pause itself, so the pause push comes first.
		got := waitTicketPushes(t, h, 2)
		if len(got) != 2 || got[0].Urgent {
			t.Fatalf("pushes = %+v, want the pause, then the escalation", got)
		}
		if esc := got[1]; !esc.Urgent || esc.Tag != "ticket:proj:epic-a/01" || esc.Path != "/t/proj:epic-a/01" ||
			!strings.Contains(esc.Body, "Automatic recovery gave up.") {
			t.Fatalf("escalation = %+v, want urgent, the ticket tag and path", esc)
		}
	})
	t.Run("epic", func(t *testing.T) {
		t.Parallel()
		unmatched := recovery.Entry{ID: "TEST", Type: events.Paused, Kind: events.Spinning, Executor: recovery.ExecutorRule, Enabled: true}
		h := startPushRecovery(t, time.Minute, unmatched, nil)
		// A map epic refuses the investigate ticket's queueing, so recovery gives up.
		testutil.WriteFile(t, filepath.Join(h.TicketStore, "proj", "epic-a"), "ticket.md", "---\nkind: map\nstatus: open\n---\n# map\n")
		h.Server.Recovery().RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "every remaining ticket is paused"})
		got := waitTicketPushes(t, h, 1)
		if len(got) != 1 || !got[0].Urgent || got[0].Tag != "epic:proj:epic-a" || got[0].Path != "/e/proj:epic-a" ||
			!strings.Contains(got[0].Body, "All tickets are paused.") {
			t.Fatalf("pushes = %+v, want one urgent epic escalation", got)
		}
	})
}

// The push never consults focus: a tab on a session of the ticket changes nothing.
func TestRecoveryPush_SentWhileSessionOnScreen(t *testing.T) {
	t.Parallel()
	h := startPushRecovery(t, time.Minute, personEntry(events.Spinning), servertest.WithTestHolder())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{
		Name: "fix-build", Ticket: "proj:epic-a/01", Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind),
	})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	servertest.OpenTUI(ctx, t, h, "tab-1")
	servertest.ReportFocus(ctx, t, h, wire.FocusReport{Client: "tab-1", Focused: true, Session: res.Session.ID})
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	if got := waitTicketPushes(t, h, 1); len(got) != 1 {
		t.Fatalf("pushes = %+v, want one while the session is on screen", got)
	}
}
