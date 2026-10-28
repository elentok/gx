package server

import "testing"

func TestFixAgentName(t *testing.T) {
	t.Parallel()
	if got := fixAgentName("proj:loop/01"); got != "fix-proj_cloop_s01" {
		t.Errorf("ordinary address = %q", got)
	}
	pairs := [][2]string{
		{"a-b:c/01", "a:b-c/01"},
		{"a:b/01", "a-b/01"},
		{"a_c:b/01", "a:c_b/01"},
	}
	for _, p := range pairs {
		if a, b := fixAgentName(p[0]), fixAgentName(p[1]); a == b {
			t.Errorf("%q and %q share the name %q", p[0], p[1], a)
		}
	}
}
