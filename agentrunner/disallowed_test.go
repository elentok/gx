package agentrunner_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
)

func TestWithDesignDisallowedTools_BansAskUserQuestion(t *testing.T) {
	got := agentrunner.WithDesignDisallowedTools([]string{"--x"})
	if want := []string{"--x", "--disallowedTools", "AskUserQuestion"}; !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

// A caller's ban adds to gx's, so agents.claude-args can't lift it.
func TestWithDesignDisallowedTools_MergesTheCallersBan(t *testing.T) {
	for name, tc := range map[string]struct{ args, want []string }{
		"comma list": {
			[]string{"--disallowedTools", "Bash,WebFetch", "--x"},
			[]string{"--disallowedTools", "AskUserQuestion,Bash,WebFetch", "--x"},
		},
		"space separated": {
			[]string{"--disallowedTools", "Bash", "WebFetch", "--x"},
			[]string{"--disallowedTools", "AskUserQuestion,Bash,WebFetch", "--x"},
		},
		"equals form": {
			[]string{"--disallowedTools=Bash"},
			[]string{"--disallowedTools", "AskUserQuestion,Bash"},
		},
		"kebab alias and a second flag": {
			[]string{"--disallowed-tools", "Bash", "--x", "--disallowedTools", "WebFetch"},
			[]string{"--disallowedTools", "AskUserQuestion,Bash,WebFetch", "--x"},
		},
		"already banned": {
			[]string{"--disallowedTools", "AskUserQuestion"},
			[]string{"--disallowedTools", "AskUserQuestion"},
		},
	} {
		if got := agentrunner.WithDesignDisallowedTools(tc.args); !slices.Equal(got, tc.want) {
			t.Errorf("%s: args = %q, want %q", name, got, tc.want)
		}
	}
}

func TestWithLoopDisallowedTools_MergesTheCallersBan(t *testing.T) {
	got := agentrunner.WithLoopDisallowedTools([]string{"--disallowedTools", "Bash"})
	want := []string{"--disallowedTools", agentrunner.LoopDisallowedTools + ",Bash"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}
