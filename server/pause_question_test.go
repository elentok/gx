package server_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

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
	if got.Type != tickets.EventParkQuestion || got.By != "gx" || got.ParkKind != string(kind) || got.Summary != summary || !strings.HasPrefix(got.Text, summary) {
		t.Errorf("event = %+v, want a by-gx %s question %q", got, kind, summary)
	}
}

func TestPause_ZeroCommitLogsGxQuestion(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	runToZeroCommitPause(t, h, "done")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	waitStatus(t, path, schema.StatusNeedsAnswer)
	assertGxQuestion(t, path, events.ZeroCommit,
		"The agent finished with no commits. Adopt it as done, or reopen the ticket?")
}

func TestPause_AmbiguousBaseLogsGxQuestion(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{BlockedBy: []string{"01", "02"}})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLaunch(h)
	if _, err := h.Server.ClaimTicket("proj:epic-a/03", "claude"); err != nil {
		t.Fatal(err)
	}
	assertGxQuestion(t, filepath.Join(store, "proj", "epic-a", "issues", "03-third.md"), events.AmbiguousBase,
		"Several blockers have different branches. Pick the base branch.")
}
