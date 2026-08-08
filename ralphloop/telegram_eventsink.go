package ralphloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/elentok/gx/tickets"
)

const (
	telegramAPIBaseURL  = "https://api.telegram.org"
	telegramSendTimeout = 10 * time.Second
)

// telegramEventSink decorates another EventSink: every call forwards to
// inner unchanged first, and IterationFinished/IterationPaused/EpicComplete
// additionally fire off a Telegram notification — the events an operator
// away from the terminal actually needs a heads-up about. Every other event
// is a pure pass-through.
type telegramEventSink struct {
	EventSink
	botToken   string
	chatID     string
	apiBaseURL string
	httpClient *http.Client
	// scratchDir/epicName locate the run's run-log.jsonl for
	// notification-sent/notification-failed logging (see eventlog.go); both
	// empty is a valid no-op state (logEvent no-ops on empty scratchDir/
	// epicName), used by SendTelegramTestMessage's standalone send.
	scratchDir string
	epicName   string
}

// NewTelegramEventSink returns an EventSink that wraps inner and additionally
// sends a Telegram message via botToken/chatID for IterationFinished,
// IterationPaused, and EpicComplete. Sends run in their own goroutine and
// never return an error or block the caller: a failed send is logged via
// gx's logger and, tagged with the triggering event, appended to
// scratchDir/epicName's run-log.jsonl.
func NewTelegramEventSink(inner EventSink, botToken, chatID, scratchDir, epicName string) EventSink {
	return newTelegramEventSink(inner, botToken, chatID, telegramAPIBaseURL, scratchDir, epicName)
}

func newTelegramEventSink(inner EventSink, botToken, chatID, apiBaseURL, scratchDir, epicName string) *telegramEventSink {
	return &telegramEventSink{
		EventSink:  inner,
		botToken:   botToken,
		chatID:     chatID,
		apiBaseURL: apiBaseURL,
		httpClient: &http.Client{Timeout: telegramSendTimeout},
		scratchDir: scratchDir,
		epicName:   epicName,
	}
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

// SendTelegramMessage synchronously sends arbitrary text via the Telegram Bot
// API and returns any error instead of swallowing it, for callers like `gx
// notify` that need to report success/failure directly — unlike the
// EventSink decorator's fire-and-forget send. text is escaped for
// MarkdownV2 here, so callers pass plain text.
func SendTelegramMessage(botToken, chatID, text string) error {
	return sendTelegramMessage(botToken, chatID, telegramAPIBaseURL, text)
}

func sendTelegramMessage(botToken, chatID, apiBaseURL, text string) error {
	return sendTelegramRaw(botToken, chatID, apiBaseURL, telegramStyle.escape(text))
}

// sendTelegramRaw POSTs text to the Telegram Bot API as-is, without any
// further escaping — callers are responsible for pre-escaping MarkdownV2
// text (see notification_text.go).
func sendTelegramRaw(botToken, chatID, apiBaseURL, text string) error {
	s := newTelegramEventSink(nil, botToken, chatID, apiBaseURL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), telegramSendTimeout)
	defer cancel()
	return s.sendSync(ctx, text)
}

func (s *telegramEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.EventSink.IterationFinished(ticket, epicName, stats)
	s.send(telegramStyle.iterationFinishedText(ticket, epicName, stats), notifyKindIterationFinished)
}

func (s *telegramEventSink) IterationPaused(label string, kind PauseKind, reason string) {
	s.EventSink.IterationPaused(label, kind, reason)
	s.send(telegramStyle.iterationPausedText(label, kind, reason), notifyKindIterationPaused)
}

func (s *telegramEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.EventSink.EpicComplete(epicName, completed, elapsedSeconds)
	s.send(telegramStyle.epicCompleteText(epicName, completed, elapsedSeconds), notifyKindEpicComplete)
}

// send POSTs text to the Telegram Bot API's sendMessage endpoint in its own
// goroutine, so a slow or unreachable API never blocks the caller. Any
// failure is logged via gx's logger and otherwise swallowed — it never
// surfaces to the caller — but every attempt, success or failure, is also
// durably recorded to run-log.jsonl tagged with notifyKind (the live event
// that triggered it), since logger.Debug alone leaves no trace an operator
// checking a run would see. See sendSync for the synchronous, error-returning
// core.
func (s *telegramEventSink) send(text, notifyKind string) {
	sendNotification(s.scratchDir, s.epicName, "telegram", notifyKind, telegramSendTimeout, func(ctx context.Context) error {
		return s.sendSync(ctx, text)
	})
}

// sendSync POSTs text to the Telegram Bot API's sendMessage endpoint and
// waits for the response, returning any failure (marshal error, network
// error, non-2xx response) instead of swallowing it. text is expected to
// already be MarkdownV2-escaped (see notification_text.go); parse_mode tells
// Telegram to render its "*bold*" markers instead of showing them literally.
func (s *telegramEventSink) sendSync(ctx context.Context, text string) error {
	payload, err := json.Marshal(map[string]string{
		"chat_id":    s.chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", s.apiBaseURL, url.PathEscape(s.botToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send failed with status %d", resp.StatusCode)
	}
	return nil
}
