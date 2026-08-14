package chatmarkup

import "strings"

// Telegram is the MarkdownV2 dialect: bold `*x*`, italic `__x__`, and a long
// list of ASCII punctuation (including the hyphens epic/ticket names
// commonly contain) must be backslash-escaped or the API rejects the
// message outright.
var Telegram = Style{
	escape:      escapeTelegramMarkdownV2,
	unescape:    unescapeTelegramMarkdownV2,
	boldDelim:   "*",
	italicDelim: "__",
	escapeChar:  '\\',
}

// Slack is the mrkdwn dialect: bold `*x*`, italic `_x_`. Only `&`/`<`/`>`
// are escaped (as HTML entities) — `*`/`_` pass through unescaped, so a
// title containing either renders with unintended emphasis on Slack. That's
// cosmetic (Slack's webhook doesn't reject on it) and accepted, not fixed
// here — see the epic root ticket's non-goals.
var Slack = Style{
	escape:      escapeSlackMrkdwn,
	unescape:    unescapeSlackMrkdwn,
	boldDelim:   "*",
	italicDelim: "_",
	escapeChar:  0,
}

// telegramMarkdownV2SpecialChars are the ASCII punctuation characters
// Telegram's MarkdownV2 parser treats as syntax; every occurrence outside a
// deliberate emphasis marker (Bold/Italic's own delimiters) must be
// backslash-escaped or the API rejects the message.
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

// unescapeTelegramMarkdownV2 reverses escapeTelegramMarkdownV2: every
// backslash in escaped text was inserted immediately before the one
// original character it protects (including a literal backslash itself),
// so dropping each backslash and keeping the rune that follows it is exact.
func unescapeTelegramMarkdownV2(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) {
			b.WriteRune(runes[i+1])
			i++
			continue
		}
		b.WriteRune(runes[i])
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

// unescapeSlackMrkdwn reverses escapeSlackMrkdwn. &lt;/&gt; are decoded
// before &amp; so a decoded "&" doesn't accidentally start a new entity.
func unescapeSlackMrkdwn(s string) string {
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}
