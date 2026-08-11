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
//	{elapsed} · {tokens} · {done}/{total} done
//	[gx] {epic}/{ticket}
func (s mrkdwnStyle) iterationFinishedText(ticket tickets.Ticket, epicName string, stats IterationStats) string {
	ref := s.escape(fmt.Sprintf("%s/%s", epicName, ticket.Identifier))
	return fmt.Sprintf(
		"✅ *%s*\n\n%s · %s · %d/%d done\n%s %s",
		s.escape(ticket.Title), formatDuration(stats.ElapsedSeconds), formatTokens(stats.PeakContextTokens),
		stats.Completed, stats.Total, s.gxPrefix, ref,
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

// ticketNeedsAnswerText renders the "needs-answer" notification: unlike
// iterationPausedText's "still in progress, will resume/clear on its own",
// this means the iteration is stuck and won't proceed without a human
// looking at it — no commit landed and the agent never declared the
// zero-commit finish intentional via `gx tickets set --commitless true`.
//
//	🆘 *{epic}/{ticket} needs answer*
//
//	No commits landed; marked needs-answer.
func (s mrkdwnStyle) ticketNeedsAnswerText(identifier, epicName string) string {
	ref := s.escape(fmt.Sprintf("%s/%s", epicName, identifier))
	body := s.escape("No commits landed; marked needs-answer.")
	return fmt.Sprintf("\U0001f198 *%s needs answer*\n\n%s", ref, body)
}

// epicCompleteText renders the "epic complete" notification:
//
//	🎉 *epic complete: {epicName}*
//
//	{completed} tickets landed in {elapsed}
func (s mrkdwnStyle) epicCompleteText(epicName string, completed int, elapsedSeconds int) string {
	return fmt.Sprintf(
		"\U0001f389 *epic complete: %s*\n\n%d tickets landed in %s",
		s.escape(epicName), completed, formatDuration(elapsedSeconds),
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
