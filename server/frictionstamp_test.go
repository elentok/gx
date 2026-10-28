package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func appendPauseRound(t *testing.T, ticketPath string, close bool) {
	t.Helper()
	q, err := tickets.AppendLog(ticketPath, tickets.LogEvent{Type: tickets.EventPauseQuestion, PauseKind: "zero-commit"})
	if err != nil {
		t.Fatal(err)
	}
	if !close {
		return
	}
	if _, err := tickets.AppendLog(ticketPath, tickets.LogEvent{Type: tickets.EventPauseClosed, Question: q.Seq, Source: tickets.SourceCLI}); err != nil {
		t.Fatal(err)
	}
}

// A ticket that paused once and then merged carries a friction summary with that pause.
func TestFrictionStamp_MergedTicketCarriesItsPause(t *testing.T) {
	t.Parallel()
	f, tk := runMergeTicket(t, func(f *schedFixture, cwd string) string {
		appendPauseRound(t, f.ticketFile(t, "01"), true)
		testutil.WriteFile(t, cwd, "fix.txt", "fix")
		testutil.CommitAll(t, cwd, "epic: Fix typecheck/lint after rebase onto main")
		return "finished"
	})
	if tk.Status != schema.StatusDone {
		t.Fatalf("status = %s, want done", tk.Status)
	}
	// The stamp lands just after the done event, so wait for it.
	servertest.Poll(5*time.Second, func() bool {
		var err error
		if tk, err = schema.ParseTicket(f.ticketFile(t, "01")); err != nil {
			t.Fatal(err)
		}
		return tk.Friction != nil
	})
	if tk.Friction == nil || tk.Friction.PausesByReason["zero-commit"] != 1 || tk.Friction.PauseEndings["cli"] != 1 {
		t.Errorf("friction = %+v, want one zero-commit pause ended by cli", tk.Friction)
	}
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
	appendPauseRound(t, path, false)
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
