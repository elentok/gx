package ralphloop

import "strings"

// independentTelegramMarkdownV2SpecialChars is a second, hand-typed list of
// the ASCII punctuation Telegram's MarkdownV2 parser treats as syntax — kept
// deliberately separate from chatmarkup's telegramMarkdownV2SpecialChars so
// this checker is a genuine second opinion: if production's list ever went
// missing a character or had one wrong, a checker built from the same list
// would silently agree with the same wrong answer instead of catching it.
const independentTelegramMarkdownV2SpecialChars = "\\_*[]()~`>#+-=|{}.!"

// isValidTelegramMarkdownV2 reports whether text would be accepted by a real
// Telegram sendMessage call with parse_mode=MarkdownV2: every occurrence of
// a reserved char (independentTelegramMarkdownV2SpecialChars) must be
// backslash-escaped. A bare "*" is treated as a deliberate bold marker (this
// package only ever emits matched *headline* pairs, never a lone literal
// one) rather than something that needs escaping — everything else reserved
// must be escaped. Used both by the full-epic e2e test's fake Telegram
// server and by the fast unit tests in
// notification_markdownv2_validity_test.go.
func isValidTelegramMarkdownV2(text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' {
			i++ // an escaped char (or a trailing lone backslash, itself invalid)
			if i >= len(runes) {
				return false
			}
			continue
		}
		if r == '*' {
			continue
		}
		if strings.ContainsRune(independentTelegramMarkdownV2SpecialChars, r) {
			return false
		}
	}
	return true
}
