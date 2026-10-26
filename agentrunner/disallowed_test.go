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

func TestWithDesignDisallowedTools_KeepsTheCallersBan(t *testing.T) {
	args := []string{"--disallowedTools", "Bash"}
	if got := agentrunner.WithDesignDisallowedTools(args); !slices.Equal(got, args) {
		t.Errorf("args = %q, want %q", got, args)
	}
}
