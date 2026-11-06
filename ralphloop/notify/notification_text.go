package notify

import (
	"fmt"
	"math"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
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

// iterationFinishedText renders the "done" notification:
//
//	✅ *{title}*
//
//	{elapsed} · {tokens} · {cost} · {counts line}
//	[gx] {epic}/{ticket}
//
// The counts line here carries only done/in-progress/total (see
// schedule.EpicCounts.PausedIdentifiers, Blocked, Ready, which IterationStats never
// populates): a ticket-merged message reports what this merging changed,
// not the epic's full paused/blocked/ready breakdown, which belongs to the
// epic-level messages alone (see epicOutcomeText).
func (s mrkdwnStyle) iterationFinishedText(ticket tickets.Ticket, epicName string, stats runlog.IterationStats) chatmarkup.Text {
	line := RenderCountsLine(schedule.EpicCounts{Done: stats.Completed, InProgress: stats.InProgress, Total: stats.Total})
	counts := fmt.Sprintf(
		"%s · %s · %s · %s",
		formatDuration(stats.ElapsedSeconds), formatTokens(stats.PeakContextTokens), tickets.FormatCost(stats.Cost), line,
	)
	return s.chatStyle.Message("✅", ticket.Title, counts, "", s.identityLine(epicName, ticket.Identifier))
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
