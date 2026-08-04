package ralphloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/elentok/gx/logger"
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
}

// NewTelegramEventSink returns an EventSink that wraps inner and additionally
// sends a Telegram message via botToken/chatID for IterationFinished,
// IterationPaused, and EpicComplete. Sends run in their own goroutine and
// never return an error or block the caller: a failed send is logged via
// gx's logger and otherwise swallowed.
func NewTelegramEventSink(inner EventSink, botToken, chatID string) EventSink {
	return newTelegramEventSink(inner, botToken, chatID, telegramAPIBaseURL)
}

func newTelegramEventSink(inner EventSink, botToken, chatID, apiBaseURL string) *telegramEventSink {
	return &telegramEventSink{
		EventSink:  inner,
		botToken:   botToken,
		chatID:     chatID,
		apiBaseURL: apiBaseURL,
		httpClient: &http.Client{Timeout: telegramSendTimeout},
	}
}

func (s *telegramEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.EventSink.IterationFinished(ticket, epicName, stats)
	remaining := stats.Total - stats.Completed
	s.send(fmt.Sprintf(
		"Ralph-loop: finished ticket %s/%s %s in %ds %dtok (%d tickets in progress, %d out of %d left)",
		epicName, ticket.Identifier, ticket.Title, stats.ElapsedSeconds, stats.PeakContextTokens,
		stats.InProgress, remaining, stats.Total,
	))
}

func (s *telegramEventSink) IterationPaused(label string, kind PauseKind, reason string) {
	s.EventSink.IterationPaused(label, kind, reason)
	s.send(fmt.Sprintf("Ralph-loop: %s paused: %s", label, reason))
}

func (s *telegramEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.EventSink.EpicComplete(epicName, completed, elapsedSeconds)
	s.send(fmt.Sprintf("Ralph-loop: epic %s complete, %d ticket(s) landed in %ds", epicName, completed, elapsedSeconds))
}

// send POSTs text to the Telegram Bot API's sendMessage endpoint in its own
// goroutine, so a slow or unreachable API never blocks the caller. Any
// failure (marshal error, network error, timeout, non-2xx response) is
// logged and otherwise swallowed — it never surfaces to the caller.
func (s *telegramEventSink) send(text string) {
	go func() {
		payload, err := json.Marshal(map[string]string{
			"chat_id": s.chatID,
			"text":    text,
		})
		if err != nil {
			logger.Debug("telegram: failed to marshal message: %v\n", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), telegramSendTimeout)
		defer cancel()

		endpoint := fmt.Sprintf("%s/bot%s/sendMessage", s.apiBaseURL, url.PathEscape(s.botToken))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			logger.Debug("telegram: failed to build request: %v\n", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			logger.Debug("telegram: send failed: %v\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Debug("telegram: send failed with status %d\n", resp.StatusCode)
		}
	}()
}
