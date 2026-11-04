package server

import (
	"github.com/elentok/gx/server/wire"
	"testing"
	"time"
)

func TestCardTimeFor_QueuedFallsBackToQueuedAt(t *testing.T) {
	t.Parallel()
	queuedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := wire.EpicListRow{Section: wire.SectionImplementing, State: wire.EpicListQueued, QueuedAt: queuedAt}
	kind, start := cardTimeFor(row, cardTimeSources{})
	if kind != wire.CardTimeElapsed || !start.Equal(queuedAt) {
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
		row  wire.EpicListRow
		src  cardTimeSources
		kind wire.CardTimeKind
	}{
		{"needs you", wire.EpicListRow{Section: wire.SectionNeedsYou, WaitingSince: at}, cardTimeSources{}, wire.CardTimeWaiting},
		{"running", wire.EpicListRow{Section: wire.SectionImplementing, State: wire.EpicListRunning}, cardTimeSources{startedAt: at}, wire.CardTimeElapsed},
		{"ready to queue", wire.EpicListRow{Section: wire.SectionReadyToQueue, CreatedAt: at}, cardTimeSources{}, wire.CardTimeCreated},
		{"ready to design", wire.EpicListRow{Section: wire.SectionReadyToDesign, CreatedAt: at}, cardTimeSources{}, wire.CardTimeCreated},
		{"finished", wire.EpicListRow{Section: wire.SectionFinished, CompletedAt: at}, cardTimeSources{}, wire.CardTimeFinished},
		{"no start", wire.EpicListRow{Section: wire.SectionNeedsYou}, cardTimeSources{}, ""},
		{"no section", wire.EpicListRow{CreatedAt: at}, cardTimeSources{}, ""},
	} {
		kind, start := cardTimeFor(tc.row, tc.src)
		if kind != tc.kind || (kind != "" && !start.Equal(at)) {
			t.Errorf("%s: time = %q %v, want %q %v", tc.name, kind, start, tc.kind, at)
		}
	}
}
