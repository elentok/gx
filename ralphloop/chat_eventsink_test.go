package ralphloop

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// fakeChatTransport records every text it's asked to send, in memory, so
// chatEventSink's membership and park-cardinality behavior can be tested
// without a real HTTP round trip.
type fakeChatTransport struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeChatTransport) name() string           { return "fake" }
func (f *fakeChatTransport) timeout() time.Duration { return time.Second }

func (f *fakeChatTransport) sendSync(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	return nil
}

func (f *fakeChatTransport) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.sent))
	copy(out, f.sent)
	return out
}

func waitForSentCount(f *fakeChatTransport, want int) []string {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := f.snapshot(); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return f.snapshot()
}

func newFakeChatSink(inner EventSink) (*chatEventSink, *fakeChatTransport) {
	transport := &fakeChatTransport{}
	sink := newChatEventSink(inner, slackStyle, transport, "", "epic")
	return sink, transport
}

func TestChatEventSink_ChatMembers_EachSendExactlyOneMessage(t *testing.T) {
	ticket := tickets.Ticket{Identifier: "04", Title: "Some ticket"}
	stats := IterationStats{ElapsedSeconds: 10, PeakContextTokens: 100, Completed: 1, Total: 2}

	cases := []struct {
		name string
		fire func(sink *chatEventSink)
	}{
		{"EpicStarted", func(s *chatEventSink) { s.EpicStarted("epic", 0, 3) }},
		{"IterationStarted", func(s *chatEventSink) { s.IterationStarted(ticket, "iter-04", "/repo", "sess-1") }},
		{"IterationPaused", func(s *chatEventSink) { s.IterationPaused("04", "iter-04", PauseRateLimit, "rate limited") }},
		{"IterationResumed", func(s *chatEventSink) { s.IterationResumed("04", "iter-04", PauseRateLimit) }},
		{"IterationFinished", func(s *chatEventSink) { s.IterationFinished(ticket, "epic", stats) }},
		{"TicketNeedsHuman", func(s *chatEventSink) { s.TicketNeedsHuman("04", "epic", "needs-answer", "no commits landed") }},
		{"EpicParked", func(s *chatEventSink) { s.EpicParked("epic", []StalledTicket{{Identifier: "04"}}) }},
		{"EpicComplete", func(s *chatEventSink) { s.EpicComplete("epic", 5, 300) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sink, transport := newFakeChatSink(&recordingSink{})
			c.fire(sink)
			if got := waitForSentCount(transport, 1); len(got) != 1 {
				t.Fatalf("sent = %v, want exactly 1 message", got)
			}
		})
	}
}

func TestChatEventSink_NonMembers_SendNoMessage(t *testing.T) {
	sink, transport := newFakeChatSink(&recordingSink{})

	sink.TicketReverted("01")
	sink.TicketReattached("01", "iter-01", "/repo", "sess-1")
	sink.TicketClaimed(tickets.Ticket{Identifier: "01"})
	sink.TranscriptLine("iter-01", "some line")
	sink.ContextOccupancy("01", 100)
	sink.CherryPickStarted("01")
	sink.ConflictResolutionStarted("01")
	sink.SmartZoneCompactStarted("01")
	sink.SmartZoneFinishingUp("01")
	sink.SmartZoneRecovered("01")
	sink.TicketCleanupFinished("01")
	sink.TicketRecovering("01")
	sink.TicketRecovered("01", "epic", "branch", "sha")
	sink.TicketUnrecoverable("01", "epic")

	// Give any (unwanted) async send a moment to have fired before asserting none did.
	time.Sleep(50 * time.Millisecond)
	if got := transport.snapshot(); len(got) != 0 {
		t.Errorf("sent = %v, want none", got)
	}
}

func TestChatEventSink_EveryEvent_ForwardsToInner(t *testing.T) {
	inner := &recordingSink{}
	sink, _ := newFakeChatSink(inner)

	sink.EpicStarted("epic", 0, 3)
	sink.IterationStarted(tickets.Ticket{Identifier: "01"}, "iter-01", "/repo", "sess-1")
	sink.IterationPaused("01", "iter-01", PauseRateLimit, "rate limited")
	sink.IterationResumed("01", "iter-01", PauseRateLimit)
	sink.IterationFinished(tickets.Ticket{Identifier: "01"}, "epic", IterationStats{})
	sink.TicketNeedsHuman("01", "epic", "needs-answer", "no commits landed")
	sink.EpicParked("epic", []StalledTicket{{Identifier: "01"}})
	sink.EpicComplete("epic", 1, 0)

	want := []string{
		"EpicStarted", "IterationStarted", "IterationPaused", "IterationResumed",
		"IterationFinished", "TicketNeedsHuman", "EpicParked", "EpicComplete",
	}
	if got := inner.snapshot(); len(got) != len(want) {
		t.Fatalf("inner events = %v, want %v", got, want)
	} else {
		for i, w := range want {
			if got[i] != w {
				t.Errorf("inner events[%d] = %q, want %q", i, got[i], w)
			}
		}
	}
}

// TestChatEventSink_Park_ProducesExactlyOneChatMessage pins the park
// cardinality rule: a park fires both IterationPaused(needs-repair) and
// TicketNeedsHuman on the underlying ticket, but only TicketNeedsHuman may
// reach chat — otherwise a single park would read as two messages.
func TestChatEventSink_Park_ProducesExactlyOneChatMessage(t *testing.T) {
	sink, transport := newFakeChatSink(&recordingSink{})

	sink.IterationPaused("04", "iter-04", PauseNeedsRepair, "agent blocked on permission prompt")
	sink.TicketNeedsHuman("04", "epic", "needs-repair", "agent blocked on permission prompt")

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 message for the park", got)
	}
	want := slackStyle.ticketNeedsHumanText("04", "epic", "needs-repair", "agent blocked on permission prompt", EpicCounts{})
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

func TestChatEventSink_IterationPausedResumed_NeedsRepairKindNeverReachesChat(t *testing.T) {
	sink, transport := newFakeChatSink(&recordingSink{})

	sink.IterationPaused("04", "iter-04", PauseNeedsRepair, "blocked")
	sink.IterationResumed("04", "iter-04", PauseNeedsRepair)

	time.Sleep(50 * time.Millisecond)
	if got := transport.snapshot(); len(got) != 0 {
		t.Errorf("sent = %v, want none for a needs-repair pause/resume", got)
	}
}

func TestChatEventSink_IterationPausedResumed_RateLimitKindReachesChat(t *testing.T) {
	sink, transport := newFakeChatSink(&recordingSink{})

	sink.IterationPaused("04", "iter-04", PauseRateLimit, "rate limited")
	sink.IterationResumed("04", "iter-04", PauseRateLimit)

	if got := waitForSentCount(transport, 2); len(got) != 2 {
		t.Fatalf("sent = %v, want exactly 2 messages", got)
	}
}
