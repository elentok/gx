// Package deliver types text into an agent's terminal. It is a leaf: it
// imports only the standard library, and agentrunner/claudeui must never
// import it.
package deliver

import (
	"unicode"
	"unicode/utf8"
)

type runeRange struct{ lo, hi rune }

// removed holds the characters claude strips at Enter, pinned to claude
// 2.1.330. The set will drift; the settle Enter is the guard, not this list.
// A stripped character at the start of a "/" or "!" command swallows the Enter.
var removed = []runeRange{
	{0x00, 0x08}, {0x0B, 0x1F}, {0x7F, 0x9F}, // C0 without TAB and LF, DEL, C1
	{0x2028, 0x2029},
	{0x00AD, 0x00AD}, {0x034F, 0x034F}, {0x061C, 0x061C},
	{0x115F, 0x1160}, {0x17B4, 0x17B5}, {0x180B, 0x180F},
	{0x200B, 0x200B}, {0x200E, 0x200F}, {0x202A, 0x202E},
	{0x2060, 0x206F}, {0x3164, 0x3164}, {0xFEFF, 0xFEFF},
	{0xFFA0, 0xFFA0}, {0xFFF0, 0xFFFB}, {0x1D173, 0x1D17A},
	{0xE0000, 0xE00FF}, {0xE01F0, 0xE0FFF},
}

func inRanges(r rune, ranges []runeRange) bool {
	for _, rr := range ranges {
		if r >= rr.lo && r <= rr.hi {
			return true
		}
	}
	return false
}

// conditional characters join their neighbours (emoji sequences, Persian
// words): claude keeps them between two non-space characters.
func conditional(r rune) bool {
	return r == 0x200C || r == 0x200D ||
		(r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF)
}

// Strip removes the characters claude strips from a prompt and returns the
// cleaned text and how many runes it removed. A lone surrogate is invalid
// UTF-8 in a Go string and goes too. Conditional characters are judged after
// the unconditional removals, so "a<ZWSP><ZWJ>" loses the ZWJ.
func Strip(text string) (string, int) {
	kept := make([]rune, 0, len(text))
	dropped := 0
	for i := 0; i < len(text); {
		r, w := utf8.DecodeRuneInString(text[i:])
		i += w
		if (r == utf8.RuneError && w == 1) || inRanges(r, removed) {
			dropped++
			continue
		}
		kept = append(kept, r)
	}
	out := make([]rune, 0, len(kept))
	for i, r := range kept {
		if conditional(r) && (i == 0 || i == len(kept)-1 ||
			unicode.IsSpace(kept[i-1]) || unicode.IsSpace(kept[i+1])) {
			dropped++
			continue
		}
		out = append(out, r)
	}
	return string(out), dropped
}
