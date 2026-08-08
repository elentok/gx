package ralphloop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// telegramRequest captures one decoded call to the fake Telegram API's
// sendMessage endpoint.
type telegramRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// fakeTelegramServer starts an httptest.Server standing in for the Telegram
// Bot API and returns it alongside a thread-safe accessor for the requests
// it received, since sends happen from their own goroutine.
func fakeTelegramServer(t *testing.T, statusCode int) (*httptest.Server, func() []telegramRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []telegramRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req telegramRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(server.Close)

	return server, func() []telegramRequest {
		mu.Lock()
		defer mu.Unlock()
		out := make([]telegramRequest, len(requests))
		copy(out, requests)
		return out
	}
}

// waitForRequests polls (sends happen asynchronously) until want requests
// have arrived or the timeout expires, returning whatever arrived.
func waitForRequests(get func() []telegramRequest, want int) []telegramRequest {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := get(); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return get()
}

func TestTelegramEventSink_IterationFinished_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")

	stats := IterationStats{ElapsedSeconds: 42, PeakContextTokens: 1234, InProgress: 2, Completed: 3, Total: 5}
	sink.IterationFinished(tickets.Ticket{Identifier: "04", Title: "Telegram decorator"}, "epic", stats)

	if got := inner.snapshot(); len(got) != 1 || got[0] != "IterationFinished" {
		t.Errorf("inner events = %v, want [IterationFinished]", got)
	}

	reqs := waitForRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	if reqs[0].ChatID != "chat-1" {
		t.Errorf("chat_id = %q, want %q", reqs[0].ChatID, "chat-1")
	}
	if reqs[0].ParseMode != "MarkdownV2" {
		t.Errorf("parse_mode = %q, want %q", reqs[0].ParseMode, "MarkdownV2")
	}
	want := telegramStyle.iterationFinishedText(tickets.Ticket{Identifier: "04", Title: "Telegram decorator"}, "epic", stats)
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_IterationPaused_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")

	sink.IterationPaused("iter-04", PauseNeedsAttention, "agent blocked on permission prompt")

	if got := inner.snapshot(); len(got) != 1 || got[0] != "IterationPaused" {
		t.Errorf("inner events = %v, want [IterationPaused]", got)
	}

	reqs := waitForRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := telegramStyle.iterationPausedText("iter-04", PauseNeedsAttention, "agent blocked on permission prompt")
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_EpicComplete_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")

	sink.EpicComplete("epic", 5, 300)

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}

	reqs := waitForRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := telegramStyle.epicCompleteText("epic", 5, 300)
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_OtherMethods_ForwardWithoutSendingAnyRequest(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")

	sink.NoTicketsFound("epic")
	sink.AlreadyComplete("epic", 1, 2)
	sink.TicketReverted("01")
	sink.TicketReattached("01", "iter-01", "/repo", "sess-1")
	sink.TicketStillNeedsAttention("01")
	sink.TicketClaimed(tickets.Ticket{Identifier: "01"})
	sink.IterationStarted("01", "iter-01", "/repo", "sess-1")
	sink.IterationResumed("iter-01", PauseRateLimit)
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

	want := []string{
		"NoTicketsFound", "AlreadyComplete", "TicketReverted", "TicketReattached",
		"TicketStillNeedsAttention", "TicketClaimed", "IterationStarted", "IterationResumed",
		"TranscriptLine", "ContextOccupancy", "CherryPickStarted", "ConflictResolutionStarted",
		"SmartZoneCompactStarted", "SmartZoneFinishingUp", "SmartZoneRecovered",
		"TicketCleanupFinished", "TicketRecovering", "TicketRecovered", "TicketUnrecoverable",
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

	// Give any (unwanted) async send a moment to have fired before asserting none did.
	time.Sleep(50 * time.Millisecond)
	if reqs := getRequests(); len(reqs) != 0 {
		t.Errorf("requests = %v, want none", reqs)
	}
}

func TestTelegramEventSink_FailingServer_NeverErrorsOrBlocks(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusInternalServerError)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	waitForRequests(getRequests, 1)
}

func TestTelegramEventSink_UnreachableServer_NeverErrorsOrBlocks(t *testing.T) {
	inner := &recordingSink{}
	// A closed server's URL is unreachable but still well-formed, which is
	// what a broken/unreachable Telegram API looks like to the client.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}
}

func TestSendTelegramTestMessage_SendsSynchronouslyAndReturnsNilOnSuccess(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)

	err := sendTelegramTestMessage("tok", "chat-1", server.URL)
	if err != nil {
		t.Fatalf("sendTelegramTestMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := telegramStyle.testMessageText()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
	if reqs[0].ParseMode != "MarkdownV2" {
		t.Errorf("parse_mode = %q, want %q", reqs[0].ParseMode, "MarkdownV2")
	}
}

func TestSendTelegramTestMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	server, _ := fakeTelegramServer(t, http.StatusInternalServerError)

	if err := sendTelegramTestMessage("tok", "chat-1", server.URL); err == nil {
		t.Fatal("sendTelegramTestMessage: want error, got nil")
	}
}

func TestSendTelegramMessage_SendsGivenTextEscaped(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)

	err := sendTelegramMessage("tok", "chat-1", server.URL, "hello-world!")
	if err != nil {
		t.Fatalf("sendTelegramMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := telegramStyle.escape("hello-world!")
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSendTelegramMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	server, _ := fakeTelegramServer(t, http.StatusInternalServerError)

	if err := sendTelegramMessage("tok", "chat-1", server.URL, "hello"); err == nil {
		t.Fatal("sendTelegramMessage: want error, got nil")
	}
}

func TestTelegramEventSink_LogsNotificationSentToRunLog(t *testing.T) {
	dir := t.TempDir()
	server, _ := fakeTelegramServer(t, http.StatusOK)
	sink := newTelegramEventSink(&recordingSink{}, "tok", "chat-1", server.URL, dir, "epic")

	sink.EpicComplete("epic", 1, 0)

	var events []Event
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationSent || events[0].Channel != "telegram" || events[0].NotifyKind != notifyKindEpicComplete {
		t.Fatalf("run-log events = %#v, want one notification-sent/telegram/epic-complete", events)
	}
}

func TestTelegramEventSink_LogsNotificationFailedToRunLog(t *testing.T) {
	dir := t.TempDir()
	server, _ := fakeTelegramServer(t, http.StatusInternalServerError)
	sink := newTelegramEventSink(&recordingSink{}, "tok", "chat-1", server.URL, dir, "epic")

	sink.EpicComplete("epic", 1, 0)

	var events []Event
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationFailed || events[0].Channel != "telegram" || events[0].Reason == "" {
		t.Fatalf("run-log events = %#v, want one notification-failed/telegram with a non-empty reason", events)
	}
}

func TestTelegramEventSink_LogsNotificationFailedToRunLog_RedactsBotToken(t *testing.T) {
	dir := t.TempDir()
	const secretToken = "super-secret-bot-token-123"
	// A closed server's URL is unreachable but well-formed, so http.Client.Do
	// returns a *url.Error whose Error() embeds the full request URL —
	// including the bot token baked into its path by sendSync's endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newTelegramEventSink(&recordingSink{}, secretToken, "chat-1", server.URL, dir, "epic")

	sink.EpicComplete("epic", 1, 0)

	var events []Event
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationFailed || events[0].Reason == "" {
		t.Fatalf("run-log events = %#v, want one notification-failed with a non-empty reason", events)
	}
	if strings.Contains(events[0].Reason, secretToken) {
		t.Errorf("Reason = %q, must not contain the bot token", events[0].Reason)
	}
}
