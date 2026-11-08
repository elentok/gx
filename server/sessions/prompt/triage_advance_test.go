package prompt_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// triageRunEnd ends a turn whose last message is the run-end line and waits
// for the Actionable that line raises.
func triageRunEnd(t *testing.T, step string, done bool) *grillingFixture {
	t.Helper()
	f, _ := servertest.StartTriageSession(t, step, done)
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	return f
}

func TestTriageRunEnd_VerifyRaisesContinueAndAutoAdvances(t *testing.T) {
	t.Parallel()
	f := triageRunEnd(t, "verify", false)
	got := sessionSnapshot(t, f)
	if len(got.Actionable.Action) != 2 || got.Actionable.Action[1] != "/gx-triage-follow-ups --project proj" {
		t.Fatalf("action = %q, want /clear then the Triage skill", got.Actionable.Action)
	}
	waitForSubmitted(t, f.Conv, "\"/clear\"\n\"/gx-triage-follow-ups --project proj\"\n")
}

func TestTriageRunEnd_WriteWithPendingRowsRaisesContinue(t *testing.T) {
	t.Parallel()
	f := triageRunEnd(t, "write", false)
	if got := sessionSnapshot(t, f); len(got.Actionable.Action) == 0 {
		t.Fatalf("actionable = %+v, want a Continue", got.Actionable)
	}
}

func TestTriageRunEnd_FinalStepsRaiseNoContinueAndNoCountdown(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, step string
		done       bool
	}{
		{"plan run end, step already approve", "approve", false},
		{"write with no pending rows", "write", true},
		{"done", "done", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := triageRunEnd(t, tc.step, tc.done)
			got := sessionSnapshot(t, f)
			if len(got.Actionable.Action) != 0 || got.AutoAdvanceAt != nil {
				t.Fatalf("actionable = %+v, advance at %v, want the line alone", got.Actionable, got.AutoAdvanceAt)
			}
			time.Sleep(2 * servertest.CountdownDelay) // negative wait: a countdown would have sent by now
			if n := servertest.AutoSends(t, f); n != 0 {
				t.Errorf("auto sends = %d, want none", n)
			}
		})
	}
}
