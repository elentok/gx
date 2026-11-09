package launchtest_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func countPauseEvents(t *testing.T, ticketPath, typ string) int {
	t.Helper()
	log, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range log {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

func TestStormStop_PausesProcessStormOnceWhicheverPathGetsThereFirst(t *testing.T) {
	t.Parallel()
	h, ticketPath := startWaitingRestart(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	tracked, ok := h.Server.Registry().Tracked(servertest.AnswerAddr)
	if !ok {
		t.Fatal("run not tracked")
	}
	stopper := h.Server.Budget().Stopper()

	if err := stopper.StormStop(tracked, 812, 500); err != nil {
		t.Fatal(err)
	}
	// The run's own settle arrives after the kill path already paused it.
	if stopping, err := stopper.SettleStop(tracked); err != nil || !stopping {
		t.Fatalf("SettleStop = %v, %v; want stopping with no second pause", stopping, err)
	}

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusPaused || tk.PauseReason != schema.PauseProcessStorm {
		t.Errorf("ticket = status %q reason %q, want paused process-storm", tk.Status, tk.PauseReason)
	}
	log, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	var detail string
	for _, ev := range log {
		if ev.Type == tickets.EventPauseRepair {
			detail += ev.Reason
		}
	}
	if !strings.Contains(detail, "812") || !strings.Contains(detail, "500") {
		t.Errorf("pause detail = %q, want count and threshold", detail)
	}
	if n := countPauseEvents(t, ticketPath, tickets.EventPauseRepair); n != 1 {
		t.Errorf("pause.repair events = %d, want 1", n)
	}
	if n := countPauseEvents(t, ticketPath, tickets.EventPauseClosed); n > 1 {
		t.Errorf("pause.closed events = %d, want at most 1", n)
	}
}

func TestStormStop_SettlePathFirstPausesOnceAndStartsNoRecovery(t *testing.T) {
	t.Parallel()
	h, ticketPath := startWaitingRestart(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	tracked, _ := h.Server.Registry().Tracked(servertest.AnswerAddr)
	stopper := h.Server.Budget().Stopper()

	h.Server.Registry().MarkStop(tracked.Address, runs.Stop{
		Kind: events.ProcessStorm, Reason: schema.ProcessStormReason(812, 500), CloseReason: ticketops.CloseProcessStorm,
	})
	if stopping, err := stopper.SettleStop(tracked); err != nil || !stopping {
		t.Fatalf("SettleStop = %v, %v", stopping, err)
	}
	if err := stopper.StormStop(tracked, 812, 500); err != nil {
		t.Fatal(err)
	}
	if n := countPauseEvents(t, ticketPath, tickets.EventPauseRepair); n != 1 {
		t.Errorf("pause.repair events = %d, want 1", n)
	}
	if got := len(h.Server.Runs()); got > 1 {
		t.Errorf("runs = %d, want no recovery run", got)
	}
}
