package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/ralphloop/ticketops"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// fakeChatTransport records every text it's asked to send, in memory, so
// chatEventSink's membership and pause-cardinality behavior can be tested
// without a real HTTP round trip. sendSyncFunc, if set, overrides the
// default always-succeeds-immediately behavior — used by the shutdown-redesign
// tests to simulate a hung/failing send that respects ctx cancellation, the
// same way a real HTTP transport would.
type fakeChatTransport struct {
	mu              sync.Mutex
	sent            []string
	sendSyncFunc    func(ctx context.Context, text chatmarkup.Text) (sendResult, error)
	timeoutOverride time.Duration
}

func (f *fakeChatTransport) name() string { return "fake" }
func (f *fakeChatTransport) timeout() time.Duration {
	if f.timeoutOverride != 0 {
		return f.timeoutOverride
	}
	return time.Second
}

func (f *fakeChatTransport) sendSync(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
	if f.sendSyncFunc != nil {
		return f.sendSyncFunc(ctx, text)
	}
	f.recordSent(text.String())
	return sendResult{StatusCode: 200}, nil
}

func (f *fakeChatTransport) recordSent(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
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
func newFakeChatSink(t *testing.T, inner runlog.EventSink) (*chatEventSink, *fakeChatTransport) {
	t.Helper()
	transport := &fakeChatTransport{}
	sink := newChatEventSink(inner, slackStyle, transport, "", "epic")
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	return sink, transport
}

// sendTestMessage enqueues a fixed text through send, so batching, gate,
// requeue and close tests don't depend on any run-loop event's wording.
func sendTestMessage(sink *chatEventSink, text, kind string) {
	sink.sendMessage(chatMessage{subject: text, kind: kind}, epicSource(sink.epicName), "")
}

// sendTestTicketMessage is sendTestMessage filed under a ticket source, the
// shape the per-source mute gate keys on.
func sendTestTicketMessage(sink *chatEventSink, text, kind, identifier string) {
	sink.sendMessage(chatMessage{subject: text, kind: kind}, ResolveTicketPath(sink.scratchDir, sink.epicName, identifier), identifier)
}

// TestChatEventSink_Close_CancelsInFlightSendPromptlyInsteadOfWaitingOutBackoff
// covers the ticket's first required seam: an in-flight sendRaw stuck
// sleeping in sendWithRetry's backoff when Close is called must be cancelled
// promptly, not waited out.
func TestChatEventSink_Close_CancelsInFlightSendPromptlyInsteadOfWaitingOutBackoff(t *testing.T) {
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	transport.timeoutOverride = 5 * time.Second
	transport.sendSyncFunc = func(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
		<-ctx.Done()
		return sendResult{}, ctx.Err()
	}

	sendTestMessage(sink, "done", notifyKindEpicComplete)
	sink.flush()

	start := time.Now()
	sink.Close()
	elapsed := time.Since(start)

	if elapsed >= notificationRetryBackoff {
		t.Fatalf("Close took %v, want well under notificationRetryBackoff (%v) — in-flight send was waited out, not cancelled", elapsed, notificationRetryBackoff)
	}
}

// TestChatEventSink_CloseFlush_ConsolidatedSend_RetriesOnceLikeNormalPath
// covers the ticket's third required seam: closeFlush's consolidated send
// must retry like the normal async path, not make a single attempt.
func TestChatEventSink_CloseFlush_ConsolidatedSend_RetriesOnceLikeNormalPath(t *testing.T) {
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	var attempts atomic.Int32
	transport.sendSyncFunc = func(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
		if attempts.Add(1) == 1 {
			return sendResult{}, errors.New("first attempt fails")
		}
		transport.recordSent(text.String())
		return sendResult{StatusCode: 200}, nil
	}

	sendTestMessage(sink, "done", notifyKindEpicComplete)
	sink.Close()

	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want exactly 2 (retry-once)", got)
	}
	if got := transport.snapshot(); len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 message", got)
	}
}

// TestChatEventSink_Close_WaitsForInProgressFlush_BeforeCancellingInFlight
// covers the ticket's second required seam: a flush that has emptied the
// queue but not yet reached sendRaw (so inFlight hasn't been incremented
// yet) when Close is called must not lose its message. This uses
// flushMidpointHook to pause a real ticker-driven flush at exactly that
// point; it would fail against a naive stopFlushLoop that only closes
// flushDone without waiting for the tick's flush() to actually return.
func TestChatEventSink_Close_WaitsForInProgressFlush_BeforeCancellingInFlight(t *testing.T) {
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

	hookEntered := make(chan struct{})
	release := make(chan struct{})
	sink.flushMidpointHook = func() {
		close(hookEntered)
		<-release
	}

	sink.startFlushLoop(5 * time.Millisecond)
	sendTestMessage(sink, "done", notifyKindEpicComplete)

	<-hookEntered

	closeDone := make(chan struct{})
	go func() {
		sink.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		t.Fatalf("Close returned before the in-progress flush was released")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-closeDone

	if got := transport.snapshot(); len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 message, in-progress flush was lost", got)
	}
}

// TestChatEventSink_Close_TotalShutdownDeadline_ReturnsWithinBound covers
// the ticket's fourth required seam: Close must return within
// shutdownTotalBudget even when sends would otherwise hang indefinitely.
// Not parallel: it mutates the package-level shutdownTotalBudget var.
func TestChatEventSink_Close_TotalShutdownDeadline_ReturnsWithinBound(t *testing.T) {
	original := shutdownTotalBudget
	shutdownTotalBudget = 200 * time.Millisecond
	defer func() { shutdownTotalBudget = original }()

	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	transport.timeoutOverride = time.Minute
	transport.sendSyncFunc = func(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
		<-ctx.Done()
		return sendResult{}, ctx.Err()
	}

	sendTestMessage(sink, "done", notifyKindEpicComplete)
	sink.flush()

	start := time.Now()
	sink.Close()
	elapsed := time.Since(start)

	if elapsed > shutdownTotalBudget+500*time.Millisecond {
		t.Fatalf("Close took %v, want within shutdownTotalBudget (%v) plus slack", elapsed, shutdownTotalBudget)
	}
}

func TestChatEventSink_NonMembers_SendNoMessage(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

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
		Type:   schema.TypeImplement,
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
	sink := newChatEventSink(&fakes.RecordingSink{}, slackStyle, transport, scratchDir, epicName)
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	return sink, transport
}

// TestChatEventSink_RepeatedEvent_TripsPerSourceMuteAndPausesTicket covers
// two of ticket 04's test seams at once: the end-to-end trip (5 identical
// events within 60s mutes the source and sends exactly one "muting this"
// notice) and the loop-registry pauseTicket wiring (the trip resolves the
// ticket's real path and reaches markRepairWithReason with a
// storm-mute reason).
func TestChatEventSink_RepeatedEvent_TripsPerSourceMuteAndPausesTicket(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicName := "epic"
	ticketPath := writeChatGateTicket(t, scratchDir, epicName, "04")
	sink, transport := newGateChatSink(t, scratchDir, epicName)

	for range 5 {
		sendTestTicketMessage(sink, "paused", notifyKindTicketNeedsHuman, "04")
	}
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want 1 batched message (4 deduped normal sends plus 1 muting notice)", got)
	}
	normalText := slackStyle.chatStyle.Escape("paused").String()
	muted := mutedMessage(ticketPath, "04")
	if got, want := muted.Headline(), "🔇 Muted epic/04 Ticket"; got != want {
		t.Errorf("muted headline = %q, want %q", got, want)
	}
	if want := filepath.Base(scratchDir); muted.project != want {
		t.Errorf("muted project = %q, want %q", muted.project, want)
	}
	wantMuted := muted.Text(slackStyle.chatStyle).String()
	want := "🔇 2 updates: Muted\n\n---\n\n" + fmt.Sprintf("%s ×4", normalText) + "\n\n---\n\n" + wantMuted
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}

	ticket, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	fakes.AssertPaused(t, ticket, schema.PauseClassRepair, "")
	if t.Failed() {
		t.FailNow()
	}
	if len(ticket.Mutes) != 1 || ticket.Mutes[0].EventType != notifyKindTicketNeedsHuman {
		t.Fatalf("ticket.Mutes = %+v, want one %s mute", ticket.Mutes, notifyKindTicketNeedsHuman)
	}

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("read ticket file: %v", err)
	}
	if !strings.Contains(string(raw), "storm mute") {
		t.Errorf("ticket body = %q, want it to name the storm-mute reason", raw)
	}

	// Seam B: the quarantine writes exactly one pause event.
	evs, _, err := runlog.ReadEvents(scratchDir, epicName)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var pauses []runlog.Event
	for _, ev := range evs {
		if ev.Type == string(events.Paused) {
			pauses = append(pauses, ev)
		}
	}
	if len(pauses) != 1 || pauses[0].Kind != string(events.IterationError) || pauses[0].Address != fixture.CanonicalAddr(scratchDir, epicName, "04") {
		t.Errorf("pause events = %+v, want one of kind iteration-error for 04", pauses)
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
		sendTestTicketMessage(sink, "paused", notifyKindTicketNeedsHuman, "07")
	}
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message deduped ×3 (no trip below threshold)", got)
	}
	want := fmt.Sprintf("%s ×3", slackStyle.chatStyle.Escape("paused").String())
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// TestChatEventSink_MultipleDistinctEvents_FlushSendsOneSeparatorJoinedMessage
// pins ticket 06's batch-shape seam: several distinct queued messages render
// as one send, joined by a separator line.
func TestChatEventSink_MultipleDistinctEvents_FlushSendsOneSeparatorJoinedMessage(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

	sendTestMessage(sink, "started", notifyKindEpicComplete)
	sendTestMessage(sink, "done", notifyKindEpicComplete)
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message", got)
	}
	want := "🔔 2 updates: \n\n---\n\n" + slackStyle.chatStyle.Escape("started").String() + "\n\n---\n\n" + slackStyle.chatStyle.Escape("done").String()
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// flushMessages queues msgs on a fresh sink, flushes once and returns what
// went out plus the transport.
func flushMessages(t *testing.T, msgs ...chatMessage) []string {
	t.Helper()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	for i, m := range msgs {
		sink.sendMessage(m, epicSource(sink.epicName), fmt.Sprint(i))
	}
	sink.flush()
	return waitForSentCount(transport, 1)
}

func repairMsg(subject string) chatMessage {
	return chatMessage{icon: "🛑", status: "Repair", subject: subject, kind: notifyKindTicketNeedsHuman, urgency: urgencyRepair}
}

func answerMsg(subject string) chatMessage {
	return chatMessage{icon: "🙋", status: "Answer", subject: subject, kind: notifyKindTicketNeedsHuman, urgency: urgencyAnswer}
}

func mergedMsg(subject string) chatMessage {
	return chatMessage{icon: "🎉", status: "Merged", subject: subject, kind: notifyKindEpicComplete, urgency: urgencyResult}
}

func TestChatEventSink_BatchSummary_IconIsMostUrgentWordsInUrgencyOrderWithCounts(t *testing.T) {
	t.Parallel()
	got := flushMessages(t, mergedMsg("m"), repairMsg("r1"), answerMsg("a"), repairMsg("r2"))
	if len(got) != 1 {
		t.Fatalf("sent = %v, want one send", got)
	}
	first, _, _ := strings.Cut(got[0], "\n")
	if want := "🛑 4 updates: Repair ×2, Answer, Merged"; first != want {
		t.Errorf("summary = %q, want %q", first, want)
	}
	order := []string{"🛑 Repair r1", "🛑 Repair r2", "🙋 Answer a", "🎉 Merged m"}
	last := -1
	for _, h := range order {
		i := strings.Index(got[0], h)
		if i <= last {
			t.Errorf("%q out of urgency order in:\n%s", h, got[0])
		}
		last = i
	}
}

func TestChatEventSink_BatchSummary_DedupedBurstCountsOnceAndNoticeAddsWord(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	for range 3 {
		sink.enqueueMessage(repairMsg("same"))
	}
	sink.enqueueMessage(serverNoticeMessage(ServerNotice{Emoji: "🚀", Title: "server started"}))
	sink.flush()
	got := waitForSentCount(transport, 1)
	if first, _, _ := strings.Cut(got[0], "\n"); first != "🛑 2 updates: Repair, Notice" {
		t.Errorf("summary = %q", first)
	}
	if !strings.Contains(got[0], "🛑 Repair same ×3") {
		t.Errorf("burst headline lost its count:\n%s", got[0])
	}
}

func TestChatEventSink_BatchSummary_CutsAfterThreeWords(t *testing.T) {
	t.Parallel()
	conflict := chatMessage{icon: "🔀", status: "Conflict", subject: "c", urgency: urgencyConflict}
	got := flushMessages(t, repairMsg("r"), answerMsg("a"), conflict, mergedMsg("m"))
	if first, _, _ := strings.Cut(got[0], "\n"); first != "🛑 4 updates: Repair, Answer, Conflict, …" {
		t.Errorf("summary = %q", first)
	}
}

func TestChatEventSink_SingleMessageSendsNoSummary(t *testing.T) {
	t.Parallel()
	got := flushMessages(t, repairMsg("only"))
	if want := repairMsg("only").Text(slackStyle.chatStyle).String(); got[0] != want {
		t.Errorf("sent = %q, want %q", got[0], want)
	}
}

func TestChatEventSink_BatchSummary_IsValidMarkdownV2(t *testing.T) {
	t.Parallel()
	items := []batchedMessage{
		{msg: new(repairMsg("a"))},
		{msg: new(mergedMsg("b"))},
	}
	text := renderBatch(telegramStyle, items).String()
	if first, _, _ := strings.Cut(text, "\n"); first != `🛑 2 updates: Repair, Merged` {
		t.Errorf("summary = %q", first)
	}
	if !strings.Contains(text, `\-\-\-`) {
		t.Errorf("separator not escaped: %q", text)
	}
}

func TestUrgencyRankingByKind(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		msg  chatMessage
		want urgency
	}{
		{"work-holding notice", serverNoticeMessage(ServerNotice{Title: "x", HoldsWork: true}), urgencyRepair},
		{"fault pause", ticketPauseMessage("p", "e", "", "01", ticketops.PauseStatus("spinning"), "d"), urgencyRepair},
		{"answer pause", ticketPauseMessage("p", "e", "", "01", ticketops.PauseStatus("zero-commit"), "d"), urgencyAnswer},
		{"escalated", escalatedMessage("p", "e", "", "01", ""), urgencyEscalated},
		{"conflict", conflictStartedMessage("p", "e", "", "01"), urgencyConflict},
		{"result", resultMessage("p", "e", "", "01", "r"), urgencyResult},
		{"epic merged", epicOutcomeMessage("p", "e", schedule.EpicCounts{}, 0, 0, true), urgencyResult},
		{"other notice", serverNoticeMessage(ServerNotice{Title: "x"}), urgencyInformation},
	} {
		if c.msg.urgency != c.want {
			t.Errorf("%s: urgency = %d, want %d", c.name, c.msg.urgency, c.want)
		}
	}
	if !(urgencyRepair > urgencyAnswer && urgencyAnswer > urgencyEscalated && urgencyEscalated > urgencyConflict &&
		urgencyConflict > urgencyResult && urgencyResult > urgencyInformation) {
		t.Error("urgency classes are not ordered repair, answer, escalated, conflict, result, information")
	}
	if mutedMessage("s/p/e/issues/01.md", "01").urgency != urgencyInformation || testMessage().urgency != urgencyInformation {
		t.Error("mute and test messages are information")
	}
}

// TestChatEventSink_IdenticalChatMessages_CountLandsOnHeadlineNotProjectLine
// pins that a deduped message shows ×N at the end of its Headline and keeps a
// bare project line, while a distinct message stays separate.
func TestChatEventSink_IdenticalChatMessages_CountLandsOnHeadlineNotProjectLine(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

	msg := chatMessage{icon: "🔀", status: "Conflict", subject: "epic/01", lines: []string{"detail"}, project: "proj", kind: notifyKindConflictStarted}
	other := msg
	other.subject = "epic/02"
	sink.sendMessage(msg, epicSource("epic"), "")
	sink.sendMessage(msg, epicSource("epic"), "")
	sink.sendMessage(other, epicSource("epic"), "")
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message", got)
	}
	repeated := msg
	repeated.repeat = 2
	body := repeated.Text(slackStyle.chatStyle).String() + "\n\n---\n\n" + other.Text(slackStyle.chatStyle).String()
	want := "🔀 2 updates: Conflict ×2\n\n---\n\n" + body
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
	if !strings.Contains(got[0], "epic/01 ×2\n") || !strings.HasSuffix(strings.SplitN(body, "\n\n---", 2)[0], "proj") {
		t.Errorf("count not on Headline with bare project line: %q", got[0])
	}
}

// TestChatEventSink_IdenticalMessages_FlushCollapsesToSingleLineWithCount
// pins ticket 06's dedup seam directly against enqueue/renderBatch, apart
// from the gate-storm scenario the RepeatedEvent test also covers.
func TestChatEventSink_IdenticalMessages_FlushCollapsesToSingleLineWithCount(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

	sendTestMessage(sink, "started", notifyKindEpicComplete)
	sendTestMessage(sink, "started", notifyKindEpicComplete)
	sendTestMessage(sink, "started", notifyKindEpicComplete)
	sink.flush()

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly 1 batched message", got)
	}
	want := fmt.Sprintf("%s ×3", slackStyle.chatStyle.Escape("started").String())
	if got[0] != want {
		t.Errorf("sent[0] = %q, want %q", got[0], want)
	}
}

// TestChatEventSink_EmptyQueue_FlushSendsNothing pins ticket 06's empty-tick
// seam: a flush with nothing queued sends nothing.
func TestChatEventSink_EmptyQueue_FlushSendsNothing(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

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
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

	sendTestMessage(sink, "started", notifyKindEpicComplete)
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
	scratchDir := fixture.EpicScratchDir(t, "epic")
	epicName := "epic"
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	sink.scratchDir = scratchDir
	sink.epicName = epicName

	// Enqueue while the transport is still unmuted — a gate call against an
	// already-muted transport would suppress at gate time (see send) and
	// never reach the queue, which is a different code path than the one
	// this test targets: a close-time flush finding the transport muted
	// after the item was already queued.
	sendTestMessage(sink, "started", notifyKindEpicComplete)

	if err := updateNotificationStateAt(sink.gateStatePath, func(state *NotificationState) {
		state.Transports["fake"] = TransportState{Muted: true, Reason: "auto-trip"}
	}); err != nil {
		t.Fatalf("updateNotificationStateAt: %v", err)
	}

	sink.Close()

	if got := transport.snapshot(); len(got) != 0 {
		t.Errorf("sent = %v, want none — transport is globally muted", got)
	}

	evs, ok, err := runlog.ReadEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var suppressed []runlog.Event
	for _, ev := range evs {
		if ev.Type == string(events.NotificationSuppressed) {
			suppressed = append(suppressed, ev)
		}
	}
	if len(suppressed) != 1 {
		t.Fatalf("suppressed run-log lines = %v, want exactly 1", suppressed)
	}
	if !strings.Contains(suppressed[0].Reason, notifyKindEpicComplete) {
		t.Errorf("suppressed reason = %q, want it to name %q", suppressed[0].Reason, notifyKindEpicComplete)
	}
}

// TestChatEventSink_BurstOfEventsCoalescedIntoOneFlush_CountsOneSendAgainstBudget
// pins ticket 12's gate-at-flush-time seam: a burst of events well over
// globalThreshold, all enqueued before a single flush, counts as exactly one
// send-series entry (not one per event) and does not trip the global
// breaker — only sustained flush volume can.
func TestChatEventSink_BurstOfEventsCoalescedIntoOneFlush_CountsOneSendAgainstBudget(t *testing.T) {
	t.Parallel()
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})

	for i := range globalThreshold + 5 {
		sendTestMessage(sink, fmt.Sprintf("started-%d", i), notifyKindEpicComplete)
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

// TestChatEventSink_Flush_FailedSend_RequeuesItems covers ticket 03b's first
// seam: a flush whose send ultimately fails (after sendNotificationSync's own
// retry) must put its items back on the queue, marked requeued, rather than
// lose them.
func TestChatEventSink_Flush_FailedSend_RequeuesItems(t *testing.T) {
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	transport.sendSyncFunc = func(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
		return sendResult{}, errors.New("boom")
	}

	sendTestMessage(sink, "done", notifyKindEpicComplete)
	sink.flush()
	sink.inFlight.Wait()

	sink.mu.Lock()
	queue := append([]batchedMessage(nil), sink.queue...)
	sink.mu.Unlock()
	if len(queue) != 1 {
		t.Fatalf("queue after failed flush = %v, want exactly 1 requeued item", queue)
	}
	if !queue[0].requeued {
		t.Errorf("requeued item not marked requeued")
	}
}

// TestChatEventSink_Requeue_MergesWithNewlyQueuedDuplicate covers ticket
// 03b's merge seam: an identical text that arrived (was enqueued fresh)
// while the failed batch was being requeued must end up as one merged entry
// with a combined count, not two separate ×N groups.
func TestChatEventSink_Requeue_MergesWithNewlyQueuedDuplicate(t *testing.T) {
	sink, _ := newFakeChatSink(t, &fakes.RecordingSink{})
	textA := slackStyle.chatStyle.Escape("a")
	textB := slackStyle.chatStyle.Escape("b")
	sink.queue = []batchedMessage{{text: textB, kind: "k", count: 1}}

	sink.requeue([]batchedMessage{
		{text: textA, kind: "k", count: 2},
		{text: textB, kind: "k", count: 1},
	})

	if len(sink.queue) != 2 {
		t.Fatalf("queue = %v, want 2 entries (a prepended, b merged)", sink.queue)
	}
	if sink.queue[0].text != textA || !sink.queue[0].requeued || sink.queue[0].count != 2 {
		t.Errorf("queue[0] = %+v, want prepended requeued textA with count 2", sink.queue[0])
	}
	if sink.queue[1].text != textB || sink.queue[1].requeued || sink.queue[1].count != 2 {
		t.Errorf("queue[1] = %+v, want merged textB with count 2, not marked requeued", sink.queue[1])
	}
}

// TestChatEventSink_Requeue_CapsSize covers ticket 03b's cap seam: repeated
// failures past maxRequeueSize leave the queue bounded, not growing
// unboundedly.
func TestChatEventSink_Requeue_CapsSize(t *testing.T) {
	sink, _ := newFakeChatSink(t, &fakes.RecordingSink{})

	items := make([]batchedMessage, maxRequeueSize+10)
	for i := range items {
		items[i] = batchedMessage{text: slackStyle.chatStyle.Escape(fmt.Sprintf("msg-%d", i)), kind: "k", count: 1}
	}
	sink.requeue(items)

	if len(sink.queue) != maxRequeueSize {
		t.Fatalf("queue len = %d, want capped at maxRequeueSize (%d)", len(sink.queue), maxRequeueSize)
	}
}

// TestChatEventSink_RequeueRetryCycle_DoesNotDoubleChargeGateBudget covers
// ticket 03b's gate-budget seam: a requeue-and-retry cycle must not record a
// second recordSend=true gate call for the same logical batch, or a
// sustained outage could trip the global-mute breaker purely from retrying.
func TestChatEventSink_RequeueRetryCycle_DoesNotDoubleChargeGateBudget(t *testing.T) {
	sink, transport := newFakeChatSink(t, &fakes.RecordingSink{})
	transport.sendSyncFunc = func(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
		return sendResult{}, errors.New("boom")
	}

	sendTestMessage(sink, "done", notifyKindEpicComplete)
	sink.flush()
	sink.inFlight.Wait()

	sink.flush() // retry cycle: empties the requeued (all-requeued) batch
	sink.inFlight.Wait()

	state, err := loadNotificationStateAt(sink.gateStatePath)
	if err != nil {
		t.Fatalf("loadNotificationStateAt: %v", err)
	}
	if got := len(state.Transports["fake"].Sends); got != 1 {
		t.Errorf("gate Sends = %d, want exactly 1 (retry cycle must not re-charge)", got)
	}
}

// TestChatEventSink_Requeue_SkippedAfterClose_LogsSuppression covers ticket
// 03b's closed-flag guard seam: a requeue attempted after Close has started
// shutting the sink down is skipped and logged via a suppression-style
// run-log line instead of silently dropped into a queue nobody will ever
// flush again.
func TestChatEventSink_Requeue_SkippedAfterClose_LogsSuppression(t *testing.T) {
	scratchDir := fixture.EpicScratchDir(t, "epic")
	epicName := "epic"
	sink, _ := newFakeChatSink(t, &fakes.RecordingSink{})
	sink.scratchDir = scratchDir
	sink.epicName = epicName

	sink.mu.Lock()
	sink.closed = true
	sink.mu.Unlock()

	sink.requeue([]batchedMessage{{text: slackStyle.chatStyle.Escape("a"), kind: notifyKindEpicComplete, count: 1}})

	sink.mu.Lock()
	queueLen := len(sink.queue)
	sink.mu.Unlock()
	if queueLen != 0 {
		t.Errorf("queue after closed requeue = %d entries, want 0 (should be skipped, not queued)", queueLen)
	}

	evs, ok, err := runlog.ReadEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var suppressed []runlog.Event
	for _, ev := range evs {
		if ev.Type == string(events.NotificationSuppressed) {
			suppressed = append(suppressed, ev)
		}
	}
	if len(suppressed) != 1 {
		t.Fatalf("suppressed run-log lines = %v, want exactly 1", suppressed)
	}
	if !strings.Contains(suppressed[0].Reason, notifyKindEpicComplete) {
		t.Errorf("suppressed reason = %q, want it to name %q", suppressed[0].Reason, notifyKindEpicComplete)
	}
}
