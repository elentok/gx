package server

import (
	"testing"
	"time"
)

func TestCardTimeFor_QueuedFallsBackToQueuedAt(t *testing.T) {
	t.Parallel()
	queuedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := EpicListRow{Section: SectionInProgress, State: EpicListQueued, QueuedAt: queuedAt}
	kind, start := cardTimeFor(row, cardTimeSources{})
	if kind != CardTimeElapsed || !start.Equal(queuedAt) {
		t.Errorf("time = %q %v, want elapsed from queued_at", kind, start)
	}
	enq := queuedAt.Add(time.Hour)
	_, start = cardTimeFor(row, cardTimeSources{enqueuedAt: enq})
	if !start.Equal(enq) {
		t.Errorf("start = %v, want the enqueue time %v", start, enq)
	}
}

func TestCardTimeFor_SectionMapsToKind(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name string
		row  EpicListRow
		src  cardTimeSources
		kind CardTimeKind
	}{
		{"needs you", EpicListRow{Section: SectionNeedsYou, WaitingSince: at}, cardTimeSources{}, CardTimeWaiting},
		{"running", EpicListRow{Section: SectionInProgress, State: EpicListRunning}, cardTimeSources{startedAt: at}, CardTimeElapsed},
		{"ready to queue", EpicListRow{Section: SectionReadyToQueue, CreatedAt: at}, cardTimeSources{}, CardTimeCreated},
		{"ready to design", EpicListRow{Section: SectionReadyToDesign, CreatedAt: at}, cardTimeSources{}, CardTimeCreated},
		{"finished", EpicListRow{Section: SectionFinished, CompletedAt: at}, cardTimeSources{}, CardTimeFinished},
		{"no start", EpicListRow{Section: SectionNeedsYou}, cardTimeSources{}, ""},
		{"no section", EpicListRow{CreatedAt: at}, cardTimeSources{}, ""},
	} {
		kind, start := cardTimeFor(tc.row, tc.src)
		if kind != tc.kind || (kind != "" && !start.Equal(at)) {
			t.Errorf("%s: time = %q %v, want %q %v", tc.name, kind, start, tc.kind, at)
		}
	}
}
