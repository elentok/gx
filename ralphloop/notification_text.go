package ralphloop

import (
	"fmt"
	"math"
	"strings"

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

// mrkdwnStyle adapts the shared message templates below to a specific
// chat platform's markup dialect: Telegram's MarkdownV2 requires
// backslash-escaping a long list of ASCII punctuation (including the
// hyphens epic/ticket names commonly contain), while Slack's mrkdwn needs
// none of that. gxPrefix carries the platform-appropriate spelling of the
// literal "[gx]" tag, since Telegram's escaper would otherwise need to
// escape the brackets there too.
type mrkdwnStyle struct {
	escape   func(string) string
	gxPrefix string
}

var telegramStyle = mrkdwnStyle{escape: escapeTelegramMarkdownV2, gxPrefix: `\[gx\]`}

var slackStyle = mrkdwnStyle{escape: escapeSlackMrkdwn, gxPrefix: "[gx]"}

// telegramMarkdownV2SpecialChars are the ASCII punctuation characters
// Telegram's MarkdownV2 parser treats as syntax; every occurrence outside
// deliberate formatting markers (the literal "*" this package wraps titles
// in) must be backslash-escaped or the API rejects the message.
const telegramMarkdownV2SpecialChars = "_*[]()~`>#+-=|{}.!\\"

func escapeTelegramMarkdownV2(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(telegramMarkdownV2SpecialChars, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeSlackMrkdwn escapes the three characters Slack's mrkdwn requires
// escaped as HTML entities; everything else (including "*" and "-") is
// passed through unchanged.
func escapeSlackMrkdwn(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// message is the one shared constructor every chat-member event's text goes
// through (see chatEventSink): an emoji and bold headline (escaped here, so
// callers pass it raw), a blank line, an optional counts line, an optional
// detail line, and identity last. counts/detail/identity arrive pre-escaped
// — callers control exactly what those carry, in particular keeping the
// iteration label (permitted in detail) out of identity, which names only
// the epic or epic-and-ticket a person needs to attribute the message to at
// a glance.
func (s mrkdwnStyle) message(emoji, headline, counts, detail, identity string) string {
	lines := []string{fmt.Sprintf("%s *%s*", emoji, s.escape(headline)), ""}
	if counts != "" {
		lines = append(lines, counts)
	}
	if detail != "" {
		lines = append(lines, detail)
	}
	lines = append(lines, identity)
	return strings.Join(lines, "\n")
}

// identityLine renders the trailing "who this is about" line: the gx tag
// plus the epic name alone, or the epic and a ticket identifier — never an
// iteration label, so parallel epics stay attributable at a glance.
func (s mrkdwnStyle) identityLine(epicName, ticketIdentifier string) string {
	ref := epicName
	if ticketIdentifier != "" {
		ref = fmt.Sprintf("%s/%s", epicName, ticketIdentifier)
	}
	return fmt.Sprintf("%s %s", s.gxPrefix, s.escape(ref))
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
func (s mrkdwnStyle) epicStartedText(epicName string, counts EpicCounts) string {
	return s.message("\U0001f680", "epic started", RenderCountsLine(counts), "", s.identityLine(epicName, ""))
}

// iterationStartedText renders the "iteration started" notification:
//
//	▶️ *{title}*
//
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) iterationStartedText(ticket tickets.Ticket, epicName string) string {
	return s.message("▶️", ticket.Title, "", "", s.identityLine(epicName, ticket.Identifier))
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
func (s mrkdwnStyle) iterationPausedText(label, reason, epicName, ticketIdentifier string) string {
	detail := s.escape(fmt.Sprintf("%s: %s", label, reason))
	return s.message("⏸", "paused", "", detail, s.identityLine(epicName, ticketIdentifier))
}

// iterationResumedText renders the "resumed" notification, the counterpart
// to iterationPausedText — also only reached for a non-park pause kind:
//
//	▶️ *resumed*
//
//	{label}
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) iterationResumedText(label, epicName, ticketIdentifier string) string {
	return s.message("▶️", "resumed", "", s.escape(label), s.identityLine(epicName, ticketIdentifier))
}

// iterationFinishedText renders the "done" notification:
//
//	✅ *{title}*
//
//	{elapsed} · {tokens} · {counts line}
//	[gx] {epic}/{ticket}
//
// The counts line here carries only done/in-progress/total (see
// EpicCounts.ParkedIdentifiers, Blocked, Ready, which IterationStats never
// populates): a ticket-landed message reports what this landing changed,
// not the epic's full parked/blocked/ready breakdown, which belongs to the
// epic-level messages alone (see epicStartedText/epicCompleteText).
func (s mrkdwnStyle) iterationFinishedText(ticket tickets.Ticket, epicName string, stats IterationStats) string {
	line := RenderCountsLine(EpicCounts{Done: stats.Completed, InProgress: stats.InProgress, Total: stats.Total})
	counts := fmt.Sprintf(
		"%s · %s · %s",
		formatDuration(stats.ElapsedSeconds), formatTokens(stats.PeakContextTokens), line,
	)
	return s.message("✅", ticket.Title, counts, "", s.identityLine(epicName, ticket.Identifier))
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
func (s mrkdwnStyle) ticketNeedsHumanText(identifier, epicName, status, reason string, counts EpicCounts) string {
	emoji, headline := "\U0001f198", "needs answer"
	if status != "needs-answer" {
		emoji, headline = "\U0001f6d1", "needs repair"
	}
	detail := fmt.Sprintf("%s\n%s", s.escape(reason), RenderCountsLine(counts))
	return s.message(emoji, headline, "", detail, s.identityLine(epicName, identifier))
}

// epicParkedText renders the "epic parked" notification — the run is still
// alive (unlike epicCompleteText) with every pane recoverable, waiting on a
// person to clear one of the named tickets:
//
//	🅿️ *epic parked*
//
//	Nothing runnable left; waiting on {stalled}
//	[gx] {epic}
func (s mrkdwnStyle) epicParkedText(epicName string, stalled []string) string {
	detail := s.escape("Nothing runnable left; waiting on " + strings.Join(stalled, ", "))
	return s.message("\U0001f17f️", "epic parked", "", detail, s.identityLine(epicName, ""))
}

// epicCompleteText renders the "epic complete" notification. counts is the
// full queue counts line, same as epicStartedText; completed is this run's
// own landed-ticket tally, a distinct number from counts.Done (the epic's
// total done count, which may include tickets a prior run landed):
//
//	🎉 *epic complete*
//
//	{counts line}
//	{completed} ticket(s) landed in {elapsed}
//	[gx] {epic}
func (s mrkdwnStyle) epicCompleteText(epicName string, counts EpicCounts, completed int, elapsedSeconds int) string {
	detail := fmt.Sprintf("%d ticket(s) landed in %s", completed, formatDuration(elapsedSeconds))
	return s.message("\U0001f389", "epic complete", RenderCountsLine(counts), detail, s.identityLine(epicName, ""))
}

// testMessageText renders the fixed message `gx config test-notifications`
// sends to confirm a configured service is actually reachable:
//
//	🔔 *test notification*
//
//	If you can see this, notifications are working.
//	[gx]
func (s mrkdwnStyle) testMessageText() string {
	body := s.escape("If you can see this, notifications are working.")
	return s.message("\U0001f514", "test notification", "", body, s.gxPrefix)
}
