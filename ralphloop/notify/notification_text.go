package notify

import (
	"fmt"
	"math"

	"github.com/elentok/gx/chatmarkup"
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

// testMessageText renders the fixed message `gx config test-notifications`
// sends to confirm a configured service is actually reachable.
func (s mrkdwnStyle) testMessageText() chatmarkup.Text {
	return testMessage().Text(s.chatStyle)
}
