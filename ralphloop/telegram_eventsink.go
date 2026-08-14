package ralphloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elentok/gx/chatmarkup"
)

const (
	telegramAPIBaseURL = "https://api.telegram.org"
	// telegramSendTimeout bounds a single send attempt (both
	// http.Client.Timeout and the request context.WithTimeout deadline).
	// sendNotification retries once on failure, so the worst-case total is
	// roughly this timeout, plus a fixed backoff, plus a second attempt at
	// this timeout — kept short so that worst case stays close to a single
	// 10s attempt rather than doubling it.
	telegramSendTimeout = 5 * time.Second
)

// telegramTransport is the chatTransport that posts to the Telegram Bot
// API's sendMessage endpoint.
type telegramTransport struct {
	botToken   string
	chatID     string
	apiBaseURL string
	httpClient *http.Client
}

func newTelegramTransport(botToken, chatID, apiBaseURL string) *telegramTransport {
	return &telegramTransport{
		botToken:   botToken,
		chatID:     chatID,
		apiBaseURL: apiBaseURL,
		httpClient: &http.Client{Timeout: telegramSendTimeout},
	}
}

func (t *telegramTransport) name() string           { return "telegram" }
func (t *telegramTransport) timeout() time.Duration { return telegramSendTimeout }

// telegramErrorResponse is the Telegram Bot API's JSON error body shape —
// description names why a non-2xx response happened (e.g. a MarkdownV2-parse
// rejection vs. an invalid chat_id), which sendSync surfaces via
// sendResult.Description.
type telegramErrorResponse struct {
	Description string `json:"description"`
}

// telegramMarkdownParseFailureMarker is the substring Telegram's Bot API
// puts in a 400 response's description specifically when parse_mode
// rejected the text (e.g. "Bad Request: can't parse entities: character '-'
// is reserved..."), as opposed to an unrelated 400 cause like an invalid
// chat_id ("Bad Request: chat not found") — sendSync's plain-text fallback
// must trigger only on this specific cause, never on every 400.
const telegramMarkdownParseFailureMarker = "can't parse entities"

func isMarkdownV2ParseFailure(description string) bool {
	return strings.Contains(strings.ToLower(description), telegramMarkdownParseFailureMarker)
}

// sendSync POSTs text to the Telegram Bot API's sendMessage endpoint and
// waits for the response, returning any failure (marshal error, network
// error, non-2xx response) instead of swallowing it, alongside a sendResult
// carrying the status code and — on a non-2xx response whose body parses as
// JSON with a description field — that field's value. text is expected to
// already be MarkdownV2-escaped (see notification_text.go).
//
// If the first attempt 400s specifically because parse_mode rejected text
// (isMarkdownV2ParseFailure), this is a last-resort safety net on top of
// chatmarkup's compiler-enforced escaping (an edge case the escaper missed,
// or a future regression in that package): sendSync retries once more with
// parse_mode omitted, converting text back to unmarked plain text via
// chatmarkup.Telegram.Plain first. telegramTransport is Telegram-specific by
// construction, so it reaches for the package-level chatmarkup.Telegram
// style directly rather than taking one in from the caller. Plain() reading
// an already-sealed Text back out to a plain string is a sanctioned way to
// read the value (same as .String()) — Text's sealing controls
// *construction*, not reads — not a boundary violation despite handing the
// API a bare string instead of a Text. A successful fallback send reports
// Degraded: true so callers can log/surface it as a downgrade rather than an
// ordinary send.
func (t *telegramTransport) sendSync(ctx context.Context, text chatmarkup.Text) (sendResult, error) {
	result, err := t.post(ctx, text.String(), "MarkdownV2")
	if err == nil {
		return result, nil
	}
	if !isMarkdownV2ParseFailure(result.Description) {
		return result, err
	}

	fallback, fallbackErr := t.post(ctx, chatmarkup.Telegram.Plain(text), "")
	if fallbackErr != nil {
		return fallback, fallbackErr
	}
	fallback.Degraded = true
	fallback.Description = result.Description
	return fallback, nil
}

// post is the single POST-and-decode call sendSync makes for each attempt
// (the original MarkdownV2 send, and the plain-text fallback). parseMode
// empty omits the field entirely rather than sending it as "" — Telegram
// treats a present-but-empty parse_mode as if it were still MarkdownV2.
func (t *telegramTransport) post(ctx context.Context, text, parseMode string) (sendResult, error) {
	body := map[string]string{
		"chat_id": t.chatID,
		"text":    text,
	}
	if parseMode != "" {
		body["parse_mode"] = parseMode
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return sendResult{}, fmt.Errorf("marshal message: %w", err)
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.apiBaseURL, url.PathEscape(t.botToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return sendResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return sendResult{}, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody telegramErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return sendResult{StatusCode: resp.StatusCode, Description: errBody.Description},
			fmt.Errorf("send failed with status %d", resp.StatusCode)
	}
	return sendResult{StatusCode: resp.StatusCode}, nil
}

// NewTelegramEventSink returns an EventSink that wraps inner and additionally
// sends a Telegram message via botToken/chatID for every chat-member event
// (see chatEventSink). Sends run in their own goroutine and never return an
// error or block the caller: a failed send is logged via gx's logger and,
// tagged with the triggering event, appended to scratchDir/epicName's
// run-log.jsonl.
func NewTelegramEventSink(inner EventSink, botToken, chatID, scratchDir, epicName string) EventSink {
	return newTelegramEventSink(inner, botToken, chatID, telegramAPIBaseURL, scratchDir, epicName)
}

func newTelegramEventSink(inner EventSink, botToken, chatID, apiBaseURL, scratchDir, epicName string) *chatEventSink {
	s := newChatEventSink(inner, telegramStyle, newTelegramTransport(botToken, chatID, apiBaseURL), scratchDir, epicName)
	s.startFlushLoop(batchFlushInterval)
	return s
}

// SendTelegramTestMessage synchronously sends a fixed test notification via
// the Telegram Bot API and returns any error instead of swallowing it, for
// callers like `gx config test-notifications` that need to report
// success/failure directly — unlike the EventSink decorator's fire-and-forget
// send.
func SendTelegramTestMessage(botToken, chatID string) error {
	return sendTelegramTestMessage(botToken, chatID, telegramAPIBaseURL)
}

func sendTelegramTestMessage(botToken, chatID, apiBaseURL string) error {
	return sendTelegramRaw(botToken, chatID, apiBaseURL, telegramStyle.testMessageText())
}

// SendTelegramTestBatch synchronously sends two fixed test messages through
// the exact same batch-join path a real flush uses (renderBatch,
// chat_eventsink.go), for callers like `gx notify --test-batch` that need to
// reproduce/verify the batch-separator MarkdownV2 escaping bug live against
// the real Bot API instead of only in a unit test.
func SendTelegramTestBatch(botToken, chatID string) error {
	return sendTelegramTestBatch(botToken, chatID, telegramAPIBaseURL)
}

func sendTelegramTestBatch(botToken, chatID, apiBaseURL string) error {
	items := []batchedMessage{
		{text: telegramStyle.testMessageText(), kind: "test"},
		{text: telegramStyle.testMessageText(), kind: "test"},
	}
	return sendTelegramRaw(botToken, chatID, apiBaseURL, renderBatch(telegramStyle, items))
}

// SendTelegramMessage synchronously sends arbitrary text via the Telegram Bot
// API and returns any error instead of swallowing it, for callers like `gx
// notify` that need to report success/failure directly — unlike the
// EventSink decorator's fire-and-forget send. text is escaped for
// MarkdownV2 here, so callers pass plain text.
func SendTelegramMessage(botToken, chatID, text string) error {
	return sendTelegramMessage(botToken, chatID, telegramAPIBaseURL, text)
}

func sendTelegramMessage(botToken, chatID, apiBaseURL, text string) error {
	return sendTelegramRaw(botToken, chatID, apiBaseURL, telegramStyle.chatStyle.Escape(text))
}

// sendTelegramRaw POSTs text to the Telegram Bot API as-is, without any
// further escaping — callers are responsible for pre-escaping MarkdownV2
// text (see notification_text.go).
func sendTelegramRaw(botToken, chatID, apiBaseURL string, text chatmarkup.Text) error {
	t := newTelegramTransport(botToken, chatID, apiBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), telegramSendTimeout)
	defer cancel()
	_, err := t.sendSync(ctx, text)
	return err
}
