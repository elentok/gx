package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// A restart settles a claimed ticket whose agent already reported finished,
// even when its pane still reads working (a leftover background task).
func TestReclaim_FinishedReportSettlesAWorkingPane(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLiveAgent(h)
	working := map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1", "agent_status": "working", "state_change_seq": 1}}
	for _, verb := range []string{"get", "wait"} {
		h.Herdr.Register("agent", verb, func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
			return working, herdrfake.Identities{}, nil
		})
	}
	_, ticketPath := leaveIteration(t, h, store, repo)
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.IterationFinishedAt = time.Now()
	}); err != nil {
		t.Fatal(err)
	}

	h.Restart(t)
	waitStatus(t, ticketPath, schema.StatusDone)
}

func TestPauseStuckFinishedClaims(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)
	epics, err := tickets.Load(store + "/proj")
	if err != nil {
		t.Fatal(err)
	}
	path := epics[0].Tickets[0].Path
	finishedAt := time.Now().Add(-time.Hour)
	if err := schema.UpdateTicket(path, func(tk *schema.Ticket) {
		tk.Status = schema.StatusClaimed
		tk.IterationStatus = schema.IterationStatusFinished
		tk.IterationFinishedAt = finishedAt
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Client.TicketChanged(context.Background(), "proj:epic-a/01"); err != nil {
		t.Fatal(err)
	}

	h.Server.PauseStuckFinishedClaims(finishedAt.Add(time.Minute))
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusClaimed {
		t.Fatalf("within the grace: status = %q, err = %v, want claimed", tk.Status, err)
	}

	h.Server.PauseStuckFinishedClaims(finishedAt.Add(time.Hour))
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusPaused {
		t.Errorf("past the grace: status = %q, want paused", tk.Status)
	}
}
