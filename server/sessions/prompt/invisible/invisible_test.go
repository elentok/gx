package invisible

import "testing"

func TestStripInvisible(t *testing.T) {
	tests := []struct {
		name, in, want string
		removed        int
	}{
		{"plain", "hello world", "hello world", 0},
		{"zero-width space", "he\u200bllo", "hello", 1},
		{"bom and soft hyphen", "\ufeffa\u00adb", "ab", 2},
		{"bidi marks", "a\u202ab\u202cc\u200e", "abc", 3},
		{"line and paragraph separators", "a\u2028b\u2029c", "abc", 2},
		{"trailing zwj", "ok\u200d", "ok", 1},
		{"zwnj next to a space", "a \u200cb", "a b", 1},
		{"keeps zwj in emoji", "👨\u200d👩", "👨\u200d👩", 0},
		{"keeps zwnj", "می\u200cخواهم", "می\u200cخواهم", 0},
		{"keeps newline and nbsp", "a\n\u00a0b", "a\n\u00a0b", 0},
	}
	for _, tt := range tests {
		got, removed := StripInvisible(tt.in)
		if got != tt.want || removed != tt.removed {
			t.Errorf("%s: StripInvisible(%q) = %q, %d; want %q, %d", tt.name, tt.in, got, removed, tt.want, tt.removed)
		}
	}
}
