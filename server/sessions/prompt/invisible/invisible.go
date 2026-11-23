package invisible

import (
	"unicode"
)

// StripInvisible removes the characters claude reports as "Removed N invisible
// character from your prompt" from text gx is about to type: the Unicode
// format characters (category Cf: zero-width space, BOM, soft hyphen, word
// joiner, bidi marks, tags) and the line and paragraph separators U+2028 and
// U+2029. Checked against claude 2.1.328.
//
// A zero-width joiner or non-joiner between two characters stays: claude keeps
// it inside emoji sequences and Persian words, and drops it only at the edge of
// the text or next to a space. It returns the text and how many runes it
// removed.
func StripInvisible(text string) (string, int) {
	in := []rune(text)
	out := make([]rune, 0, len(in))
	for i, r := range in {
		switch {
		case r == '‌' || r == '‍':
			if i == 0 || i == len(in)-1 || unicode.IsSpace(in[i-1]) || unicode.IsSpace(in[i+1]) {
				continue
			}
		case unicode.Is(unicode.Cf, r), r == ' ', r == ' ':
			continue
		}
		out = append(out, r)
	}
	return string(out), len(in) - len(out)
}
