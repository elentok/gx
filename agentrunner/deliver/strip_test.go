package deliver

import (
	"testing"
	"unicode/utf8"
)

// pinned is the spec list written out independently of strip.go.
var pinned = []runeRange{
	{0x00, 0x08}, {0x0B, 0x1F}, {0x7F, 0x9F},
	{0x2028, 0x2029},
	{0x00AD, 0x00AD}, {0x034F, 0x034F}, {0x061C, 0x061C},
	{0x115F, 0x1160}, {0x17B4, 0x17B5}, {0x180B, 0x180F},
	{0x200B, 0x200F}, {0x202A, 0x202E}, {0x2060, 0x206F},
	{0x3164, 0x3164}, {0xFE00, 0xFE0F}, {0xFEFF, 0xFEFF},
	{0xFFA0, 0xFFA0}, {0xFFF0, 0xFFFB}, {0x1D173, 0x1D17A},
	{0xE0000, 0xE0FFF},
}

func isPinned(r rune) bool {
	for _, rr := range pinned {
		if r >= rr.lo && r <= rr.hi {
			return true
		}
	}
	return false
}

func TestStripWalksEveryCodePoint(t *testing.T) {
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		in := "a" + string(r) + "b"
		got, n := Strip(in)
		if conditional(r) {
			// Between two letters a joiner stays.
			if got != in || n != 0 {
				t.Errorf("U+%04X between letters: got %q, %d", r, got, n)
			}
			continue
		}
		if isPinned(r) {
			if got != "ab" || n != 1 {
				t.Errorf("U+%04X should be removed: got %q, %d", r, got, n)
			}
		} else if got != in || n != 0 {
			t.Errorf("U+%04X should stay: got %q, %d", r, got, n)
		}
	}
}

func TestStripConditionalEdges(t *testing.T) {
	for _, r := range []rune{0x200C, 0x200D, 0xFE00, 0xFE0F, 0xE0100, 0xE01EF} {
		c := string(r)
		for _, in := range []string{c + "a", "a" + c, "a " + c + "b", "a" + c + " b"} {
			if got, n := Strip(in); n != 1 || got == in {
				t.Errorf("U+%04X in %q: got %q, %d", r, in, got, n)
			}
		}
	}
}

func TestStrip(t *testing.T) {
	tests := []struct {
		name, in, want string
		removed        int
	}{
		{"plain", "hello world", "hello world", 0},
		{"keeps tab, newline and nbsp", "a\t\n b", "a\t\n b", 0},
		{"zero-width space", "he​llo", "hello", 1},
		{"bidi marks", "a‪b‬c‎", "abc", 3},
		{"bom and soft hyphen", string(rune(0xFEFF)) + "a" + string(rune(0xAD)) + "b", "ab", 2},
		{"tag character", "a\U000E0041b", "ab", 1},
		{"hangul filler", "aㅤb", "ab", 1},
		{"bel byte", "a\x07b", "ab", 1},
		{"cr, esc, del, c1", "a\r\x1b\x7f\u0085b", "ab", 4},
		{"line and paragraph separators", "a b c", "abc", 2},
		{"keeps emoji zwj", "\U0001F468‍\U0001F469", "\U0001F468‍\U0001F469", 0},
		{"keeps persian zwnj", "می‌خواهم", "می‌خواهم", 0},
		{"zwj at start", "‍ab", "ab", 1},
		{"zwj at end", "ab‍", "ab", 1},
		{"zwnj next to space", "a ‌b", "a b", 1},
		{"keeps vs16 after emoji", "❤️x", "❤️x", 0},
		{"vs16 at end", "x❤️", "x❤", 1},
		{"zwj after zwsp", "a​‍", "a", 2},
		{"lone surrogate", "a\xed\xa0\x80b", "ab", 3},
		{"keeps real U+FFFD", "a�b", "a�b", 0},
	}
	for _, tt := range tests {
		got, removed := Strip(tt.in)
		if got != tt.want || removed != tt.removed {
			t.Errorf("%s: Strip(%q) = %q, %d; want %q, %d", tt.name, tt.in, got, removed, tt.want, tt.removed)
		}
	}
}
