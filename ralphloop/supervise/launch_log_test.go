package supervise

import (
	"os"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
)

// unwritableLogParams points the run log at a directory, so appending to it
// fails with something other than a missing file.
func unwritableLogParams(t *testing.T) LaunchParams {
	t.Helper()
	scratch := t.TempDir()
	if err := os.MkdirAll(runlog.RunLogPath(scratch, "epic"), 0o755); err != nil {
		t.Fatal(err)
	}
	return LaunchParams{ScratchDir: scratch, EpicName: "epic", Ticket: "01"}
}

func TestLogGateEvent_ReturnsWriteError(t *testing.T) {
	t.Parallel()
	p := unwritableLogParams(t)
	if err := p.LogGateEvent(events.BackgroundTaskGateHeld, "s", "task-1", "r"); err == nil {
		t.Fatal("LogGateEvent returned nil for a failing run-log write")
	}
}

func TestLogAgentEvent_ReturnsWriteError(t *testing.T) {
	t.Parallel()
	p := unwritableLogParams(t)
	if err := p.LogAgentEvent("x", "s", "r"); err == nil {
		t.Fatal("LogAgentEvent returned nil for a failing run-log write")
	}
}
