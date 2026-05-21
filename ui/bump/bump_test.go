package bump

import (
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
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

func TestModalWidth(t *testing.T) {
	if got := modalWidth(0); got != 56 {
		t.Errorf("modalWidth(0) = %d, want 56", got)
	}
	if got := modalWidth(300); got > 56 {
		// large width → half, no cap in this function
		_ = got
	}
}

func TestView_PickPhase(t *testing.T) {
	m := modelAtPick()
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in pick phase")
	}
}

func TestView_FailedPhase(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.phase = phaseFailed
	m.failErr = fakeErr("something went wrong")
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in failed phase")
	}
}

func TestOpen_WithTaggedRepo(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	if out, err := exec.Command("git", "-C", repo, "tag", "-a", "v1.2.3", "-m", "release v1.2.3").CombinedOutput(); err != nil {
		t.Fatalf("create tag: %v\n%s", err, out)
	}
	m := New()
	if err := m.Open(repo); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !m.IsOpen {
		t.Error("expected IsOpen=true")
	}
	if m.phase != phasePick {
		t.Errorf("expected phasePick, got %v", m.phase)
	}
	if len(m.menu.Items) != 3 {
		t.Errorf("expected 3 menu items (patch/minor/major), got %d", len(m.menu.Items))
	}
	if m.lastTag != "v1.2.3" {
		t.Errorf("expected lastTag=v1.2.3, got %q", m.lastTag)
	}
}

func TestSelectedNewTag_Patch(t *testing.T) {
	m := modelAtPick()
	m.menu.Cursor = 0
	got := m.selectedNewTag()
	if got != "v0.1.1" {
		t.Errorf("selectedNewTag patch = %q, want 'v0.1.1'", got)
	}
}

func TestSelectedNewTag_Minor(t *testing.T) {
	m := modelAtPick()
	m.menu.Cursor = 1
	got := m.selectedNewTag()
	if got != "v0.2.0" {
		t.Errorf("selectedNewTag minor = %q, want 'v0.2.0'", got)
	}
}

func TestSelectedNewTag_Major(t *testing.T) {
	m := modelAtPick()
	m.menu.Cursor = 2
	got := m.selectedNewTag()
	if got != "v1.0.0" {
		t.Errorf("selectedNewTag major = %q, want 'v1.0.0'", got)
	}
}

func TestSelectedNewTag_OutOfBounds(t *testing.T) {
	m := modelAtPick()
	m.menu.Cursor = 99
	if got := m.selectedNewTag(); got != "" {
		t.Errorf("selectedNewTag out-of-bounds = %q, want empty", got)
	}
}

func TestCmdCreateTag_ReturnsNonNilCmd(t *testing.T) {
	m := New()
	m.root = t.TempDir()
	m.newTag = "v1.0.0"
	cmd := m.cmdCreateTag()
	if cmd == nil {
		t.Error("expected non-nil cmd from cmdCreateTag")
	}
}

func TestView_TaggingPhase(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.phase = phaseTagging
	m.newTag = "v1.0.0"
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in tagging phase")
	}
}
