package ralphloop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

// TestSlackEventSink_EpicComplete_PostsSlackWireFormat exercises the real
// slackTransport (POST body shape) end-to-end through a real event; the
// membership/park-cardinality/message-content behavior is exercised
// transport-agnostically in chat_eventsink_test.go.
func TestSlackEventSink_EpicComplete_PostsSlackWireFormat(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.EpicComplete("epic", 5, 300)
	sink.flush()

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.epicCompleteText("epic", EpicCounts{}, 5, 300, 0).String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

// TestSlackEventSink_DrainComplete_PostsSlackWireFormat mirrors
// TestSlackEventSink_EpicComplete_PostsSlackWireFormat for the drain-queue
// epic's distinct end-of-drain notification.
func TestSlackEventSink_DrainComplete_PostsSlackWireFormat(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.DrainComplete("epic", 5, 300)
	sink.flush()

	if got := inner.snapshot(); len(got) != 1 || got[0] != "DrainComplete" {
		t.Errorf("inner events = %v, want [DrainComplete]", got)
	}

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.drainCompleteText("epic", EpicCounts{}, 5, 300, 0).String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

// TestSlackEventSink_CountsLine_AppearsOnEpicAndTicketMilestones pins ticket
// 27's placement rule: a counts line rides on epic started, ticket landed,
// ticket parked, and epic complete — the messages where counts materially
// moved — and on neither IterationPaused nor EpicParked, which chat also
// sends text for but where the counts haven't changed since the prior
// message. DrainComplete is a fifth counts-line kind added later; it's
// covered by its own test above rather than folded into this one.
func TestSlackEventSink_CountsLine_AppearsOnEpicAndTicketMilestones(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeSlackServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, "", "")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.EpicStarted("epic", 0, 3)
	sink.IterationFinished(tickets.Ticket{Identifier: "01"}, "epic", IterationStats{Completed: 1, Total: 3})
	sink.TicketNeedsHuman("02", "epic", "needs-answer", "no commits landed")
	sink.IterationPaused("03", "iter-03", PauseRateLimit, "rate limit hit")
	sink.EpicParked("epic", []StalledTicket{{Identifier: "03"}})
	sink.EpicComplete("epic", 1, 300)
	sink.flush()

	reqs := waitForSlackRequests(getRequests, 1)
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1 batched request", reqs)
	}

	// all six messages land in one flush, separator-joined (see renderBatch);
	// match each by a marker unique to its message instead of assuming
	// positional order. Emoji markers pick out EpicStarted/EpicParked/
	// EpicComplete, which otherwise share the same "[gx] epic" identity line.
	segments := strings.Split(reqs[0].Text, "\n---\n")
	cases := []struct {
		label      string
		marker     string
		wantCounts bool
	}{
		{"EpicStarted", "\U0001f680", true},
		{"IterationFinished", "epic/01", true},
		{"TicketNeedsHuman", "epic/02", true},
		{"IterationPaused", "iter-03:", false},
		{"EpicParked", "\U0001f17f", false},
		{"EpicComplete", "\U0001f389", true},
	}
	for _, tt := range cases {
		var matched string
		found := false
		for _, seg := range segments {
			if strings.Contains(seg, tt.marker) {
				matched = seg
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: no segment matched marker %q among %v", tt.label, tt.marker, segments)
			continue
		}
		got := strings.Contains(matched, "done ·") || strings.Contains(matched, "done\n")
		if got != tt.wantCounts {
			t.Errorf("%s segment = %q, contains a counts line = %v, want %v", tt.label, matched, got, tt.wantCounts)
		}
	}
}

func TestSlackEventSink_FailingServer_NeverErrorsOrBlocks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, getRequests := fakeSlackServer(t, http.StatusInternalServerError)
	inner := &recordingSink{}
	sink := newSlackEventSink(inner, server.URL, dir, "epic")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	sink.flush()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	waitForSlackRequests(getRequests, 1)
	// Drain the retry-and-log goroutine sendNotification spawns (it sleeps
	// notificationRetryBackoff then retries) so it doesn't outlive the test.
	waitForRunLogEvent(t, dir, "epic")
}

func TestSlackEventSink_UnreachableServer_NeverErrorsOrBlocks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inner := &recordingSink{}
	// A closed server's URL is unreachable but still well-formed, which is
	// what a broken/unreachable Slack webhook looks like to the client.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newSlackEventSink(inner, server.URL, dir, "epic")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	sink.flush()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
	}
	// Drain the retry-and-log goroutine sendNotification spawns (it sleeps
	// notificationRetryBackoff then retries) so it doesn't outlive the test.
	waitForRunLogEvent(t, dir, "epic")
}

func TestSendSlackTestMessage_SendsSynchronouslyAndReturnsNilOnSuccess(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeSlackServer(t, http.StatusOK)

	err := SendSlackTestMessage(server.URL)
	if err != nil {
		t.Fatalf("SendSlackTestMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.testMessageText().String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSendSlackTestMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	t.Parallel()
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)

	if err := SendSlackTestMessage(server.URL); err == nil {
		t.Fatal("SendSlackTestMessage: want error, got nil")
	}
}

func TestSendSlackMessage_SendsGivenTextEscaped(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeSlackServer(t, http.StatusOK)

	err := SendSlackMessage(server.URL, "hello <world> & friends")
	if err != nil {
		t.Fatalf("SendSlackMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := slackStyle.chatStyle.Escape("hello <world> & friends").String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

// TestSlackTransport_SendSync_ResultCarriesStatusCode pins ticket 04a's
// transport seam for Slack: the webhook response body carries no field
// comparable to Telegram's description (see slackTransport.sendSync's doc
// comment), so only the status code is asserted here.
func TestSlackTransport_SendSync_ResultCarriesStatusCode(t *testing.T) {
	t.Parallel()
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)

	transport := newSlackTransport(server.URL)
	result, err := transport.sendSync(t.Context(), slackStyle.testMessageText())
	if err == nil {
		t.Fatal("sendSync: want error for a 500 response")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("result.StatusCode = %d, want %d", result.StatusCode, http.StatusInternalServerError)
	}
	if result.Description != "" {
		t.Errorf("result.Description = %q, want empty — Slack's webhook body carries no comparable field", result.Description)
	}
}

func TestSendSlackMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	t.Parallel()
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)

	if err := SendSlackMessage(server.URL, "hello"); err == nil {
		t.Fatal("SendSlackMessage: want error, got nil")
	}
}

func TestSlackEventSink_LogsNotificationSentToRunLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, _ := fakeSlackServer(t, http.StatusOK)
	sink := newSlackEventSink(&recordingSink{}, server.URL, dir, "epic")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.EpicComplete("epic", 1, 0)
	sink.flush()

	var events []Event
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, "epic")
		if len(events) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != eventNotificationSent || events[0].Channel != "slack" || events[0].NotifyKind != notifyKindBatch {
		t.Fatalf("run-log events = %#v, want one notification-sent/slack/batch", events)
	}
}

func TestSlackEventSink_LogsNotificationFailedToRunLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)
	sink := newSlackEventSink(&recordingSink{}, server.URL, dir, "epic")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.EpicComplete("epic", 1, 0)
	sink.flush()

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
	t.Parallel()
	dir := t.TempDir()
	const secretPath = "T00/B00/super-secret-webhook-token"
	// A closed server's URL is unreachable but well-formed, so http.Client.Do
	// returns a *url.Error whose Error() embeds the full request URL —
	// including the secret Slack webhook path segment.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newSlackEventSink(&recordingSink{}, server.URL+"/services/"+secretPath, dir, "epic")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.EpicComplete("epic", 1, 0)
	sink.flush()

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
