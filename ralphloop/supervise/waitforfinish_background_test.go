package supervise

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/transcript"
)

func backgroundTaskGateParams(scratchDir string) LaunchParams {
	return LaunchParams{
		Label: "iter-30", Agent: agent.AgentClaude, Pane: "pane-1", Ticket: "30",
		SessionCwd: "/repo/iter-30", Gate: NewGate(),
		ScratchDir: scratchDir, EpicName: "epic",
	}
}

// TestWaitForFinish_BackgroundTaskGateHoldsUntilResolved covers the core gate
// contract: a pane that debounce-confirms idle while its transcript still
// shows an outstanding-fresh background task must not be reported finished
// until that task resolves.
func TestWaitForFinish_BackgroundTaskGateHoldsUntilResolved(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 3 {
			status = transcript.BackgroundTaskResolved
		}
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}},
		}, nil
	}
	d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

	if err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if reads != 3 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 3 (held, held, resolved)", reads)
	}
	// One sleep from ConfirmFinished's own debounce, two more pacing the
	// gate's re-reads while the marker stayed outstanding-fresh, plus one more
	// from the recheck ConfirmFinished runs once the gate releases.
	if sleeps != 4 {
		t.Errorf("Sleep calls = %d, want 4: the gate must pace re-reads at SmartZonePollMs, not spin, and recheck idle once it releases", sleeps)
	}

	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var held, released int
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateHeld):
			held++
			if !strings.Contains(ev.Reason, "task-1") {
				t.Errorf("gate-held reason = %q, want it to name the task id", ev.Reason)
			}
		case string(eventsc.BackgroundTaskGateReleased):
			released++
		}
	}
	if held != 1 {
		t.Errorf("gate-held events = %d, want exactly 1 (once per task id, not once per poll tick)", held)
	}
	if released != 1 {
		t.Errorf("gate-released events = %d, want exactly 1", released)
	}
}

// TestWaitForFinish_RecoveryForceReleasesAHeldGate covers R10's hook: a task
// that never resolves stops holding once GateReleased reports true.
func TestWaitForFinish_RecoveryForceReleasesAHeldGate(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh}},
		}, nil
	}
	d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)
	d.GateReleased = func() bool { return reads >= 2 }

	if err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if reads != 2 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 2 (held, released by recovery)", reads)
	}
	events, _, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var types []string
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateHeld), string(eventsc.BackgroundTaskGateReleased):
			types = append(types, ev.Type+" "+ev.TaskID+" "+ev.Reason)
		}
	}
	want := []string{
		string(eventsc.BackgroundTaskGateHeld) + " task-1 background task task-1",
		string(eventsc.BackgroundTaskGateReleased) + " task-1 background task task-1: recovery forced the release",
	}
	if !slices.Equal(types, want) {
		t.Errorf("gate events = %q, want %q", types, want)
	}
}

// finishedTicket writes a claimed ticket whose agent reported finished at at,
// with the ticket's path set on the returned params.
func finishedTicket(t *testing.T, scratchDir string, at time.Time) LaunchParams {
	t.Helper()
	p := backgroundTaskGateParams(scratchDir)
	p.TicketPath = fixture.WriteTicket(t, "---\nid: \"01\"\nstatus: claimed\ntype: implement\niteration_status: finished\niteration_finished_at: "+
		at.UTC().Format(time.RFC3339)+"\n---\n# Ticket\n")
	return p
}

func gateEventTypes(t *testing.T, scratchDir string) []string {
	t.Helper()
	evs, _, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var out []string
	for _, ev := range evs {
		out = append(out, ev.Type+" "+ev.TaskID+" "+ev.Reason)
	}
	return out
}

// A finished report with an ended turn skips a task that started before the
// report and logs a released event for it.
func TestWaitForFinish_FinishedReportSkipsAnOlderTask(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	reportedAt := time.Now().Add(-time.Minute).Truncate(time.Second)
	var sleeps int
	read := func(string, string) (transcript.BackgroundTaskReading, error) {
		return transcript.BackgroundTaskReading{Markers: []transcript.BackgroundTaskMarker{
			{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh, StartedAt: reportedAt.Add(-time.Hour)},
		}}, nil
	}
	d := fakes.IdleBackgroundTaskDeps(read, &sleeps)

	if err := WaitForFinish(d, finishedTicket(t, scratchDir, reportedAt), "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	want := string(eventsc.BackgroundTaskGateReleased) + " task-1 agent reported finished, task task-1 ignored"
	got := gateEventTypes(t, scratchDir)
	if !slices.Contains(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

// A task started after the report and an open turn each hold, and each pauses
// background-task-gate at the one cap, with no expired event after it.
func TestWaitForFinish_HoldCapPausesATaskAfterTheReportAndAnOpenTurn(t *testing.T) {
	t.Parallel()
	cases := map[string]transcript.BackgroundTaskReading{
		"later task": {Markers: []transcript.BackgroundTaskMarker{
			{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh, StartedAt: time.Now().Add(time.Hour)},
		}},
		"open turn": {AwaitingTurnEnd: true, Markers: []transcript.BackgroundTaskMarker{
			{TaskID: "task-1", Status: transcript.BackgroundTaskResolved, StartedAt: time.Now().Add(-time.Hour)},
		}},
	}
	for name, reading := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scratchDir := fixture.EpicScratchDir(t, "epic")
			var sleeps int
			d := fakes.IdleBackgroundTaskDeps(func(string, string) (transcript.BackgroundTaskReading, error) { return reading, nil }, &sleeps)
			p := finishedTicket(t, scratchDir, time.Now().Add(-time.Minute))

			err := WaitForFinish(d, p, "sess-30")
			if _, ok := errors.AsType[*BlockedPausedError](err); !ok {
				t.Fatalf("WaitForFinish error = %v, want a pause", err)
			}
			if want := int(BackgroundTaskHoldCap/(SmartZonePollMs*time.Millisecond)) + 1; sleeps != want {
				t.Errorf("Sleep calls = %d, want %d (debounce plus one per poll up to the cap)", sleeps, want)
			}
			evs, _, _ := runlog.ReadEvents(scratchDir, "epic")
			var paused bool
			for _, ev := range evs {
				if ev.Type == string(eventsc.BackgroundTaskGateExpired) {
					t.Errorf("unexpected expired event %+v", ev)
				}
				if ev.Type == string(eventsc.Paused) && ev.Kind == string(eventsc.BackgroundTaskGate) && ev.TaskID == "task-1" {
					paused = true
				}
			}
			if !paused {
				t.Errorf("events = %q, want a background-task-gate pause naming task-1", gateEventTypes(t, scratchDir))
			}
		})
	}
}

// The env knob shortens the cap, so a short value pauses after one poll.
func TestWaitForFinish_HoldCapEnvShortensTheCap(t *testing.T) {
	t.Setenv(BackgroundTaskHoldCapEnv, "1ms")
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps int
	reading := transcript.BackgroundTaskReading{AwaitingTurnEnd: true}
	d := fakes.IdleBackgroundTaskDeps(func(string, string) (transcript.BackgroundTaskReading, error) { return reading, nil }, &sleeps)

	err := WaitForFinish(d, finishedTicket(t, scratchDir, time.Now().Add(-time.Minute)), "sess-30")
	if _, ok := errors.AsType[*BlockedPausedError](err); !ok {
		t.Fatalf("WaitForFinish error = %v, want a pause", err)
	}
	if want := 2; sleeps != want {
		t.Errorf("Sleep calls = %d, want %d (debounce plus one poll)", sleeps, want)
	}
}

// A Codex agent has no transcript marker, so the Claude gate never reads or
// holds: a Codex turn that ends with a terminal running finishes at turn end.
func TestWaitForFinish_BackgroundTaskGateDoesNotHoldACodexAgent(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh}},
		}, nil
	}
	d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)
	p := backgroundTaskGateParams(scratchDir)
	p.Agent = agent.AgentCodex

	if err := WaitForFinish(d, p, "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v, want a plain finish", err)
	}
	if reads != 0 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 0 for Codex", reads)
	}
	events, _, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type == string(eventsc.BackgroundTaskGateHeld) {
			t.Errorf("got a %s event for a Codex agent", ev.Type)
		}
	}
}

// TestWaitForFinish_BackgroundTaskGateReleaseRechecksIdle covers the false-
// finish this gate can otherwise produce: a background task resolving only
// proves that one task's own notification merged, not that the agent's
// overall turn is over — seeing the result is exactly when it's likely to
// resume real work. If the pane is back to "working" by the time the gate
// releases, WaitForFinish must not declare the iteration done off the stale
// idle signal from before the gate started holding; it must keep polling
// until a later idle genuinely holds up.
func TestWaitForFinish_BackgroundTaskGateReleaseRechecksIdle(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 2 {
			status = transcript.BackgroundTaskResolved
		}
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}},
		}, nil
	}
	// waits: 1 = outer loop's first wait (idle); 2 = that idle's own
	// ConfirmFinished recheck (still idle); 3 = the gate-release recheck this
	// fix adds (agent resumed work, not idle); 4 = outer loop's second wait,
	// once the agent is genuinely done; 5 = that second idle's own
	// ConfirmFinished recheck (idle; no gate re-hold since ReadBackgroundTasks
	// already reports resolved).
	r := &fakes.BlipRunner{Runner: fakes.IdleRunner("iter-30"), TimeoutOn: 3}
	d := deps.Deps{
		Runner:              r,
		ReadBackgroundTasks: readBackgroundTasks,
		Sleep:               func(time.Duration) { sleeps++ },
	}

	if err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if r.Waits < 4 {
		t.Errorf("Runner.Wait calls = %d, want at least 4: the gate-release recheck finding the pane busy must send WaitForFinish back around its outer poll loop", r.Waits)
	}

	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var held, released int
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateHeld):
			held++
		case string(eventsc.BackgroundTaskGateReleased):
			released++
		}
	}
	if held != 1 || released != 1 {
		t.Errorf("gate-held/-released events = %d/%d, want exactly 1/1 (only the first outstanding task ever gates)", held, released)
	}
}

// TestWaitForFinish_BackgroundTaskGateHoldsUntilTheWokenTurnEnds covers the
// race that paused a working agent as zero-commit: the task's notification
// woke the agent, but the pane still read idle through its first quick tool
// calls, so an idle recheck alone passed. The transcript's turn end decides.
func TestWaitForFinish_BackgroundTaskGateHoldsUntilTheWokenTurnEnds(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		return transcript.BackgroundTaskReading{
			Markers:         []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: transcript.BackgroundTaskResolved}},
			AwaitingTurnEnd: reads < 3,
		}, nil
	}
	d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

	if err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if reads != 3 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 3 (awaiting, awaiting, turn ended)", reads)
	}
}

// TestWaitForFinish_BackgroundTaskUnreadableAndUnsupportedNeverHoldGate covers
// the fail-open requirement: neither a transcript that failed to parse nor an
// agent with no transcript at all (Codex, via ReadBackgroundTasks == nil at
// the call site) is evidence, so WaitForFinish must conclude on the first
// debounce-confirmed idle exactly as it would with no reader wired at all.
func TestWaitForFinish_BackgroundTaskUnreadableAndUnsupportedNeverHoldGate(t *testing.T) {
	t.Parallel()

	t.Run("unreadable transcript fails open", func(t *testing.T) {
		t.Parallel()
		scratchDir := t.TempDir()
		var sleeps int
		readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
			return transcript.BackgroundTaskReading{FileStatus: transcript.BackgroundTaskUnreadable}, nil
		}
		d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

		if err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
			t.Fatalf("WaitForFinish: %v", err)
		}
		if sleeps != 1 {
			t.Errorf("Sleep calls = %d, want exactly 1 (ConfirmFinished's debounce only)", sleeps)
		}
	})

	t.Run("unsupported (Codex) agent never even calls the reader", func(t *testing.T) {
		t.Parallel()
		scratchDir := t.TempDir()
		var sleeps, reads int
		readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
			reads++
			return transcript.BackgroundTaskReading{FileStatus: transcript.BackgroundTaskUnsupported}, nil
		}
		d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)
		p := backgroundTaskGateParams(scratchDir)
		p.Agent = agent.AgentCodex

		if err := WaitForFinish(d, p, "sess-30"); err != nil {
			t.Fatalf("WaitForFinish: %v", err)
		}
		if reads != 0 {
			t.Errorf("ReadBackgroundTasks calls = %d, want 0: WaitForBackgroundTasks must not call it for Codex", reads)
		}
		if sleeps != 1 {
			t.Errorf("Sleep calls = %d, want exactly 1 (ConfirmFinished's debounce only)", sleeps)
		}
	})
}

// TestWaitForFinish_BackgroundTaskGateAccumulatesAgainstFinishTimeoutMs covers
// the elapsed-time contract: time spent holding the gate must accumulate
// against a caller-set FinishTimeoutMs (never reset), so a bound shorter than
// the ~2h aged-out cap still fires while the gate is held.
func TestWaitForFinish_BackgroundTaskGateAccumulatesAgainstFinishTimeoutMs(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var sleeps int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh}},
		}, nil
	}
	d := fakes.IdleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

	p := backgroundTaskGateParams(scratchDir)
	p.FinishTimeoutMs = SmartZonePollMs / 2

	err := WaitForFinish(d, p, "sess-30")
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("WaitForFinish error = %v, want a FinishTimeoutMs timeout while the gate held", err)
	}
}
