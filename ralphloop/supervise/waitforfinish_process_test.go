package supervise

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/transcript"
)

func outstandingTask(string, string) (transcript.BackgroundTaskReading, error) {
	return transcript.BackgroundTaskReading{Markers: []transcript.BackgroundTaskMarker{
		{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh, StartedAt: time.Now().Add(time.Hour)},
	}}, nil
}

func eventCount(t *testing.T, scratchDir string, typ eventsc.Type) int {
	t.Helper()
	evs, _, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	return len(slices.DeleteFunc(evs, func(ev runlog.Event) bool { return ev.Type != string(typ) }))
}

// A live process with a quiet output file never releases: the wait logs one
// still-waiting line per check and pauses background-task-gate at the cap.
func TestWaitForBackgroundTasks_LiveProcessWaitsToTheCap(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps int
	d := fakes.IdleBackgroundTaskDeps(outstandingTask, &sleeps)
	d.ReadBackgroundProcess = func(string, string) (deps.BackgroundProcess, error) {
		return deps.BackgroundProcess{Known: true, Alive: true, OutputBytes: 0}, nil
	}

	err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30")
	if _, ok := errors.AsType[*BlockedPausedError](err); !ok {
		t.Fatalf("WaitForFinish error = %v, want a pause", err)
	}
	if got, want := eventCount(t, scratchDir, eventsc.BackgroundTaskStillWaiting), int(BackgroundTaskHoldCap/BackgroundTaskCheckEvery); got != want {
		t.Errorf("still-waiting lines = %d, want %d", got, want)
	}
	evs, _, _ := runlog.ReadEvents(scratchDir, "epic")
	if !slices.ContainsFunc(evs, func(ev runlog.Event) bool {
		return ev.Type == string(eventsc.Paused) && ev.Kind == string(eventsc.BackgroundTaskGate)
	}) {
		t.Errorf("events = %q, want a background-task-gate pause", gateEventTypes(t, scratchDir))
	}
}

// A process that is gone with no notification releases the wait at the next
// check, not at the cap.
func TestWaitForBackgroundTasks_GoneProcessWithoutNotificationReleases(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps int
	d := fakes.IdleBackgroundTaskDeps(outstandingTask, &sleeps)
	d.ReadBackgroundProcess = func(string, string) (deps.BackgroundProcess, error) {
		return deps.BackgroundProcess{Known: true, Alive: false}, nil
	}

	if err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if want := int(BackgroundTaskCheckEvery / (SmartZonePollMs * time.Millisecond)); sleeps > want+3 {
		t.Errorf("Sleep calls = %d, want the release at the first 10 minute check (about %d)", sleeps, want)
	}
	got := gateEventTypes(t, scratchDir)
	if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "process gone, no notification") }) {
		t.Errorf("events = %q, want a process-gone release", got)
	}
}

// An unknown process state is never evidence the task is gone.
func TestWaitForBackgroundTasks_UnknownProcessStateKeepsWaiting(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
	var sleeps int
	d := fakes.IdleBackgroundTaskDeps(outstandingTask, &sleeps)
	d.ReadBackgroundProcess = func(string, string) (deps.BackgroundProcess, error) {
		return deps.BackgroundProcess{}, nil
	}

	err := WaitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30")
	if _, ok := errors.AsType[*BlockedPausedError](err); !ok {
		t.Fatalf("WaitForFinish error = %v, want a pause", err)
	}
}
