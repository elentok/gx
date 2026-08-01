package ralphloop

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

var _ EventSink = (*ChannelEventSink)(nil)

func TestChannelEventSink_ForwardsCallsAsLiveEvents(t *testing.T) {
	s := NewChannelEventSink()

	s.TicketClaimed(tickets.Ticket{Identifier: "04a", Number: 4})
	s.IterationStarted("04a", "iter-04a")
	s.IterationPaused("iter-04a", PauseNeedsAttention, "Codex is waiting for operator intervention")
	s.IterationResumed("iter-04a", PauseNeedsAttention)
	s.IterationFinished(tickets.Ticket{Identifier: "04a"}, "my-epic")

	want := []struct {
		kind       LiveEventKind
		identifier string
		label      string
		reason     string
		pauseKind  PauseKind
	}{
		{kind: LiveEventTicketClaimed, identifier: "04a"},
		{kind: LiveEventIterationStarted, identifier: "04a", label: "iter-04a"},
		{kind: LiveEventIterationPaused, label: "iter-04a", pauseKind: PauseNeedsAttention, reason: "Codex is waiting for operator intervention"},
		{kind: LiveEventIterationResumed, label: "iter-04a", pauseKind: PauseNeedsAttention},
		{kind: LiveEventIterationFinished, identifier: "04a"},
	}

	for i, w := range want {
		select {
		case ev := <-s.Events():
			if ev.Kind != w.kind || ev.Identifier != w.identifier || ev.Label != w.label ||
				ev.Reason != w.reason || ev.PauseKind != w.pauseKind {
				t.Errorf("event %d = %+v, want %+v", i, ev, w)
			}
		default:
			t.Fatalf("event %d: channel empty, want %+v", i, w)
		}
	}
}
