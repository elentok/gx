package push

import (
	"testing"
	"time"

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

// phaseConfirm accept → starts fetch.
func TestConfirmAccept_StartsFetch(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseConfirm
	m.yes = true
	m.remote = "origin"
	next, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if result.Done {
		t.Fatal("expected Done=false while fetching")
	}
	if next.phase != phaseFetching {
		t.Fatalf("expected phaseFetching, got %d", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected non-nil fetch command")
	}
}

// phaseConfirm decline → closes.
func TestConfirmDecline_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseConfirm
	m.yes = true
	next, _, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after decline")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

// runnerDoneMsg error → phaseFailed.
func TestRunnerDoneError_Fails(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseFetching
	next, _, _ := m.Update(runnerDoneMsg{phase: phaseFetching, err: fakeErr("network error")})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %d", next.phase)
	}
}

// phaseFailed esc → closes with error.
func TestFailedEsc_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseFailed
	m.failErr = fakeErr("oops")
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false")
	}
	if !result.Done || result.Err == nil {
		t.Fatal("expected Done=true with error")
	}
}

// humanizeOrUnknown returns "unknown time" for zero time.
func TestHumanizeOrUnknown_Zero(t *testing.T) {
	got := humanizeOrUnknown(time.Time{})
	if got != "unknown time" {
		t.Errorf("got %q, want 'unknown time'", got)
	}
}

// humanizeOrUnknown returns a relative string for a non-zero time.
func TestHumanizeOrUnknown_NonZero(t *testing.T) {
	got := humanizeOrUnknown(time.Now().Add(-1 * time.Hour))
	if got == "unknown time" {
		t.Error("expected relative time, got 'unknown time'")
	}
}

// stepPushTag uses the tag field.
func TestStepPushTag_UsesTagField(t *testing.T) {
	m := New()
	m.tag = "v1.2.3"
	step := m.stepPushTag()
	if step.TitleBefore != "push tag v1.2.3" {
		t.Errorf("unexpected TitleBefore: %q", step.TitleBefore)
	}
}

func newModelWithLog() Model {
	m := New()
	m.IsOpen = true
	m.log = ui.NewCommandOutputLog()
	return m
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

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
