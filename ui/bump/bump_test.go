package bump

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/components"
)

func modelAtPick() Model {
	m := New()
	m.IsOpen = true
	m.phase = phasePick
	m.menu = components.MenuState{
		Items: []components.MenuItem{
			{Label: "patch", Detail: "v0.1.0 → v0.1.1"},
			{Label: "minor", Detail: "v0.1.0 → v0.2.0"},
			{Label: "major", Detail: "v0.1.0 → v1.0.0"},
		},
	}
	return m
}

func TestEscapeAtPickCancelsWithoutTag(t *testing.T) {
	m := modelAtPick()

	_, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if !result.Done {
		t.Fatal("expected Done=true on escape")
	}
	if result.NewTag != "" {
		t.Fatalf("expected NewTag to be empty on cancel, got %q", result.NewTag)
	}
	if result.Err != nil {
		t.Fatalf("expected no error on cancel, got %v", result.Err)
	}
}

// Selecting an item advances to phaseTagging and emits a command.
func TestEnterAtPick_StartsTagging(t *testing.T) {
	m := modelAtPick()
	next, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.phase != phaseTagging {
		t.Fatalf("expected phaseTagging, got %v", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd after accepting")
	}
	if result.Done {
		t.Fatal("expected Done=false while tagging in progress")
	}
}

// tagDoneMsg success → Done with new tag.
func TestTagDoneSuccess_ReturnsDone(t *testing.T) {
	m := modelAtPick()
	m.phase = phaseTagging
	m.newTag = "v0.1.1"
	_, _, result := m.Update(tagDoneMsg{})
	if !result.Done {
		t.Fatal("expected Done=true after successful tag")
	}
	if result.NewTag != "v0.1.1" {
		t.Fatalf("expected NewTag=v0.1.1, got %q", result.NewTag)
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

// tagDoneMsg error → phaseFailed.
func TestTagDoneError_Fails(t *testing.T) {
	m := modelAtPick()
	m.phase = phaseTagging
	errFake := fakeErr("tag failed")
	next, _, result := m.Update(tagDoneMsg{err: errFake})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", next.phase)
	}
	if result.Done {
		t.Fatal("expected Done=false while in failed state (not yet dismissed)")
	}
}

// esc/enter/q at phaseFailed → Done with error.
func TestFailedEsc_ReturnsDoneWithError(t *testing.T) {
	m := modelAtPick()
	m.phase = phaseFailed
	m.failErr = fakeErr("tag failed")
	_, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !result.Done {
		t.Fatal("expected Done=true after dismissing failure")
	}
	if result.Err == nil {
		t.Fatal("expected non-nil error in result")
	}
}

// unhandled key at phasePick → no-op.
func TestUnhandledKeyAtPick_NoOp(t *testing.T) {
	m := modelAtPick()
	_, _, result := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if result.Done {
		t.Fatal("expected Done=false for unhandled key")
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }
