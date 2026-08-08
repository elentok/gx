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

// TestTelegramEventSink_EpicComplete_PostsTelegramWireFormat exercises the
// real telegramTransport (chat_id/parse_mode/body shape) end-to-end through
// a real event; the membership/park-cardinality/message-content behavior is
// exercised transport-agnostically in chat_eventsink_test.go.
func TestTelegramEventSink_EpicComplete_PostsTelegramWireFormat(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.EpicComplete("epic", 5, 300)
	sink.flush()

	if got := inner.snapshot(); len(got) != 1 || got[0] != "EpicComplete" {
		t.Errorf("inner events = %v, want [EpicComplete]", got)
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
	want := telegramStyle.epicCompleteText("epic", EpicCounts{}, 5, 300, 0).String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

// TestTelegramEventSink_DrainComplete_PostsTelegramWireFormat mirrors
// TestTelegramEventSink_EpicComplete_PostsTelegramWireFormat for the
// drain-queue epic's distinct end-of-drain notification.
func TestTelegramEventSink_DrainComplete_PostsTelegramWireFormat(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeTelegramServer(t, http.StatusOK)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, "", "")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	sink.DrainComplete("epic", 5, 300)
	sink.flush()

	if got := inner.snapshot(); len(got) != 1 || got[0] != "DrainComplete" {
		t.Errorf("inner events = %v, want [DrainComplete]", got)
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
	want := telegramStyle.drainCompleteText("epic", EpicCounts{}, 5, 300, 0).String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestTelegramEventSink_FailingServer_NeverErrorsOrBlocks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, getRequests := fakeTelegramServer(t, http.StatusInternalServerError)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, dir, "epic")
	defer sink.Close()
	sink.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")

	start := time.Now()
	sink.EpicComplete("epic", 1, 0)
	sink.flush()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("EpicComplete blocked for %s, want near-instant return", elapsed)
	}

	waitForRequests(getRequests, 1)
	// Drain the retry-and-log goroutine sendNotification spawns (it sleeps
	// notificationRetryBackoff then retries) so it doesn't outlive the test.
	waitForRunLogEvent(t, dir, "epic")
}

func TestTelegramEventSink_UnreachableServer_NeverErrorsOrBlocks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inner := &recordingSink{}
	// A closed server's URL is unreachable but still well-formed, which is
	// what a broken/unreachable Telegram API looks like to the client.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, dir, "epic")
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

// waitForRunLogEvent polls run-log.jsonl until sendNotification's background
// goroutine has logged its failure outcome, so callers can be sure that
// goroutine — including its notificationRetryBackoff sleep and second
// attempt (each bounded by the transport's timeout) — has exited before the
// test returns.
func waitForRunLogEvent(t *testing.T, dir, epicName string) []Event {
	t.Helper()
	var events []Event
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _ = readEvents(dir, epicName)
		if len(events) > 0 && events[0].Type == eventNotificationFailed {
			return events
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a notification-failed run-log event in %s", dir)
	return nil
}

// unescapedTelegramMarkdownV2Char scans text for a reserved MarkdownV2
// character that isn't backslash-escaped, returning it (and its index) or
// ("", -1) if text is fully escaped. "*" is exempted: every message builder
// in notification_text.go deliberately emits it unescaped as the bold
// headline delimiter, so a literal "*" is valid syntax, not stray data.
// telegramMarkdownV2SpecialChars mirrors chatmarkup.Telegram's reserved
// character set for these tests' own escaping assertions, now that
// notification_text.go no longer declares this itself (chatmarkup.Style
// owns escaping — see chatmarkup/styles.go).
const telegramMarkdownV2SpecialChars = "_*[]()~`>#+-=|{}.!\\"

func unescapedTelegramMarkdownV2Char(text string) (string, int) {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' {
			i++ // skip the escaped char
			continue
		}
		if runes[i] == '*' {
			continue
		}
		if strings.ContainsRune(telegramMarkdownV2SpecialChars, runes[i]) {
			return string(runes[i]), i
		}
	}
	return "", -1
}

// TestRenderBatch_TelegramStyle_MultipleItems_SeparatorIsEscaped guards
// against the batch-separator MarkdownV2 bug directly: renderBatch's own
// "\n---\n" join must not introduce an unescaped reserved character, since
// nothing downstream of it (sendRaw/sendSync) escapes the joined text again
// — chat_eventsink.go's existing separator test only ever exercised
// slackStyle, whose dialect doesn't reserve "-", so it never caught this.
func TestRenderBatch_TelegramStyle_MultipleItems_SeparatorIsEscaped(t *testing.T) {
	t.Parallel()
	items := []batchedMessage{
		{text: telegramStyle.testMessageText(), kind: "a"},
		{text: telegramStyle.testMessageText(), kind: "b"},
	}
	got := renderBatch(telegramStyle, items)
	if ch, idx := unescapedTelegramMarkdownV2Char(got.String()); idx != -1 {
		t.Errorf("renderBatch produced unescaped %q at index %d in %q", ch, idx, got)
	}
}

// TestSendTelegramTestBatch_PostsRenderBatchOutputAsIs pins the `gx notify
// --test-batch` entry point to renderBatch's exact output — the live
// reproduction path a person runs against the real Bot API, mirrored here
// against a fake server as documentation and to catch a future regression
// where the batch send stops matching what a real flush() does.
func TestSendTelegramTestBatch_PostsRenderBatchOutputAsIs(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeTelegramServer(t, http.StatusOK)

	if err := sendTelegramTestBatch("tok", "chat-1", server.URL); err != nil {
		t.Fatalf("sendTelegramTestBatch: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := renderBatch(telegramStyle, []batchedMessage{
		{text: telegramStyle.testMessageText(), kind: "test"},
		{text: telegramStyle.testMessageText(), kind: "test"},
	})
	if reqs[0].Text != want.String() {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

func TestSendTelegramTestMessage_SendsSynchronouslyAndReturnsNilOnSuccess(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeTelegramServer(t, http.StatusOK)

	err := sendTelegramTestMessage("tok", "chat-1", server.URL)
	if err != nil {
		t.Fatalf("sendTelegramTestMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := telegramStyle.testMessageText().String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
	if reqs[0].ParseMode != "MarkdownV2" {
		t.Errorf("parse_mode = %q, want %q", reqs[0].ParseMode, "MarkdownV2")
	}
}

func TestSendTelegramTestMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	t.Parallel()
	server, _ := fakeTelegramServer(t, http.StatusInternalServerError)

	if err := sendTelegramTestMessage("tok", "chat-1", server.URL); err == nil {
		t.Fatal("sendTelegramTestMessage: want error, got nil")
	}
}

func TestSendTelegramMessage_SendsGivenTextEscaped(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeTelegramServer(t, http.StatusOK)

	err := sendTelegramMessage("tok", "chat-1", server.URL, "hello-world!")
	if err != nil {
		t.Fatalf("sendTelegramMessage: %v", err)
	}

	reqs := getRequests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly 1", reqs)
	}
	want := telegramStyle.chatStyle.Escape("hello-world!").String()
	if reqs[0].Text != want {
		t.Errorf("text = %q, want %q", reqs[0].Text, want)
	}
}

// TestTelegramTransport_SendSync_NonOKResponse_ResultCarriesStatusAndDescription
// pins ticket 04a's transport seam: a 400 response whose body carries a
// description field surfaces both the status code and that description in
// the returned sendResult, not just a generic error.
func TestTelegramTransport_SendSync_NonOKResponse_ResultCarriesStatusAndDescription(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities"}`))
	}))
	t.Cleanup(server.Close)

	transport := newTelegramTransport("tok", "chat-1", server.URL)
	result, err := transport.sendSync(t.Context(), telegramStyle.testMessageText())
	if err == nil {
		t.Fatal("sendSync: want error for a 400 response")
	}
	if result.StatusCode != http.StatusBadRequest {
		t.Errorf("result.StatusCode = %d, want %d", result.StatusCode, http.StatusBadRequest)
	}
	if result.Description != "Bad Request: can't parse entities" {
		t.Errorf("result.Description = %q, want the response body's description field", result.Description)
	}
}

// TestTelegramTransport_SendSync_OKResponse_ResultCarriesStatus pins the
// success path: a 2xx response returns a nil error and a sendResult carrying
// that status code.
func TestTelegramTransport_SendSync_OKResponse_ResultCarriesStatus(t *testing.T) {
	t.Parallel()
	server, _ := fakeTelegramServer(t, http.StatusOK)

	transport := newTelegramTransport("tok", "chat-1", server.URL)
	result, err := transport.sendSync(t.Context(), telegramStyle.testMessageText())
	if err != nil {
		t.Fatalf("sendSync: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("result.StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}
}

func TestSendTelegramMessage_ReturnsErrorOnFailingServer(t *testing.T) {
	t.Parallel()
	server, _ := fakeTelegramServer(t, http.StatusInternalServerError)

	if err := sendTelegramMessage("tok", "chat-1", server.URL, "hello"); err == nil {
		t.Fatal("sendTelegramMessage: want error, got nil")
	}
}

// fakeTelegramMarkdownRejectingServer 400s with a MarkdownV2-parse
// description whenever parse_mode is set, and 200s once it's omitted —
// standing in for a real MarkdownV2 parse rejection that a plain-text retry
// resolves.
func fakeTelegramMarkdownRejectingServer(t *testing.T) (*httptest.Server, func() []telegramRequest) {
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
		if req.ParseMode != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities: character '-' is reserved"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
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

// TestTelegramTransport_SendSync_MarkdownParseRejection_RetriesPlainAndSucceeds
// pins ticket 05's safety net: a 400 whose description names a MarkdownV2
// parse failure triggers exactly one plain-text (no parse_mode) retry, which
// succeeds and reports Degraded.
func TestTelegramTransport_SendSync_MarkdownParseRejection_RetriesPlainAndSucceeds(t *testing.T) {
	t.Parallel()
	server, getRequests := fakeTelegramMarkdownRejectingServer(t)

	transport := newTelegramTransport("tok", "chat-1", server.URL)
	text := telegramStyle.chatStyle.Escape("hello - world")
	result, err := transport.sendSync(t.Context(), text)
	if err != nil {
		t.Fatalf("sendSync: %v", err)
	}
	if !result.Degraded {
		t.Errorf("result.Degraded = false, want true")
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("result.StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}

	reqs := getRequests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %v, want exactly 2 (markdown attempt + plain fallback)", reqs)
	}
	if reqs[0].ParseMode != "MarkdownV2" {
		t.Errorf("first request parse_mode = %q, want MarkdownV2", reqs[0].ParseMode)
	}
	if reqs[1].ParseMode != "" {
		t.Errorf("fallback request parse_mode = %q, want empty", reqs[1].ParseMode)
	}
	if reqs[1].Text != "hello - world" {
		t.Errorf("fallback request text = %q, want unescaped plain text", reqs[1].Text)
	}
}

// TestTelegramTransport_SendSync_UnrelatedBadRequest_NoFallback pins the
// negative case: a 400 for a reason other than a MarkdownV2 parse failure
// (e.g. an invalid chat_id) must not trigger the plain-text fallback.
func TestTelegramTransport_SendSync_UnrelatedBadRequest_NoFallback(t *testing.T) {
	t.Parallel()
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`))
	}))
	t.Cleanup(server.Close)

	transport := newTelegramTransport("tok", "chat-1", server.URL)
	result, err := transport.sendSync(t.Context(), telegramStyle.testMessageText())
	if err == nil {
		t.Fatal("sendSync: want error, got nil")
	}
	if result.Degraded {
		t.Errorf("result.Degraded = true, want false")
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d, want exactly 1 (no fallback attempt)", requestCount)
	}
}

// TestTelegramEventSink_MarkdownParseRejection_LogsDegradedAndTogglesToast
// covers the ticket-05 end-to-end path: a successful fallback send logs a
// distinct notification-degraded run-log line (not notification-sent) and
// still reaches NotificationFailed for the TUI toast pipeline.
func TestTelegramEventSink_MarkdownParseRejection_LogsDegradedAndTogglesToast(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, _ := fakeTelegramMarkdownRejectingServer(t)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, dir, "epic")
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
	if len(events) != 1 || events[0].Type != eventNotificationDegraded || events[0].Channel != "telegram" {
		t.Fatalf("run-log events = %#v, want one notification-degraded/telegram", events)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls := inner.snapshot(); len(calls) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls := inner.snapshot(); len(calls) != 2 || calls[0] != "EpicComplete" || calls[1] != "NotificationFailed" {
		t.Errorf("inner events = %v, want [EpicComplete NotificationFailed]", calls)
	}
}

func TestTelegramEventSink_LogsNotificationSentToRunLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, _ := fakeTelegramServer(t, http.StatusOK)
	sink := newTelegramEventSink(&recordingSink{}, "tok", "chat-1", server.URL, dir, "epic")
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
	if len(events) != 1 || events[0].Type != eventNotificationSent || events[0].Channel != "telegram" || events[0].NotifyKind != notifyKindBatch {
		t.Fatalf("run-log events = %#v, want one notification-sent/telegram/batch", events)
	}
}

func TestTelegramEventSink_LogsNotificationFailedToRunLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	server, _ := fakeTelegramServer(t, http.StatusInternalServerError)
	inner := &recordingSink{}
	sink := newTelegramEventSink(inner, "tok", "chat-1", server.URL, dir, "epic")
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
	if len(events) != 1 || events[0].Type != eventNotificationFailed || events[0].Channel != "telegram" || events[0].Reason == "" {
		t.Fatalf("run-log events = %#v, want one notification-failed/telegram with a non-empty reason", events)
	}

	// The failed send must also reach the embedded EventSink as a
	// NotificationFailed call, not just run-log.jsonl — that's what lets a
	// live TUI turn a Telegram 400 into an in-app toast (see
	// ui/tickets/loop_registry.go's LiveEventNotificationFailed case).
	if calls := inner.snapshot(); len(calls) != 2 || calls[0] != "EpicComplete" || calls[1] != "NotificationFailed" {
		t.Errorf("inner events = %v, want [EpicComplete NotificationFailed]", calls)
	}
}

func TestTelegramEventSink_LogsNotificationFailedToRunLog_RedactsBotToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const secretToken = "super-secret-bot-token-123"
	// A closed server's URL is unreachable but well-formed, so http.Client.Do
	// returns a *url.Error whose Error() embeds the full request URL —
	// including the bot token baked into its path by sendSync's endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	sink := newTelegramEventSink(&recordingSink{}, secretToken, "chat-1", server.URL, dir, "epic")
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
	if strings.Contains(events[0].Reason, secretToken) {
		t.Errorf("Reason = %q, must not contain the bot token", events[0].Reason)
	}
}
