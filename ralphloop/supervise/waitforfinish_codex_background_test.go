package supervise

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/testutil/runnerfake"
)

// codexTitleRunner is a Codex session whose title heuristic reads "Ready": it
// is working with a background terminal, so no wait ever sees it idle.
type codexTitleRunner struct {
	*runnerfake.Runner
	waits int
}

func (r *codexTitleRunner) Wait(agentrunner.Session, []agentrunner.State, time.Duration) (agentrunner.Status, error) {
	r.waits++
	return agentrunner.Status{}, agentrunner.ErrTimeout
}

func (r *codexTitleRunner) Status(agentrunner.Session) (agentrunner.Status, error) {
	return agentrunner.Status{State: agentrunner.StateWorking, Background: true}, nil
}

// A Codex session on its title heuristic is held up to the one cap, then the
// ticket pauses background-task-gate.
func TestWaitForFinish_CodexTitleHeuristicHeldThenPausedAtTheCap(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	r := &codexTitleRunner{Runner: fakes.IdleRunner("iter-30")}
	d := deps.Deps{Runner: r, Sleep: func(time.Duration) {}}
	p := finishedTicket(t, scratchDir, time.Now())
	p.Agent = agent.AgentCodex

	err := WaitForFinish(d, p, "sess-30")
	if _, ok := errors.AsType[*BlockedPausedError](err); !ok {
		t.Fatalf("WaitForFinish error = %v, want a pause", err)
	}
	if want := int(BackgroundTaskHoldCap / (SmartZonePollMs * time.Millisecond)); r.waits != want {
		t.Errorf("Wait calls = %d, want %d (one per poll up to the cap)", r.waits, want)
	}
	evs, _, _ := runlog.ReadEvents(scratchDir, "epic")
	var paused bool
	for _, ev := range evs {
		paused = paused || ev.Type == string(eventsc.Paused) && ev.Kind == string(eventsc.BackgroundTaskGate)
	}
	if !paused {
		t.Errorf("events = %q, want a background-task-gate pause", gateEventTypes(t, scratchDir))
	}
}
