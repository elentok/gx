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
	ref := s.escape(fmt.Sprintf("%s/%s", epicName, ticket.Identifier))
	line := RenderCountsLine(EpicCounts{Done: stats.Completed, InProgress: stats.InProgress, Total: stats.Total})
	return fmt.Sprintf(
		"✅ *%s*\n\n%s · %s · %s\n%s %s",
		s.escape(ticket.Title), formatDuration(stats.ElapsedSeconds), formatTokens(stats.PeakContextTokens),
		line, s.gxPrefix, ref,
	)
}

// iterationPausedText renders the "paused" notification. It leads with 🛑
// for a needs-repair pause (an operator must act) and ⏸ for anything
// else (rate-limit today; it clears on its own):
//
//	🛑 *{label} paused*
//
//	{reason}
func (s mrkdwnStyle) iterationPausedText(label string, kind PauseKind, reason string) string {
	emoji := "⏸"
	if kind == PauseNeedsRepair {
		emoji = "\U0001f6d1"
	}
	return fmt.Sprintf("%s *%s paused*\n\n%s", emoji, s.escape(label), s.escape(reason))
}

// ticketNeedsHumanText renders the "a machine parked this ticket for a
// person" notification, for either status: unlike iterationPausedText's
// "still in progress, will resume/clear on its own", this means the
// iteration is stuck and won't proceed without a human looking at it.
// needs-answer means no commit landed and the agent never declared the
// zero-commit finish intentional via `gx tickets set --iteration-status
// finished --commitless true`; needs-repair means a fault (operator
// intervention, an iteration error, a dead-on-arrival reconciliation)
// parked it instead.
//
//	🆘 *{epic}/{ticket} needs answer*      (needs-answer)
//	🛑 *{epic}/{ticket} needs repair*      (needs-repair)
//
//	{reason}
//	{counts line}
func (s mrkdwnStyle) ticketNeedsHumanText(identifier, epicName, status, reason string, counts EpicCounts) string {
	ref := s.escape(fmt.Sprintf("%s/%s", epicName, identifier))
	emoji, label := "\U0001f198", "needs answer"
	if status != "needs-answer" {
		emoji, label = "\U0001f6d1", "needs repair"
	}
	return fmt.Sprintf("%s *%s %s*\n\n%s\n%s", emoji, ref, label, s.escape(reason), RenderCountsLine(counts))
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
//	🚀 *epic started: {epicName}*
//
//	{counts line}
func (s mrkdwnStyle) epicStartedText(epicName string, counts EpicCounts) string {
	return fmt.Sprintf(
		"\U0001f680 *epic started: %s*\n\n%s",
		s.escape(epicName), RenderCountsLine(counts),
	)
}

// epicCompleteText renders the "epic complete" notification. counts is the
// full queue counts line, same as epicStartedText; completed is this run's
// own landed-ticket tally, a distinct number from counts.Done (the epic's
// total done count, which may include tickets a prior run landed):
//
//	🎉 *epic complete: {epicName}*
//
//	{counts line}
//	{completed} ticket(s) landed in {elapsed}
func (s mrkdwnStyle) epicCompleteText(epicName string, counts EpicCounts, completed int, elapsedSeconds int) string {
	return fmt.Sprintf(
		"\U0001f389 *epic complete: %s*\n\n%s\n%d ticket(s) landed in %s",
		s.escape(epicName), RenderCountsLine(counts), completed, formatDuration(elapsedSeconds),
	)
}

// epicParkedText renders the "epic parked" notification — the run is still
// alive (unlike epicCompleteText) with every pane recoverable, waiting on a
// person to clear one of the named tickets:
//
//	🅿️ *epic parked: {epicName}*
//
//	Nothing runnable left; waiting on {stalled}
func (s mrkdwnStyle) epicParkedText(epicName string, stalled []string) string {
	return fmt.Sprintf(
		"\U0001f17f️ *epic parked: %s*\n\n%s",
		s.escape(epicName), s.escape("Nothing runnable left; waiting on "+strings.Join(stalled, ", ")),
	)
}

// testMessageText renders the fixed message `gx config test-notifications`
// sends to confirm a configured service is actually reachable:
//
//	[gx] 🔔 *test notification*
//
//	If you can see this, notifications are working.
func (s mrkdwnStyle) testMessageText() string {
	body := s.escape("If you can see this, notifications are working.")
	return fmt.Sprintf("%s \U0001f514 *test notification*\n\n%s", s.gxPrefix, body)
}
