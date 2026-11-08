package ralphloop

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// A task that goes live after the finish was confirmed, right before the
// zero-commit pause, is waited out instead of pausing: the commits it lets the
// agent land then merge.
func TestFinishIteration_PauseRereadsSignalAndWaitsWhenTaskIsLive(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, _, _ := fakes.FakeDeps()
	var counts, liveReads atomic.Int32
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		// The finish count and its recheck see none; the recount after the wait sees one.
		if counts.Add(1) <= 2 {
			return 0, nil
		}
		return 1, nil
	}
	d.ReadBackgroundTasks = func(string, string) (transcript.BackgroundTaskReading, error) {
		status := transcript.BackgroundTaskResolved
		if counts.Load() == 2 && liveReads.Add(1) == 1 {
			status = transcript.BackgroundTaskOutstandingFresh
		}
		return transcript.BackgroundTaskReading{Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}}}, nil
	}

	out := driveIteration(t, d, scratchDir, path, fakes.NewRecordingEventSink())

	if liveReads.Load() == 0 {
		t.Fatal("the pause never re-read the signal")
	}
	if !out.Merged {
		t.Errorf("outcome = %+v, want merged once the task finished", out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if fakes.RawIsPaused(t, string(raw), schema.PauseClassAnswer) || !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket paused or not done:\n%s", raw)
	}
}
