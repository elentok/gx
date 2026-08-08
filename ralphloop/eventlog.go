package ralphloop

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/logger"
)

// Event type strings recorded in an epic's run-log.jsonl.
const (
	eventIterationStarted        = "iteration-started"
	eventIterationFinished       = "iteration-finished"
	eventCherryPicked            = "cherry-picked"
	eventConflictHit             = "conflict-hit"
	eventConflictResolved        = "conflict-resolved"
	eventPausedSmartZone         = "paused-smart-zone"
	eventSmartZoneRecoveryFailed = "smart-zone-recovery-failed"
	// eventSmartZoneWaitExpired marks a compact-recovery wait that expired
	// past smartZoneCompactTimeoutMs without herdr's pane-status wait ever
	// confirming completion, but where the transcript's compaction-boundary
	// signal showed compaction actually finished anyway — a slower-than-usual
	// compact, not a failure, and deliberately distinct from
	// eventSmartZoneRecoveryFailed so it isn't misread as one.
	eventSmartZoneWaitExpired = "smart-zone-wait-expired"
	eventPausedRateLimit      = "paused-rate-limit"
	eventResumed              = "resumed"
	eventNeedsInfo            = "needs-info"
	eventCommitless           = "commitless"
	eventNeedsAttention       = "needs-attention"
	eventDepsInstalled        = "deps-installed"
	// eventSchedulerScan marks one claimNext pass: every ticket the epic
	// currently has, and why the scheduler did or didn't claim it. Added to
	// debug tickets that appear queued (e.g. a code-review ticket's freshly
	// published children) but never get picked up — the usual cause is a
	// ticket falling outside the run's RunScope (see ScanDecision's
	// "out-of-scope"), which the ticket-level events above never surface
	// since they only ever fire for a ticket the scheduler already claimed.
	eventSchedulerScan = "scheduler-scan"

	eventNotificationsConfigured = "notifications-configured"
	eventNotificationSent        = "notification-sent"
	eventNotificationFailed      = "notification-failed"
)

// notifyKind* tag which live event triggered a notification-sent/
// notification-failed line — distinct from the Type field (which is always
// "notification-sent"/"notification-failed" itself).
const (
	notifyKindIterationFinished = "iteration-finished"
	notifyKindIterationPaused   = "iteration-paused"
	notifyKindEpicComplete      = "epic-complete"
)

// ScanDecision records one ticket's scheduling disposition for a single
// scheduler-scan log line, in Epic.Tickets order — every ticket
// loadNamedEpic returned at the moment claimNext evaluated the frontier, not
// just the ones in scope or in the frontier, so a ticket that's silently
// out-of-scope (the most common cause of "queued but never starts") still
// shows up.
type ScanDecision struct {
	// Ticket is the ticket's Identifier (see Event.Ticket).
	Ticket string `json:"ticket"`
	// Status is the ticket's RenderedStatus (see tickets.RenderedStatus.Word).
	Status string `json:"status"`
	// Decision is one of: "claimed" (this pass claimed it), "frontier"
	// (open/unblocked/in-scope but a different ticket was claimed instead,
	// or maxParallel was already reached elsewhere in this Run call),
	// "out-of-scope" (RunScope.Contains is false — e.g. a code-review
	// ticket's child missing its own parent: back-edge), "blocked", or
	// "settled" (already done/needs-info/needs-attention).
	Decision string `json:"decision"`
	// Reason elaborates Decision — e.g. UnresolvedBlockers's tokens for
	// "blocked". Empty when Decision is self-explanatory.
	Reason string `json:"reason,omitempty"`
}

// Event is one line of an epic's run-log.jsonl: a single lifecycle
// occurrence, timestamped and attributed to the ticket/pane/tab it happened
// on. AgentSession is empty for events not tied to a specific agent session
// (e.g. conflict-hit, which precedes the conflict-resolution agent's own
// launch). Agent is omitted by historical logs and defaults to Claude when
// reports read them. Reason carries the reason for pause and attention
// events, or the install command run (empty if none matched) for
// deps-installed events.
type Event struct {
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	// Ticket is the ticket's Identifier (the filename's full "NN[letter]"
	// prefix), not its Number, so lettered split siblings sharing a Number
	// (e.g. "04a"/"04b") remain distinguishable in the run log.
	Ticket       string    `json:"ticket"`
	Agent        AgentKind `json:"agent,omitempty"`
	Pane         string    `json:"pane,omitempty"`
	Tab          string    `json:"tab,omitempty"`
	AgentSession string    `json:"agent_session,omitempty"`
	// Cwd is the directory the agent session was launched in and is the local
	// session-data lookup key alongside AgentSession.
	Cwd    string `json:"cwd,omitempty"`
	Reason string `json:"reason,omitempty"`
	// SHA is the feature branch's tip commit right after a cherry-picked
	// event landed a ticket's iteration. Recorded because CherryPickRange
	// creates fresh commits (different hashes than the iteration branch's
	// originals), so it's the only durable record startup reconciliation can
	// check for reachability from the feature branch's later tip — the
	// iteration branch itself isn't guaranteed to still exist by then.
	SHA string `json:"sha,omitempty"`
	// Scan (scheduler-scan events only) is every ticket's disposition for
	// this claimNext pass — see ScanDecision.
	Scan []ScanDecision `json:"scan,omitempty"`
	// Telegram/Slack (notifications-configured only) record whether each
	// channel is active for this run — never the token/webhook values
	// themselves. Pointers (rather than plain bool) so "false" still
	// serializes explicitly instead of vanishing under omitempty.
	Telegram *bool `json:"telegram,omitempty"`
	Slack    *bool `json:"slack,omitempty"`
	// Channel (notification-sent/notification-failed only) is which channel
	// the attempt used: "telegram" or "slack".
	Channel string `json:"channel,omitempty"`
	// NotifyKind (notification-sent/notification-failed only) is the live
	// event that triggered the send attempt — see notifyKind* consts.
	NotifyKind string `json:"notify_kind,omitempty"`
}

// eventLogMu serializes appends across every goroutine in the process (each
// running iteration logs concurrently), so two events never interleave
// their JSON onto the same line.
var eventLogMu sync.Mutex

// runLogPath returns the append-only event log path for epicName under
// scratchDir.
func runLogPath(scratchDir, epicName string) string {
	return filepath.Join(scratchDir, epicName, "run-log.jsonl")
}

// logEvent appends ev as one JSON line to epicName's run-log.jsonl under
// scratchDir, creating the epic directory if it doesn't exist yet, and
// filling Time in if it's zero. It's a no-op if scratchDir or epicName is
// empty, so call sites that don't have logging wired up (e.g. the
// conflict-resolution launch, which logs conflict-hit/resolved manually
// instead of via the generic start/finish path) don't need to special-case
// it.
func logEvent(scratchDir, epicName string, ev Event) error {
	if scratchDir == "" || epicName == "" {
		return nil
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := runLogPath(scratchDir, epicName)

	eventLogMu.Lock()
	defer eventLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// LogNotificationsConfigured records, once per epic run, which notification
// channels are active — never the underlying bot token/webhook URL — so an
// operator reading run-log.jsonl can tell a channel that was simply never
// configured apart from one whose sends are silently failing (see
// logNotificationSent/logNotificationFailed).
func LogNotificationsConfigured(scratchDir, epicName string, telegram, slack bool) error {
	return logEvent(scratchDir, epicName, Event{
		Type:     eventNotificationsConfigured,
		Telegram: &telegram,
		Slack:    &slack,
	})
}

// logNotificationSent/logNotificationFailed record one Telegram/Slack
// delivery attempt from inside telegramEventSink/slackEventSink's send,
// tagged with the channel and the live event that triggered it. Errors are
// swallowed like every other logEvent call site here — a failure to log
// shouldn't compound the failure it was trying to record.
func logNotificationSent(scratchDir, epicName, channel, notifyKind string) {
	_ = logEvent(scratchDir, epicName, Event{
		Type: eventNotificationSent, Channel: channel, NotifyKind: notifyKind,
	})
}

func logNotificationFailed(scratchDir, epicName, channel, notifyKind, reason string) {
	_ = logEvent(scratchDir, epicName, Event{
		Type: eventNotificationFailed, Channel: channel, NotifyKind: notifyKind, Reason: reason,
	})
}

// sendNotification runs sendSync in its own goroutine, bounded by timeout, so
// a slow or unreachable notification endpoint never blocks the caller — the
// shared goroutine + log-outcome wrapper behind slackEventSink.send and
// telegramEventSink.send, which were identical except for channel and the
// sendSync call itself. Any failure is logged via gx's logger and otherwise
// swallowed, but every attempt, success or failure, is also durably recorded
// to run-log.jsonl via logNotificationSent/logNotificationFailed, tagged with
// channel and notifyKind (the live event that triggered it).
func sendNotification(scratchDir, epicName, channel, notifyKind string, timeout time.Duration, sendSync func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := sendSync(ctx); err != nil {
			err = sanitizeSendError(err)
			logger.Debug("%s: %v\n", channel, err)
			logNotificationFailed(scratchDir, epicName, channel, notifyKind, err.Error())
			return
		}
		logNotificationSent(scratchDir, epicName, channel, notifyKind)
	}()
}

// sanitizeSendError strips the request URL from a failed send's error before
// it reaches logger.Debug or run-log.jsonl. telegramEventSink/slackEventSink
// build that URL with the bot token/webhook secret embedded in the path, and
// (*url.Error).Error() — what http.Client.Do returns on failure — includes
// the full URL verbatim, so logging it as-is leaks the secret. The
// underlying cause (timeout, connection refused, DNS failure, etc.) is kept
// so the diagnostic value survives; only the URL is dropped.
func sanitizeSendError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// lastIterationSession finds the most recent iteration-started event for
// identifier (a ticket's Identifier, not Number, so lettered split siblings
// aren't cross-attributed) with a recorded AgentSession — the session
// id/cwd/agent to backfill a reattached ticket close's metadata with (ticket
// 06a), since the reattaching run never captured a fresh session of its own.
// Agent defaults to AgentClaude for historical logs that omitted it (see
// Event.Agent).
func lastIterationSession(events []Event, identifier string) (agentSession, cwd string, agent AgentKind, ok bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Ticket != identifier || ev.Type != eventIterationStarted || ev.AgentSession == "" {
			continue
		}
		agent = ev.Agent
		if agent == "" {
			agent = AgentClaude
		}
		return ev.AgentSession, ev.Cwd, agent, true
	}
	return "", "", "", false
}

// readEvents reads and parses every line of epicName's run-log.jsonl under
// scratchDir, skipping malformed lines rather than failing the whole read
// (a run-log written by a process killed mid-write may have a torn final
// line). ok is false if the log doesn't exist yet.
func readEvents(scratchDir, epicName string) (events []Event, ok bool, err error) {
	raw, err := os.ReadFile(runLogPath(scratchDir, epicName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, true, nil
}
