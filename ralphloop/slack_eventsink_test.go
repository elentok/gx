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

// slackRequest captures one decoded call to the fake Slack webhook.
type slackRequest struct {
	Text string `json:"text"`
}

// fakeSlackServer starts an httptest.Server standing in for a Slack workflow
// webhook and returns it alongside a thread-safe accessor for the requests
// it received, since sends happen from their own goroutine.
func fakeSlackServer(t *testing.T, statusCode int) (*httptest.Server, func() []slackRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []slackRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req slackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(server.Close)

	return server, func() []slackRequest {
		mu.Lock()
		defer mu.Unlock()
		out := make([]slackRequest, len(requests))
		copy(out, requests)
		return out
	}
}

func waitForSlackRequests(get func() []slackRequest, want int) []slackRequest {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := get(); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return get()
}

func TestSlackEventSink_IterationFinished_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")

	stats := IterationStats{ElapsedSeconds: 42, PeakContextTokens: 1234, InProgress: 2, Completed: 3, Total: 5}
	sink.IterationFinished(tickets.Ticket{Identifier: "04", Title: "Slack decorator"}, "epic", stats)

	if got := inner.snapshot(); len(got) != 1 || got[0] != "IterationFinished" {
		t.Errorf("inner events = %v, want [IterationFinished]", got)
	}

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.iterationFinishedText(tickets.Ticket{Identifier: "04", Title: "Slack decorator"}, "epic", stats)
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSlackEventSink_IterationPaused_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")

	sink.IterationPaused("iter-04", PauseNeedsAttention, "agent blocked on permission prompt")

	if got := inner.snapshot(); len(got) != 1 || got[0] != "IterationPaused" {
		t.Errorf("inner events = %v, want [IterationPaused]", got)
	}

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.iterationPausedText("iter-04", PauseNeedsAttention, "agent blocked on permission prompt")
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSlackEventSink_TicketNeedsInfo_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")

	sink.TicketNeedsInfo("04", "epic")

	if got := inner.snapshot(); len(got) != 1 || got[0] != "TicketNeedsInfo" {
		t.Errorf("inner events = %v, want [TicketNeedsInfo]", got)
	}

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.ticketNeedsInfoText("04", "epic")
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
	if paused := slackStyle.iterationPausedText("epic/04", PauseNeedsAttention, "stuck"); reqs[0].Text == paused {
		t.Errorf("needs-info text matched iteration-paused text: %q", reqs[0].Text)
	}
}

func TestSlackEventSink_EpicComplete_SendsOneMessageAndForwards(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")

	sink.EpicComplete("epic", 5, 300)

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.epicCompleteText("epic", 5, 300)
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSlackEventSink_OtherMethods_ForwardWithoutSendingAnyRequest(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")

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

func TestSlackEventSink_FailingServer_NeverErrorsOrBlocks(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusInternalServerError)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	waitForSlackRequests(getRequests, 1)
}

func TestSlackEventSink_UnreachableServer_NeverErrorsOrBlocks(t *testing.T) {
	inner := &recordingSink{}
	// A closed server's URL is unreachable but still well-formed, which is
	// what a broken/unreachable Slack webhook looks like to the client.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newSlackEventSink(inner, server.URL, "", "")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}
}

func TestSendSlackTestMessage_SendsSynchronouslyAndReturnsNilOnSuccess(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)

	err := SendSlackTestMessage(server.URL)
	if err != nil {
		t.Fatalf("SendSlackTestMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.testMessageText()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSendSlackTestMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)

	if err := SendSlackTestMessage(server.URL); err == nil {
		t.Fatal("SendSlackTestMessage: want error, got nil")
	}
}

func TestSendSlackMessage_SendsGivenTextEscaped(t *testing.T) {
	server, getRequests := fakeSlackServer(t, http.StatusOK)

	err := SendSlackMessage(server.URL, "hello <world> & friends")
	if err != nil {
		t.Fatalf("SendSlackMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.escape("hello <world> & friends")
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSendSlackMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)

	if err := SendSlackMessage(server.URL, "hello"); err == nil {
		t.Fatal("SendSlackMessage: want error, got nil")
	}
}

func TestSlackEventSink_LogsNotificationSentToRunLog(t *testing.T) {
	dir := t.TempDir()
	server, _ := fakeSlackServer(t, http.StatusOK)
	sink := newSlackEventSink(&recordingSink{}, server.URL, dir, "epic")

	sink.IterationPaused("iter-01", PauseNeedsAttention, "permission required")

	var events []Event
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationSent || events[0].Channel != "slack" || events[0].NotifyKind != notifyKindIterationPaused {
		t.Fatalf("run-log events = %#v, want one notification-sent/slack/iteration-paused", events)
	}
}

func TestSlackEventSink_LogsNotificationFailedToRunLog(t *testing.T) {
	dir := t.TempDir()
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)
	sink := newSlackEventSink(&recordingSink{}, server.URL, dir, "epic")

	sink.IterationPaused("iter-01", PauseNeedsAttention, "permission required")

	var events []Event
	// 4s headroom: sendNotification retries once after notificationRetryBackoff
	// (1.5s) before logging notification-failed.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationFailed || events[0].Channel != "slack" || events[0].Reason == "" {
		t.Fatalf("run-log events = %#v, want one notification-failed/slack with a non-empty reason", events)
	}
}

func TestSlackEventSink_LogsNotificationFailedToRunLog_RedactsWebhookSecret(t *testing.T) {
	dir := t.TempDir()
	const secretPath = "T00/B00/super-secret-webhook-token"
	// A closed server's URL is unreachable but well-formed, so http.Client.Do
	// returns a *url.Error whose Error() embeds the full request URL —
	// including the secret Slack webhook path segment.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newSlackEventSink(&recordingSink{}, server.URL+"/services/"+secretPath, dir, "epic")

	sink.IterationPaused("iter-01", PauseNeedsAttention, "permission required")

	var events []Event
	// 4s headroom: sendNotification retries once after notificationRetryBackoff
	// (1.5s) before logging notification-failed.
	deadline := time.Now().Add(4 * time.Second)
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
	if strings.Contains(events[0].Reason, secretPath) {
		t.Errorf("Reason = %q, must not contain the webhook secret path", events[0].Reason)
	}
}
