package git

import "testing"

func TestJoinOutput(t *testing.T) {
	tests := []struct {
		stdout, stderr string
		want           string
	}{
		{"", "", ""},
		{"out", "", "out"},
		{"", "err", "err"},
		{"out", "err", "out\nerr"},
		{"  out  ", "  err  ", "out\nerr"}, // trimmed
	}
	for _, tt := range tests {
		got := joinOutput(tt.stdout, tt.stderr)
		if got != tt.want {
			t.Errorf("joinOutput(%q, %q) = %q, want %q", tt.stdout, tt.stderr, got, tt.want)
		}
	}
}
