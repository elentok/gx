package ralphloop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// telegramRequest captures one decoded call to the fake Telegram API's
// sendMessage endpoint.
type telegramRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
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
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL)

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
	want := "Ralph-loop: finished ticket epic/04 Telegram decorator in 42s 1234tok (2 tickets in progress, 2 out of 5 left)"
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_IterationPaused_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL)

	sink.IterationPaused("iter-04", PauseNeedsAttention, "agent blocked on permission prompt")

	if got := inner.snapshot(); len(got) != 1 || got[0] != "IterationPaused" {
		t.Errorf("inner events = %v, want [IterationPaused]", got)
	}

	reqs := waitForRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := "Ralph-loop: iter-04 paused: agent blocked on permission prompt"
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_EpicComplete_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL)

	sink.EpicComplete("epic", 5, 300)

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}

	reqs := waitForRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := "Ralph-loop: epic epic complete, 5 ticket(s) landed in 300s"
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_OtherMethods_ForwardWithoutSendingAnyRequest(t *testing.T) {
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL)

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
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL)

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
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL)

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}
}
