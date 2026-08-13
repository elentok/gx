package ralphloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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

// newFakeChatSink builds a chatEventSink whose gate reads/writes a
// per-test temp state file (see chatEventSink.gateStatePath) instead of the
// real ~/.config/gx/notifications-state.json, so exercising send from a
// test never touches real user state.
func newFakeChatSink(t *testing.T, inner EventSink) (*chatEventSink, *fakeChatTransport) {
	t.Helper()
	transport := &fakeChatTransport{}
	sink := newChatEventSink(inner, slackStyle, transport, "", "epic")
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	return sink, transport
}

func TestChatEventSink_ChatMembers_EachSendExactlyOneMessage(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			sink, transport := newFakeChatSink(t, &recordingSink{})
			c.fire(sink)
			sink.flush()
			if got := waitForSentCount(transport, 1); len(got) != 1 {
				t.Fatalf("sent = %v, want exactly 1 message", got)
			}
		})
	}
}

func TestChatEventSink_NonMembers_SendNoMessage(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

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
	t.Parallel()
	inner := &recordingSink{}
	sink, _ := newFakeChatSink(t, inner)

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
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.IterationPaused("04", "iter-04", PauseNeedsRepair, "agent blocked on permission prompt")
	sink.TicketNeedsHuman("04", "epic", "needs-repair", "agent blocked on permission prompt")
	sink.flush()

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
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.IterationPaused("04", "iter-04", PauseNeedsRepair, "blocked")
	sink.IterationResumed("04", "iter-04", PauseNeedsRepair)

	time.Sleep(50 * time.Millisecond)
	if got := transport.snapshot(); len(got) != 0 {
		t.Errorf("sent = %v, want none for a needs-repair pause/resume", got)
	}
}

func TestChatEventSink_IterationPausedResumed_RateLimitKindReachesChat(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.IterationPaused("04", "iter-04", PauseRateLimit, "rate limited")
	sink.IterationResumed("04", "iter-04", PauseRateLimit)
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message joining both", got)
	}
	pausedText := slackStyle.iterationPausedText("iter-04", "rate limited", "epic", "04")
	resumedText := slackStyle.iterationResumedText("iter-04", "epic", "04")
	want := pausedText + "\n---\n" + resumedText
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// writeChatGateTicket writes a real ticket fixture at
// scratchDir/epicName/issues/<identifier>-ticket.md — the layout
// chatEventSink.resolveTicketPath expects — so the gate can parse and mute
// it for real instead of failing open on a missing file.
func writeChatGateTicket(t *testing.T, scratchDir, epicName, identifier string) string {
	t.Helper()
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues dir: %v", err)
	}
	marshaled, err := schema.MarshalTicket(schema.Ticket{
		ID:     schema.TicketID(identifier),
		Status: schema.StatusClaimed,
		Type:   schema.TypeTask,
	}, "body\n")
	if err != nil {
		t.Fatalf("MarshalTicket: %v", err)
	}
	path := filepath.Join(issuesDir, identifier+"-ticket.md")
	if err := os.WriteFile(path, marshaled, 0644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	return path
}

func newGateChatSink(t *testing.T, scratchDir, epicName string) (*chatEventSink, *fakeChatTransport) {
	t.Helper()
	transport := &fakeChatTransport{}
	sink := newChatEventSink(&recordingSink{}, slackStyle, transport, scratchDir, epicName)
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	return sink, transport
}

// TestChatEventSink_RepeatedEvent_TripsPerSourceMuteAndParksTicket covers
// two of ticket 04's test seams at once: the end-to-end trip (5 identical
// events within 60s mutes the source and sends exactly one "muting this"
// notice) and the loop-registry parkTicket wiring (the trip resolves the
// ticket's real path and reaches MarkNeedsRepairWithReason with a
// storm-mute reason).
func TestChatEventSink_RepeatedEvent_TripsPerSourceMuteAndParksTicket(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicName := "epic"
	ticketPath := writeChatGateTicket(t, scratchDir, epicName, "04")
	sink, transport := newGateChatSink(t, scratchDir, epicName)

	for range 5 {
		sink.IterationPaused("04", "iter-04", PauseRateLimit, "rate limited")
	}
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want 1 batched message (4 deduped normal sends plus 1 muting notice)", got)
	}
	normalText := slackStyle.iterationPausedText("iter-04", "rate limited", epicName, "04")
	wantMuted := slackStyle.mutedText(epicName, "04")
	want := fmt.Sprintf("%s ×4", normalText) + "\n---\n" + wantMuted
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}

	ticket, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	if ticket.Status != schema.StatusNeedsRepair {
		t.Fatalf("ticket.Status = %q, want %q", ticket.Status, schema.StatusNeedsRepair)
	}
	if len(ticket.Mutes) != 1 || ticket.Mutes[0].EventType != notifyKindIterationPaused {
		t.Fatalf("ticket.Mutes = %+v, want one %s mute", ticket.Mutes, notifyKindIterationPaused)
	}

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("read ticket file: %v", err)
	}
	if !strings.Contains(string(raw), "storm mute") {
		t.Errorf("ticket body = %q, want it to name the storm-mute reason", raw)
	}
}

// TestChatEventSink_NonTrippingSequence_SendsNormallyThroughGate is the
// third seam: a sequence under the per-source threshold passes through the
// gate and sends every message normally.
func TestChatEventSink_NonTrippingSequence_SendsNormallyThroughGate(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicName := "epic"
	writeChatGateTicket(t, scratchDir, epicName, "07")
	sink, transport := newGateChatSink(t, scratchDir, epicName)

	for range 3 {
		sink.IterationPaused("07", "iter-07", PauseRateLimit, "rate limited")
	}
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message deduped ×3 (no trip below threshold)", got)
	}
	want := fmt.Sprintf("%s ×3", slackStyle.iterationPausedText("iter-07", "rate limited", epicName, "07"))
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// TestChatEventSink_MultipleDistinctEvents_FlushSendsOneSeparatorJoinedMessage
// pins ticket 06's batch-shape seam: several distinct queued messages render
// as one send, joined by a separator line.
func TestChatEventSink_MultipleDistinctEvents_FlushSendsOneSeparatorJoinedMessage(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.EpicStarted("epic", 0, 3)
	sink.EpicComplete("epic", 1, 10)
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message", got)
	}
	want := slackStyle.epicStartedText("epic", EpicCounts{}) + "\n---\n" + slackStyle.epicCompleteText("epic", EpicCounts{}, 1, 10)
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// TestChatEventSink_IdenticalMessages_FlushCollapsesToSingleLineWithCount
// pins ticket 06's dedup seam directly against enqueue/renderBatch, apart
// from the gate-storm scenario the RepeatedEvent test also covers.
func TestChatEventSink_IdenticalMessages_FlushCollapsesToSingleLineWithCount(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.EpicStarted("epic", 0, 3)
	sink.EpicStarted("epic", 0, 3)
	sink.EpicStarted("epic", 0, 3)
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message", got)
	}
	want := fmt.Sprintf("%s ×3", slackStyle.epicStartedText("epic", EpicCounts{}))
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// TestChatEventSink_EmptyQueue_FlushSendsNothing pins ticket 06's empty-tick
// seam: a flush with nothing queued sends nothing.
func TestChatEventSink_EmptyQueue_FlushSendsNothing(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.flush()

	time.Sleep(50 * time.Millisecond)
	if got := transport.snapshot(); len(got) != 0 {
		t.Errorf("sent = %v, want none for an empty flush", got)
	}
}

// TestChatEventSink_Close_FlushesQueuedMessageSynchronously pins ticket 06's
// flush-on-close seam: Close sends the queue's one message before returning,
// with no need to poll like the async sendRaw path.
func TestChatEventSink_Close_FlushesQueuedMessageSynchronously(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	sink.EpicStarted("epic", 0, 3)
	sink.Close()

	got := transport.snapshot()
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 message from Close's synchronous flush", got)
	}
}

// TestChatEventSink_Close_GloballyMuted_SuppressesFlushAndLogsRunLogLine
// pins ticket 06's suppressed-close-time-flush seam: a close-time flush
// against an already globally-muted transport sends nothing, and appends
// exactly one notification-suppressed run-log line naming the queued kinds.
func TestChatEventSink_Close_GloballyMuted_SuppressesFlushAndLogsRunLogLine(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicName := "epic"
	sink, transport := newFakeChatSink(t, &recordingSink{})
	sink.scratchDir = scratchDir
	sink.epicName = epicName

	// Enqueue while the transport is still unmuted — a gate call against an
	// already-muted transport would suppress at gate time (see send) and
	// never reach the queue, which is a different code path than the one
	// this test targets: a close-time flush finding the transport muted
	// after the item was already queued.
	sink.EpicStarted("epic", 0, 3)

	if err := updateNotificationStateAt(sink.gateStatePath, func(state *NotificationState) {
		state.Transports["fake"] = TransportState{Muted: true, Reason: "auto-trip"}
	}); err != nil {
		t.Fatalf("updateNotificationStateAt: %v", err)
	}

	sink.Close()

	if got := transport.snapshot(); len(got) != 0 {
		t.Errorf("sent = %v, want none — transport is globally muted", got)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var suppressed []Event
	for _, ev := range events {
		if ev.Type == eventNotificationSuppressed {
			suppressed = append(suppressed, ev)
		}
	}
	if len(suppressed) != 1 {
		t.Fatalf("suppressed run-log lines = %v, want exactly 1", suppressed)
	}
	if !strings.Contains(suppressed[0].Reason, notifyKindEpicStarted) {
		t.Errorf("suppressed reason = %q, want it to name %q", suppressed[0].Reason, notifyKindEpicStarted)
	}
}

// TestChatEventSink_BurstOfEventsCoalescedIntoOneFlush_CountsOneSendAgainstBudget
// pins ticket 12's gate-at-flush-time seam: a burst of events well over
// globalThreshold, all enqueued before a single flush, counts as exactly one
// send-series entry (not one per event) and does not trip the global
// breaker — only sustained flush volume can.
func TestChatEventSink_BurstOfEventsCoalescedIntoOneFlush_CountsOneSendAgainstBudget(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &recordingSink{})

	for i := range globalThreshold + 5 {
		sink.EpicStarted(fmt.Sprintf("epic-%d", i), 0, 3)
	}
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message for one flush", got)
	}

	state, err := loadNotificationStateAt(sink.gateStatePath)
	if err != nil {
		t.Fatalf("loadNotificationStateAt: %v", err)
	}
	ts := state.Transports["fake"]
	if len(ts.Sends) != 1 {
		t.Fatalf("Sends = %v, want exactly 1 entry for one flush covering %d events", ts.Sends, globalThreshold+5)
	}
	if ts.Muted {
		t.Errorf("transport muted after a single flush, want the burst of events not to trip the global breaker")
	}
}
