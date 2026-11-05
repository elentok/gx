package fakes

import (
	"testing"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

func TestRecordingSink_CapturesIterationFinishedStatsAndEpicCompleteElapsedSeconds(t *testing.T) {
	t.Parallel()
	sink := &RecordingSink{}

	stats := runlog.IterationStats{ElapsedSeconds: 42, PeakContextTokens: 12345, InProgress: 2, Completed: 3, Total: 5}
	sink.IterationFinished(tickets.Ticket{Identifier: "01"}, "epic", stats)
	if sink.LastIterationStats != stats {
		t.Errorf("LastIterationStats = %+v, want %+v", sink.LastIterationStats, stats)
	}

	sink.EpicComplete("epic", 5, 300)
	if sink.LastEpicElapsedSeconds != 300 {
		t.Errorf("LastEpicElapsedSeconds = %d, want 300", sink.LastEpicElapsedSeconds)
	}
}
