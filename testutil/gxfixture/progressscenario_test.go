package main

import (
	"os"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

func TestRunningTicketTickScenario(t *testing.T) {
	t.Parallel()
	r := start(t, "running-ticket-tick")
	for id, harness := range map[string]string{"01": "claude", "02": "codex"} {
		if !testutil.Poll(90*time.Second, time.Second, func() bool {
			row := r.epicRow(t, "tick-epic", id)
			return row.RenderedStatus == "claimed" && row.Harness == harness
		}) {
			t.Fatalf("ticket %s: %+v, want claimed on %s", id, r.epicRow(t, "tick-epic", id), harness)
		}
	}
	if got := r.ticketStatus(t, "demo:tick-epic/03"); got == "done" {
		t.Fatalf("third ticket is %q before the release", got)
	}
	if err := os.Remove(r.HoldFile); err != nil {
		t.Fatal(err)
	}
	r.waitStatus(t, "demo:tick-epic/03", "done")
	r.stop(t)
}
