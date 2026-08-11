package ralphloop

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

var _ EventSink = (*ChannelEventSink)(nil)

func TestChannelEventSink_ForwardsCallsAsLiveEvents(t *testing.T) {
	s := NewChannelEventSink()

	s.TicketClaimed(tickets.Ticket{Identifier: "04a", Number: 4})
	s.IterationStarted(tickets.Ticket{Identifier: "04a", Number: 4}, "iter-04a", "/repo/iter-04a", "sess-1")
	s.IterationPaused("04a", "iter-04a", PauseNeedsRepair, "Codex is waiting for operator intervention")
	s.IterationResumed("04a", "iter-04a", PauseNeedsRepair)
	s.IterationFinished(tickets.Ticket{Identifier: "04a"}, "my-epic", IterationStats{})
	s.TicketReattached("04a", "iter-04a", "/repo/iter-04a", "sess-1")
	s.ContextOccupancy("04a", 12345)

	want := []struct {
		kind       LiveEventKind
		identifier string
		label      string
		reason     string
		pauseKind  PauseKind
		cwd        string
		sessionID  string
		tokens     int
	}{
		{kind: LiveEventTicketClaimed, identifier: "04a"},
		{kind: LiveEventIterationStarted, identifier: "04a", label: "iter-04a", cwd: "/repo/iter-04a", sessionID: "sess-1"},
		{kind: LiveEventIterationPaused, identifier: "04a", label: "iter-04a", pauseKind: PauseNeedsRepair, reason: "Codex is waiting for operator intervention"},
		{kind: LiveEventIterationResumed, identifier: "04a", label: "iter-04a", pauseKind: PauseNeedsRepair},
		{kind: LiveEventIterationFinished, identifier: "04a"},
		{kind: LiveEventTicketReattached, identifier: "04a", label: "iter-04a", cwd: "/repo/iter-04a", sessionID: "sess-1"},
		{kind: LiveEventContextOccupancy, identifier: "04a", tokens: 12345},
	}

	for i, w := range want {
		select {
		case ev := <-s.Events():
			if ev.Kind != w.kind || ev.Identifier != w.identifier || ev.Label != w.label ||
				ev.Reason != w.reason || ev.PauseKind != w.pauseKind ||
				ev.Cwd != w.cwd || ev.SessionID != w.sessionID || ev.Tokens != w.tokens {
				t.Errorf("event %d = %+v, want %+v", i, ev, w)
			}
		default:
			t.Fatalf("event %d: channel empty, want %+v", i, w)
		}
	}
}
