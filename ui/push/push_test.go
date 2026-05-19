package push

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
)

const testPRURL = "https://github.com/owner/repo/pull/new/feature"

func newModelAtPRPrompt() Model {
	m := New()
	m.OpenAtPRPrompt(testPRURL)
	return m
}

func TestPRPromptAcceptReturnsOpenURLCmd(t *testing.T) {
	m := newModelAtPRPrompt()

	_, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !result.Done {
		t.Fatal("expected Done=true after accepting PR prompt")
	}
	if cmd == nil {
		t.Fatal("expected non-nil URL-opener cmd after accepting PR prompt")
	}
}

func TestPRPromptRejectReturnsDoneWithNoCmd(t *testing.T) {
	m := newModelAtPRPrompt()
	m.prYes = false

	_, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !result.Done {
		t.Fatal("expected Done=true after rejecting PR prompt")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after rejecting PR prompt")
	}
}

func TestPRPromptTransitionFromPushOutput(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.log = ui.NewCommandOutputLog()
	m.phase = phasePushing

	next, cmd, result := m.Update(runnerDoneMsg{
		phase:  phasePushing,
		output: "remote:   " + testPRURL + "\n",
	})

	if result.Done {
		t.Fatal("expected not Done: should show PR prompt first")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd at PR prompt transition, got non-nil")
	}
	if next.phase != phasePRPrompt {
		t.Fatalf("expected phasePRPrompt, got phase=%d", next.phase)
	}
	if next.prURL != testPRURL {
		t.Fatalf("prURL=%q, want %q", next.prURL, testPRURL)
	}
}
