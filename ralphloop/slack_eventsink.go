package ralphloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elentok/gx/tickets"
)

// slackSendTimeout bounds a single send attempt (both http.Client.Timeout
// and the request context.WithTimeout deadline). sendNotification retries
// once on failure, so the worst-case total is roughly this timeout, plus a
// fixed backoff, plus a second attempt at this timeout — kept short so that
// worst case stays close to a single 10s attempt rather than doubling it.
const slackSendTimeout = 5 * time.Second

// slackEventSink decorates another EventSink exactly like telegramEventSink:
// every call forwards to inner unchanged first, and IterationFinished/
// IterationPaused/EpicComplete additionally POST a Slack message.
type slackEventSink struct {
	EventSink
	webhookURL string
	httpClient *http.Client
	// scratchDir/epicName locate the run's run-log.jsonl for
	// notification-sent/notification-failed logging (see eventlog.go); both
	// empty is a valid no-op state (logEvent no-ops on empty scratchDir/
	// epicName), used by SendSlackTestMessage's standalone send.
	scratchDir string
	epicName   string
}

// NewSlackEventSink returns an EventSink that wraps inner and additionally
// posts a Slack message to webhookURL — a Slack "Workflow" webhook that
// takes a single {"text": "..."} body — for IterationFinished,
// IterationPaused, and EpicComplete. Sends run in their own goroutine and
// never return an error or block the caller: a failed send is logged via
// gx's logger and, tagged with the triggering event, appended to
// scratchDir/epicName's run-log.jsonl.
func NewSlackEventSink(inner EventSink, webhookURL, scratchDir, epicName string) EventSink {
	return newSlackEventSink(inner, webhookURL, scratchDir, epicName)
}

func newSlackEventSink(inner EventSink, webhookURL, scratchDir, epicName string) *slackEventSink {
	return &slackEventSink{
		EventSink:  inner,
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: slackSendTimeout},
		scratchDir: scratchDir,
		epicName:   epicName,
	}
}

// SendSlackTestMessage synchronously sends a fixed test notification to the
// Slack webhook and returns any error instead of swallowing it, for callers
// like `gx config test-notifications` that need to report success/failure
// directly — unlike the EventSink decorator's fire-and-forget send.
func SendSlackTestMessage(webhookURL string) error {
	return sendSlackMessageRaw(webhookURL, slackStyle.testMessageText())
}

// SendSlackMessage synchronously posts arbitrary text to the Slack webhook
// and returns any error instead of swallowing it, for callers like `gx
// notify` that need to report success/failure directly — unlike the
// EventSink decorator's fire-and-forget send. text is escaped for Slack's
// mrkdwn dialect here, so callers pass plain text.
func SendSlackMessage(webhookURL, text string) error {
	return sendSlackMessageRaw(webhookURL, slackStyle.escape(text))
}

func sendSlackMessageRaw(webhookURL, text string) error {
	s := newSlackEventSink(nil, webhookURL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), slackSendTimeout)
	defer cancel()
	return s.sendSync(ctx, text)
}

func (s *slackEventSink) EpicStarted(epicName string, done, total int) {
	s.EventSink.EpicStarted(epicName, done, total)
	s.send(slackStyle.epicStartedText(epicName, done, total), notifyKindEpicStarted)
}

func (s *slackEventSink) IterationFinished(ticket tickets.Ticket, epicName string, stats IterationStats) {
	s.EventSink.IterationFinished(ticket, epicName, stats)
	s.send(slackStyle.iterationFinishedText(ticket, epicName, stats), notifyKindIterationFinished)
}

func (s *slackEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.EventSink.IterationPaused(identifier, label, kind, reason)
	s.send(slackStyle.iterationPausedText(label, kind, reason), notifyKindIterationPaused)
}

func (s *slackEventSink) TicketNeedsHuman(identifier, epicName, status, reason string) {
	s.EventSink.TicketNeedsHuman(identifier, epicName, status, reason)
	s.send(slackStyle.ticketNeedsHumanText(identifier, epicName, status, reason), notifyKindTicketNeedsHuman)
}

func (s *slackEventSink) EpicParked(epicName string, stalled []StalledTicket) {
	s.EventSink.EpicParked(epicName, stalled)
	identifiers := make([]string, len(stalled))
	for i, t := range stalled {
		identifiers[i] = t.Identifier
	}
	s.send(slackStyle.epicParkedText(epicName, identifiers), notifyKindEpicParked)
}

func (s *slackEventSink) EpicComplete(epicName string, completed int, elapsedSeconds int) {
	s.EventSink.EpicComplete(epicName, completed, elapsedSeconds)
	s.send(slackStyle.epicCompleteText(epicName, completed, elapsedSeconds), notifyKindEpicComplete)
}

// send POSTs {"text": text} to the Slack workflow webhook in its own
// goroutine, so a slow or unreachable webhook never blocks the caller. Any
// failure is logged via gx's logger and otherwise swallowed — it never
// surfaces to the caller — but every attempt, success or failure, is also
// durably recorded to run-log.jsonl tagged with notifyKind (the live event
// that triggered it), since logger.Debug alone leaves no trace an operator
// checking a run would see. See sendSync for the synchronous, error-returning
// core.
func (s *slackEventSink) send(text, notifyKind string) {
	sendNotification(s.scratchDir, s.epicName, "slack", notifyKind, slackSendTimeout, func(ctx context.Context) error {
		return s.sendSync(ctx, text)
	})
}

// sendSync POSTs {"text": text} to the Slack workflow webhook and waits for
// the response, returning any failure (marshal error, network error, non-2xx
// response) instead of swallowing it.
func (s *slackEventSink) sendSync(ctx context.Context, text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(payload))
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
