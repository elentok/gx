// Package chatmarkup produces safe-to-send chat markup for gx's outbound
// notifications. It has no dependency on ralphloop's own types — it only
// ever deals in raw strings in, sealed Text out — so a value can only
// become "safe" by passing through one of this package's constructors,
// never by convention at the call site.
package chatmarkup

import (
	"fmt"
	"strings"
)

// Text is a platform-safe, already-escaped fragment of chat markup. Its
// field is unexported so a value can only be constructed by calling into
// this package — an outside caller cannot build one directly, even by
// accident, unlike a same-package named type or struct literal would allow.
type Text struct {
	s string
}

// String returns the underlying escaped text, for handing to an HTTP
// client. Reading out is unrestricted — only construction is sealed.
func (t Text) String() string {
	return t.s
}

// WithSuffix appends raw, escaped-on-the-way-in text to t — the batch ×N
// dedup suffix's escaped path, replacing the raw `" ×%d"` literal callers
// used to append directly.
func (t Text) WithSuffix(raw string, style Style) Text {
	return Text{s: t.s + style.escape(raw)}
}

// Style carries one platform dialect's escape rules and emphasis syntax —
// see Telegram and Slack.
type Style struct {
	escape      func(string) string
	unescape    func(string) string
	boldDelim   string
	italicDelim string
	// escapeChar is the dialect's escape-prefix rune (0 if the dialect has
	// no backslash-escape scheme at all, i.e. Slack): Plain uses it to tell
	// a structural emphasis marker apart from an escaped literal character
	// that happens to match the marker's delimiter.
	escapeChar rune
}

// Escape is the sole general-purpose constructor: raw text in, Text
// carrying the dialect-escaped equivalent out.
func (s Style) Escape(raw string) Text {
	return Text{s: s.escape(raw)}
}

// Bold wraps raw text in this dialect's bold emphasis syntax, escaping the
// content first.
func (s Style) Bold(raw string) Text {
	return Text{s: s.boldDelim + s.escape(raw) + s.boldDelim}
}

// Italic wraps raw text in this dialect's italic emphasis syntax, escaping
// the content first.
func (s Style) Italic(raw string) Text {
	return Text{s: s.italicDelim + s.escape(raw) + s.italicDelim}
}

// Message assembles the shared notification layout — emoji, bold headline,
// a blank line, an optional counts line, an optional detail line, and
// identity last — from raw, unescaped fragments, escaping every one of them
// internally. This replaces the old pattern where callers were expected to
// pre-escape counts/detail/identity themselves.
func (s Style) Message(emoji, headline, counts, detail, identity string) Text {
	lines := []string{fmt.Sprintf("%s %s", emoji, s.Bold(headline).s), ""}
	if counts != "" {
		lines = append(lines, s.escape(counts))
	}
	if detail != "" {
		lines = append(lines, s.escape(detail))
	}
	lines = append(lines, s.escape(identity))
	return Text{s: strings.Join(lines, "\n")}
}

// Plain renders t back to unmarked, unescaped text for display contexts
// that don't understand the dialect's markup (e.g. TUI): emphasis markers
// are stripped first, then backslash-escapes are resolved. That order
// matters — an escaped literal delimiter (e.g. Telegram's `\*` for a
// literal asterisk) must survive stripping untouched so it isn't mistaken
// for a marker once unescaped.
func (s Style) Plain(t Text) string {
	x := stripMarker(t.s, s.boldDelim, s.escapeChar)
	x = stripMarker(x, s.italicDelim, s.escapeChar)
	return s.unescape(x)
}

// Join joins already-safe Text values with an already-safe separator,
// producing one safe Text. Both sep and every item must already have come
// through Escape (or another constructor) — Join itself does no escaping,
// which is what forces a batch separator literal through Escape before it
// can reach a joined message.
func Join(sep Text, items []Text) Text {
	strs := make([]string, len(items))
	for i, item := range items {
		strs[i] = item.s
	}
	return Text{s: strings.Join(strs, sep.s)}
}

// stripMarker removes bare (unescaped) occurrences of delim from s. When
// escapeChar is non-zero, an occurrence of escapeChar is treated as
// protecting the rune that follows it — that pair is copied through
// untouched rather than matched against delim — so an escaped literal
// delimiter character is left for unescape to resolve rather than being
// mistaken for a structural marker.
func stripMarker(s, delim string, escapeChar rune) string {
	if delim == "" {
		return s
	}
	runes := []rune(s)
	delimRunes := []rune(delim)
	n, dn := len(runes), len(delimRunes)
	var b strings.Builder
	for i := 0; i < n; {
		if escapeChar != 0 && runes[i] == escapeChar && i+1 < n {
			b.WriteRune(runes[i])
			b.WriteRune(runes[i+1])
			i += 2
			continue
		}
		if i+dn <= n && string(runes[i:i+dn]) == delim {
			i += dn
			continue
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}
