package ralphloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elentok/gx/chatmarkup"
)

// slackSendTimeout bounds a single send attempt (both http.Client.Timeout
// and the request context.WithTimeout deadline). sendNotification retries
// once on failure, so the worst-case total is roughly this timeout, plus a
// fixed backoff, plus a second attempt at this timeout — kept short so that
// worst case stays close to a single 10s attempt rather than doubling it.
const slackSendTimeout = 5 * time.Second

// slackTransport is the chatTransport that posts to a Slack "Workflow"
// webhook, which takes a single {"text": "..."} body.
type slackTransport struct {
	webhookURL string
	httpClient *http.Client
}

func newSlackTransport(webhookURL string) *slackTransport {
	return &slackTransport{webhookURL: webhookURL, httpClient: &http.Client{Timeout: slackSendTimeout}}
}

func (t *slackTransport) name() string           { return "slack" }
func (t *slackTransport) timeout() time.Duration { return slackSendTimeout }

func (t *slackTransport) sendSync(ctx context.Context, text chatmarkup.Text) error {
	payload, err := json.Marshal(map[string]string{"text": text.String()})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send failed with status %d", resp.StatusCode)
	}
	return nil
}

// NewSlackEventSink returns an EventSink that wraps inner and additionally
// posts a Slack message to webhookURL for every chat-member event (see
// chatEventSink). Sends run in their own goroutine and never return an error
// or block the caller: a failed send is logged via gx's logger and, tagged
// with the triggering event, appended to scratchDir/epicName's run-log.jsonl.
func NewSlackEventSink(inner EventSink, webhookURL, scratchDir, epicName string) EventSink {
	return newSlackEventSink(inner, webhookURL, scratchDir, epicName)
}

func newSlackEventSink(inner EventSink, webhookURL, scratchDir, epicName string) *chatEventSink {
	s := newChatEventSink(inner, slackStyle, newSlackTransport(webhookURL), scratchDir, epicName)
	s.startFlushLoop(batchFlushInterval)
	return s
}

// SendSlackTestMessage synchronously sends a fixed test notification to the
// Slack webhook and returns any error instead of swallowing it, for callers
// like `gx config test-notifications` that need to report success/failure
// directly — unlike the EventSink decorator's fire-and-forget send.
func SendSlackTestMessage(webhookURL string) error {
	return sendSlackMessageRaw(webhookURL, slackStyle.testMessageText())
}

// SendSlackTestBatch is Slack's counterpart to SendTelegramTestBatch: sends
// two fixed test messages through the same renderBatch join a real flush
// uses. Slack's mrkdwn dialect doesn't treat "-" as reserved, so this is
// expected to succeed even while the Telegram counterpart 400s — included for
// symmetry/smoke-testing the batch plumbing, not because Slack has the bug.
func SendSlackTestBatch(webhookURL string) error {
	items := []batchedMessage{
		{text: slackStyle.testMessageText(), kind: "test"},
		{text: slackStyle.testMessageText(), kind: "test"},
	}
	return sendSlackMessageRaw(webhookURL, renderBatch(slackStyle, items))
}

// SendSlackMessage synchronously posts arbitrary text to the Slack webhook
// and returns any error instead of swallowing it, for callers like `gx
// notify` that need to report success/failure directly — unlike the
// EventSink decorator's fire-and-forget send. text is escaped for Slack's
// mrkdwn dialect here, so callers pass plain text.
func SendSlackMessage(webhookURL, text string) error {
	return sendSlackMessageRaw(webhookURL, slackStyle.chatStyle.Escape(text))
}

func sendSlackMessageRaw(webhookURL string, text chatmarkup.Text) error {
	t := newSlackTransport(webhookURL)
	ctx, cancel := context.WithTimeout(context.Background(), slackSendTimeout)
	defer cancel()
	return t.sendSync(ctx, text)
}
