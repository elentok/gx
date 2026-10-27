package server

import (
	"testing"
	"time"
)

func TestSetLoopCardTime_QueuedFallsBackToQueuedAt(t *testing.T) {
	queuedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := &EpicListRow{Section: SectionInProgress, State: EpicListQueued, QueuedAt: queuedAt}
	setLoopCardTime(row, time.Time{}, time.Time{})
	if row.TimeKind != CardTimeElapsed || !row.TimeStart.Equal(queuedAt) {
		t.Errorf("time = %q %v, want elapsed from queued_at", row.TimeKind, row.TimeStart)
	}
	enq := queuedAt.Add(time.Hour)
	setLoopCardTime(row, time.Time{}, enq)
	if !row.TimeStart.Equal(enq) {
		t.Errorf("start = %v, want the enqueue time %v", row.TimeStart, enq)
	}
}
