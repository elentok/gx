package ralphloop

import (
	"fmt"
	"math"
	"strings"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/tickets"
)

// formatDuration renders totalSeconds as "5m32s"/"1h5m32s"/"42s", the
// compact form notification messages use in place of a raw second count.
func formatDuration(totalSeconds int) string {
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds", totalSeconds)
	}
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// formatTokens renders n as "39k tok", rounding to the nearest thousand once
// n reaches 1000; below that it renders the exact count.
func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d tok", n)
	}
	k := int(math.Round(float64(n) / 1000))
	return fmt.Sprintf("%dk tok", k)
}

// mrkdwnStyle adapts the shared message templates below to a specific chat
// platform's markup dialect via chatStyle (chatmarkup's own dialect value,
// which every *Text function and renderBatch's batch separator routes its
// raw fragments through — see chat_eventsink.go). gxPrefix is the raw
// literal "[gx]" tag for both platforms; it's no longer pre-escaped, since it
// now flows through the same chatmarkup.Style.Message boundary escape as
// every other fragment.
type mrkdwnStyle struct {
	chatStyle chatmarkup.Style
	gxPrefix  string
}

var telegramStyle = mrkdwnStyle{chatStyle: chatmarkup.Telegram, gxPrefix: "[gx]"}

var slackStyle = mrkdwnStyle{chatStyle: chatmarkup.Slack, gxPrefix: "[gx]"}

// identityLine renders the trailing "who this is about" line: the gx tag
// plus the epic name alone, or the epic and a ticket identifier — never an
// iteration label, so parallel epics stay attributable at a glance. The
// result is raw, unescaped text: every *Text function hands it to
// chatStyle.Message as the identity fragment, which escapes it there.
func (s mrkdwnStyle) identityLine(epicName, ticketIdentifier string) string {
	ref := epicName
	if ticketIdentifier != "" {
		ref = fmt.Sprintf("%s/%s", epicName, ticketIdentifier)
	}
	return fmt.Sprintf("%s %s", s.gxPrefix, ref)
}

// epicStartedText renders the "epic started" notification — the single
// message every epic that leaves the queue emits exactly once, folding what
// used to be separate no-tickets/already-complete notifications: a fresh
// start reads as a plain counts line, while total 0 or done == total tell
// the same story the old separate events used to. counts is the full
// done/in-progress/parked/blocked/ready/total breakdown — the "queue counts
// line" — since an epic-level message is the one place a run-wide picture
// is useful rather than noise (see iterationFinishedText):
//
//	🚀 *epic started*
//
//	{counts line}
//	[gx] {epic}
func (s mrkdwnStyle) epicStartedText(epicName string, counts EpicCounts) chatmarkup.Text {
	return s.chatStyle.Message("\U0001f680", "epic started", RenderCountsLine(counts), "", s.identityLine(epicName, ""))
}

// iterationStartedText renders the "iteration started" notification:
//
//	▶️ *{title}*
//
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) iterationStartedText(ticket tickets.Ticket, epicName string) chatmarkup.Text {
	return s.chatStyle.Message("▶️", ticket.Title, "", "", s.identityLine(epicName, ticket.Identifier))
}

// iterationPausedText renders the "paused" notification. Only reached for a
// non-park pause (PauseRateLimit — see chatEventSink.IterationPaused), which
// clears on its own, so there's a single emoji rather than the
// pause-kind-dependent choice a park's IterationPaused used to need before
// TicketNeedsHuman became the one chat-visible message for every park:
//
//	⏸ *paused*
//
//	{label}: {reason}
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) iterationPausedText(label, reason, epicName, ticketIdentifier string) chatmarkup.Text {
	detail := fmt.Sprintf("%s: %s", label, reason)
	return s.chatStyle.Message("⏸", "paused", "", detail, s.identityLine(epicName, ticketIdentifier))
}

// iterationResumedText renders the "resumed" notification, the counterpart
// to iterationPausedText — also only reached for a non-park pause kind:
//
//	▶️ *resumed*
//
//	{label}
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) iterationResumedText(label, epicName, ticketIdentifier string) chatmarkup.Text {
	return s.chatStyle.Message("▶️", "resumed", "", label, s.identityLine(epicName, ticketIdentifier))
}

// iterationFinishedText renders the "done" notification:
//
//	✅ *{title}*
//
//	{elapsed} · {tokens} · {cost} · {counts line}
//	[gx] {epic}/{ticket}
//
// The counts line here carries only done/in-progress/total (see
// EpicCounts.ParkedIdentifiers, Blocked, Ready, which IterationStats never
// populates): a ticket-landed message reports what this landing changed,
// not the epic's full parked/blocked/ready breakdown, which belongs to the
// epic-level messages alone (see epicStartedText/epicCompleteText).
func (s mrkdwnStyle) iterationFinishedText(ticket tickets.Ticket, epicName string, stats IterationStats) chatmarkup.Text {
	line := RenderCountsLine(EpicCounts{Done: stats.Completed, InProgress: stats.InProgress, Total: stats.Total})
	counts := fmt.Sprintf(
		"%s · %s · %s · %s",
		formatDuration(stats.ElapsedSeconds), formatTokens(stats.PeakContextTokens), tickets.FormatCost(stats.Cost), line,
	)
	return s.chatStyle.Message("✅", ticket.Title, counts, "", s.identityLine(epicName, ticket.Identifier))
}

// ticketNeedsHumanText renders the "a machine parked this ticket for a
// person" notification, for either status — the one chat-visible message
// for every park (see chatEventSink's park-cardinality handling).
// needs-answer means no commit landed and the agent never declared the
// zero-commit finish intentional via `gx tickets set --iteration-status
// finished --commitless true`; needs-repair means a fault (operator
// intervention, an iteration error, a dead-on-arrival reconciliation)
// parked it instead.
//
//	🆘 *needs answer*      (needs-answer)
//	🛑 *needs repair*      (needs-repair)
//
//	{reason}
//	{counts line}
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) ticketNeedsHumanText(identifier, epicName, status, reason string, counts EpicCounts) chatmarkup.Text {
	emoji, headline := "\U0001f198", "needs answer"
	if status != "needs-answer" {
		emoji, headline = "\U0001f6d1", "needs repair"
	}
	detail := fmt.Sprintf("%s\n%s", reason, RenderCountsLine(counts))
	return s.chatStyle.Message(emoji, headline, "", detail, s.identityLine(epicName, identifier))
}

// epicParkedText renders the "epic parked" notification — the run is still
// alive (unlike epicCompleteText) with every pane recoverable, waiting on a
// person to clear one of the named tickets:
//
//	🅿️ *epic parked*
//
//	Nothing runnable left; waiting on {stalled}
//	[gx] {epic}
func (s mrkdwnStyle) epicParkedText(epicName string, stalled []string) chatmarkup.Text {
	detail := "Nothing runnable left; waiting on " + strings.Join(stalled, ", ")
	return s.chatStyle.Message("\U0001f17f️", "epic parked", "", detail, s.identityLine(epicName, ""))
}

// epicCompleteText renders the "epic complete" notification. counts is the
// full queue counts line, same as epicStartedText; completed is this run's
// own landed-ticket tally, a distinct number from counts.Done (the epic's
// total done count, which may include tickets a prior run landed); totalCost
// is the epic-wide sum of every ticket's ActualCost (see loadEpicTotalCost),
// not just this run's own landings:
//
//	🎉 *epic complete*
//
//	{counts line}
//	{completed} ticket(s) landed in {elapsed} · {totalCost}
//	[gx] {epic}
func (s mrkdwnStyle) epicCompleteText(epicName string, counts EpicCounts, completed int, elapsedSeconds int, totalCost float64) chatmarkup.Text {
	detail := fmt.Sprintf("%d ticket(s) landed in %s · %s", completed, formatDuration(elapsedSeconds), tickets.FormatCost(totalCost))
	return s.chatStyle.Message("\U0001f389", "epic complete", RenderCountsLine(counts), detail, s.identityLine(epicName, ""))
}

// epicFailedText renders the "epic failed" notification — the second of the
// two epic-level messages a failed run produces (epicStartedText is the
// first), emitted by EpicFailureReporter after the run's own sink has
// already closed and drained (see epic_failure_reporter.go), so counts is
// loaded fresh rather than carried over from a live event:
//
//	🔥 *epic failed*
//
//	{counts line}
//	{err}
//	[gx] {epic}
func (s mrkdwnStyle) epicFailedText(epicName string, counts EpicCounts, errMsg string) chatmarkup.Text {
	return s.chatStyle.Message("\U0001f525", "epic failed", RenderCountsLine(counts), errMsg, s.identityLine(epicName, ""))
}

// mutedText renders the gate's edge-triggered per-source mute notice — the
// one message sent on the allowed→muted transition for a ticket's storm
// (see chatEventSink.send):
//
//	🔇 *muting this*
//
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) mutedText(epicName, ticketIdentifier string) chatmarkup.Text {
	return s.chatStyle.Message("\U0001f507", "muting this", "", "", s.identityLine(epicName, ticketIdentifier))
}

// globallyMutedText renders the gate's edge-triggered global-mute notice —
// the final message sent on transport before every further send on it is
// suppressed until `gx notify --enable`:
//
//	🚫 *globally muted*
//
//	re-enable with `gx notify --enable {transport}`
//	[gx]
func (s mrkdwnStyle) globallyMutedText(transport string) chatmarkup.Text {
	detail := fmt.Sprintf("re-enable with `gx notify --enable %s`", transport)
	return s.chatStyle.Message("\U0001f6ab", "globally muted", "", detail, s.gxPrefix)
}

// drainCompleteText renders the "drain complete" notification: the run
// ended because it was told to drain (see Gate.Drain), not because the epic
// naturally ran out of tickets — distinct from epicCompleteText so an
// operator away from the terminal can tell the two apart. counts/totalCost
// carry the same semantics as epicCompleteText's own.
//
//	🛑 *epic drained*
//
//	{counts line}
//	{completed} ticket(s) landed in {elapsed} · {totalCost}
//	[gx] {epic}
func (s mrkdwnStyle) drainCompleteText(epicName string, counts EpicCounts, completed int, elapsedSeconds int, totalCost float64) chatmarkup.Text {
	detail := fmt.Sprintf("%d ticket(s) landed in %s · %s", completed, formatDuration(elapsedSeconds), tickets.FormatCost(totalCost))
	return s.chatStyle.Message("\U0001f6d1", "epic drained", RenderCountsLine(counts), detail, s.identityLine(epicName, ""))
}

// testMessageText renders the fixed message `gx config test-notifications`
// sends to confirm a configured service is actually reachable:
//
//	🔔 *test notification*
//
//	If you can see this, notifications are working.
//	[gx]
func (s mrkdwnStyle) testMessageText() chatmarkup.Text {
	body := "If you can see this, notifications are working."
	return s.chatStyle.Message("\U0001f514", "test notification", "", body, s.gxPrefix)
}
