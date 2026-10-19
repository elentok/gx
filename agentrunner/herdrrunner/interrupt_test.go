package herdrrunner_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
)

func keysOf(t *testing.T, state interface {
	Keys(string) ([]string, error)
}) []string {
	t.Helper()
	keys, err := state.Keys("e-01")
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestInterrupt_CodexSendsNoKeyUnlessWorkingOrBlocked(t *testing.T) {
	for _, status := range []string{"idle", "done", "unknown"} {
		t.Run(status, func(t *testing.T) {
			r, state, s := startCodex(t, nil)
			if err := state.SetAgentStatus(s.Label, status, ""); err != nil {
				t.Fatal(err)
			}
			before := keysOf(t, state)
			if err := r.Interrupt(s); err != nil {
				t.Fatalf("Interrupt: %v", err)
			}
			if got := keysOf(t, state); !slices.Equal(got, before) {
				t.Fatalf("keys after Interrupt = %q, want %q", got, before)
			}
		})
	}
}

// Codex ends an interrupted turn as done, which counts as idle.
func TestInterrupt_CodexDoneAfterKeyIsAccepted(t *testing.T) {
	r, state, s := startCodex(t, nil)
	if err := state.SetAgentStatus(s.Label, "working", ""); err != nil {
		t.Fatal(err)
	}
	before := keysOf(t, state)
	if err := r.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if got, want := keysOf(t, state)[len(before):], []string{"ctrl+c", "ctrl+u"}; !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

func TestInterrupt_AgentNotFoundAfterKeyIsAFailure(t *testing.T) {
	r, state, s := startCodex(t, nil)
	if err := state.SetAgentStatus(s.Label, "working", ""); err != nil {
		t.Fatal(err)
	}
	if err := state.VanishAfterKey(s.Label, "ctrl+c"); err != nil {
		t.Fatal(err)
	}
	if err := r.Interrupt(s); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("Interrupt err = %v, want ErrNotFound", err)
	}
}

func TestInterrupt_ClaudeInputLineIsEmptyAfterwards(t *testing.T) {
	r, state, s := startAgent(t, "claude", nil)
	if err := state.SetAgentStatus(s.Label, "working", ""); err != nil {
		t.Fatal(err)
	}
	if err := state.SetInput(s.Label, "restored prompt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if got, err := state.Input(s.Label); err != nil || got != "" {
		t.Fatalf("input line = %q, %v; want empty", got, err)
	}
}
