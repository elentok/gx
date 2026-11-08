package ticketops_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// pauseOnGate pauses the waiting run's ticket on the background-task gate;
// a task ID makes it the live-task variant, none the dirty-worktree one.
func pauseOnGate(t *testing.T, h interface {
	PauseAs(string, events.Kind, string) error
}, dir, taskID string) {
	t.Helper()
	if err := h.PauseAs(answerAddr, events.BackgroundTaskGate, "waited on background task"); err != nil {
		t.Fatal(err)
	}
	if taskID == "" {
		return
	}
	ev := runlog.Event{Type: string(events.Paused), Kind: string(events.BackgroundTaskGate), Ticket: "01", Address: answerAddr, TaskID: taskID}
	if err := runlog.AppendEvent(dir, "epic-a", ev); err != nil {
		t.Fatal(err)
	}
}

// Retry of a live-task gate pause stops the waiting agent instead of
// attaching to it, and reopens the ticket.
func TestGateRetry_LiveTaskReplacesTheAgent(t *testing.T) {
	t.Parallel()
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseOnGate(t, h.Server, h.TicketStore+"/proj", "task-1")
	first, ok, err := fake.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}

	res, err := h.Client.Tickets.Retry(context.Background(), answerAddr, false)
	if err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v; want accepted", res, err)
	}
	// The scheduler launches the fresh agent once the stopped run leaves the
	// registry; the adopted run of this rig never does, so the reopen is the edge.
	if s, ok, err := fake.Find("epic-a-iter-01"); err != nil || (ok && s.ID == first.ID) {
		t.Errorf("old agent alive = %v, %v; want stopped", ok, err)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	if n := len(fake.Prompts("epic-a-iter-01")); n != 0 {
		t.Errorf("prompts = %d, want none: attaching to the waiting agent does nothing", n)
	}
}

// Retry of the dirty-worktree variant continues the live agent's work.
func TestGateRetry_DirtyWorktreePromptsTheLiveAgent(t *testing.T) {
	t.Parallel()
	h, _, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseOnGate(t, h.Server, h.TicketStore+"/proj", "")
	first, _, _ := fake.Find("epic-a-iter-01")

	res, err := h.Client.Tickets.Retry(context.Background(), answerAddr, false)
	if err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v; want accepted", res, err)
	}
	if again, ok, err := fake.Find("epic-a-iter-01"); err != nil || !ok || again.ID != first.ID {
		t.Errorf("session = %+v, %v, %v; want the same live agent", again, ok, err)
	}
	if n := len(fake.Prompts("epic-a-iter-01")); n != 1 {
		t.Errorf("prompts = %d, want 1", n)
	}
}

// Start over on a gate pause stops the agent and keeps the branch's commits
// under a backup ref.
func TestGateStartOver_StopsTheAgentAndKeepsABackup(t *testing.T) {
	t.Parallel()
	h, _, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseOnGate(t, h.Server, h.TicketStore+"/proj", "task-1")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := h.Client.Admin.Repair(ctx, "start-over", wire.RepairRequest{Address: answerAddr, Reason: "task hung"})
	if err != nil || res.Refused {
		t.Fatalf("start-over = %+v, %v; want accepted", res, err)
	}
	var out struct {
		AtticRef *string `json:"attic_ref"`
	}
	if err := json.Unmarshal(res.Data, &out); err != nil || out.AtticRef == nil {
		t.Errorf("start-over data = %s (%v); want a backup ref", res.Data, err)
	}
	if _, ok, err := fake.Find("epic-a-iter-01"); err != nil || ok {
		t.Errorf("agent alive = %v, %v after start over; want stopped", ok, err)
	}
}
