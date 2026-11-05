package ticketwrite_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestFinish_ZeroCommitPauseRecordsRun(t *testing.T) {
	t.Parallel()
	t.Run("pty", func(t *testing.T) {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		h := servertest.StartWithStore(t, store)
		servertest.RunToZeroCommitPause(t, h, "done")
		// The PTY agent's session id is generated, so any non-empty one will do.
		servertest.AssertPauseRecordsRun(t, store, events.Paused, "pty", agentrunner.Session{Label: "epic-a-iter-01"})
	})
	t.Run("headless", func(t *testing.T) {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		fake := servertest.NewTurnRunner(func(string, string) {})
		h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
		if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		servertest.WaitStatus(t, h.TicketStore+"/proj/epic-a/issues/01-first.md", schema.StatusLegacyAnswer)
		servertest.AssertPauseRecordsRun(t, store, events.Paused, "fake", agentrunner.Session{Label: "epic-a-iter-01", SessionID: "session-1"})
	})
}

func assertGxQuestion(t *testing.T, ticketPath string, kind events.Kind, summary string) {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("log = %+v, want one event", evs)
	}
	got := evs[0]
	// The text is the fixed summary, then the pause reason when it adds detail.
	if got.Type != tickets.EventPauseQuestion || got.By != "gx" || got.PauseKind != string(kind) || got.Summary != summary || !strings.HasPrefix(got.Text, summary) {
		t.Errorf("event = %+v, want a by-gx %s question %q", got, kind, summary)
	}
}

func TestPause_ZeroCommitLogsGxQuestion(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	servertest.RunToZeroCommitPause(t, h, "done")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	servertest.WaitStatus(t, path, schema.StatusLegacyAnswer)
	assertGxQuestion(t, path, events.ZeroCommit,
		"The agent finished with no commits. Adopt it as done, or reopen the ticket?")
}

func TestPause_AmbiguousBaseLogsGxQuestion(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{BlockedBy: []string{"01", "02"}})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	servertest.RegisterTurn(h)
	if _, err := h.Server.ClaimTicket("proj:epic-a/03", "claude"); err != nil {
		t.Fatal(err)
	}
	assertGxQuestion(t, filepath.Join(store, "proj", "epic-a", "issues", "03-third.md"), events.AmbiguousBase,
		"Several blockers have different branches. Pick the base branch.")
}

// A cancelled paused ticket carries the pause and a cli ending.
func TestFrictionStamp_CancelledPausedTicketEndsWithCli(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "work", "")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-work.md")
	fm := "---\nid: \"01\"\nstatus: paused\npause_reason: merge-conflict\ntype: implement\n---\n\n# work\n"
	if err := os.WriteFile(path, []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.AppendPauseRound(t, path, false)
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Friction == nil || tk.Friction.PausesByReason["zero-commit"] != 1 || tk.Friction.PauseEndings["cli"] != 1 {
		t.Errorf("friction = %+v, want one zero-commit pause ended by cli", tk.Friction)
	}
}
