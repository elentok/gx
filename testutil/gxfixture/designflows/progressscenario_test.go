package designflows

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
	if got := r.TicketStatus(t, "demo:tick-epic/03"); got == "done" {
		t.Fatalf("third ticket is %q before the release", got)
	}
	if err := os.Remove(r.HoldFile); err != nil {
		t.Fatal(err)
	}
	r.WaitStatus(t, "demo:tick-epic/03", "done")
	r.Stop(t)
}

func TestRecoveryScenarios_EachStartsAndPrintsItsJSON(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"hung-pane-twice", "hung-pane-once", "hung-pane-twice-codex", "codex-drops-prompt", "verify-notes", "summary-notes", "map-evolve", "live-task-gate", "live-task-gate-codex", "live-task-gate-dirty", "slow-holder", "holder-unreachable", "classifier-outage"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := start(t, name)
			if r.WebURL == "" {
				t.Errorf("%s printed no web_url: %+v", name, r)
			}
			r.Stop(t)
		})
	}
}
