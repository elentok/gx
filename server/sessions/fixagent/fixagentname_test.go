package fixagent

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestFixAgentName(t *testing.T) {
	t.Parallel()
	if got := FixAgentName("proj:loop/01"); got != "fix-proj_cloop_s01" {
		t.Errorf("ordinary address = %q", got)
	}
	pairs := [][2]string{
		{"a-b:c/01", "a:b-c/01"},
		{"a:b/01", "a-b/01"},
		{"a_c:b/01", "a:c_b/01"},
	}
	for _, p := range pairs {
		if a, b := FixAgentName(p[0]), FixAgentName(p[1]); a == b {
			t.Errorf("%q and %q share the name %q", p[0], p[1], a)
		}
	}
}

func TestFixAgentPrompt_TypesTheSessionsPrefix(t *testing.T) {
	t.Parallel()
	var tk tickets.Ticket
	for agent, want := range map[string]string{
		"claude": "/gx-investigate gx:e/01 is in error",
		"codex":  "$gx-investigate gx:e/01 is in error",
	} {
		if got := FixAgentPrompt(agent, "gx:e/01", tk); got != want {
			t.Errorf("%s prompt = %q, want %q", agent, got, want)
		}
	}
}
