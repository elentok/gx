package tickets

import "testing"

func TestRewriteMovedRef(t *testing.T) {
	for _, tc := range []struct{ ref, want string }{
		{"01", "src/01"},
		{"04a", "src/04a"},
		{"dst/02", "02"},
		{"mine:dst/02", "02"},
		{"other/02", "other/02"},
		{"ralph-loop/branch", "ralph-loop/branch"},
		{"main", "main"},
	} {
		if got := rewriteMovedRef(tc.ref, "src", "dst", "mine"); got != tc.want {
			t.Errorf("rewriteMovedRef(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}
