package prompt

import (
	"strings"
	"unicode"
)

// stripInvisible removes the invisible format characters (Unicode category Cf:
// zero-width space, BOM, soft hyphen, bidi marks and the like) from text gx is
// about to type. Claude strips them itself on Enter, shows "Removed N
// invisible char" and holds the prompt unsent, so the Enter is lost. The
// zero-width joiner and non-joiner stay: dropping them breaks emoji sequences
// and Persian words. It returns the text and how many runes it removed.
func stripInvisible(text string) (string, int) {
	removed := 0
	out := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) && r != '‌' && r != '‍' {
			removed++
			return -1
		}
		return r
	}, text)
	return out, removed
}
