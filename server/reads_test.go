package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/servertest"
)

func TestReads_HistoryLocksAndProjects(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	projectDir := filepath.Join(store, "proj")
	for _, ticket := range []string{"01", "02", "01"} {
		if err := ralphloop.AppendEvent(projectDir, "epic-a", ralphloop.Event{Type: string(events.CherryPicked), Ticket: ticket}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ralphloop.AcquireLandLockFor(filepath.Join(projectDir, "epic-a"), "epic-a", "01"); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()

	hist, err := h.Client.History(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Events) != 2 || hist.Events[0].Address != "proj:epic-a/01" {
		t.Errorf("history = %+v", hist)
	}
	if _, err := h.Client.History(ctx, "nope:epic-a/01"); err == nil {
		t.Error("history of an unknown project should fail")
	}

	locks, err := h.Client.Locks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 1 || locks[0].Kind != "land" || locks[0].Epic != "proj:epic-a" || locks[0].Pid != os.Getpid() || !locks[0].Alive || locks[0].Ticket != "01" {
		t.Errorf("locks = %+v", locks)
	}

	projects, err := h.Client.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "proj" || projects[0].Tickets["open"] != 2 {
		t.Errorf("projects = %+v", projects)
	}
}
