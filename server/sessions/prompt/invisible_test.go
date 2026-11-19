package prompt

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
		{"keeps zwj in emoji", "👨\u200d👩", "👨\u200d👩", 0},
		{"keeps zwnj", "می\u200cخواهم", "می\u200cخواهم", 0},
		{"keeps newline and nbsp", "a\n\u00a0b", "a\n\u00a0b", 0},
	}
	for _, tt := range tests {
		got, removed := stripInvisible(tt.in)
		if got != tt.want || removed != tt.removed {
			t.Errorf("%s: stripInvisible(%q) = %q, %d; want %q, %d", tt.name, tt.in, got, removed, tt.want, tt.removed)
		}
	}
}
