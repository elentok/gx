package stash

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func stashListOutput(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "stash", "list")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git stash list: %v\n%s", err, out)
	}
	return string(out)
}

func typeString(m Model, s string) Model {
	for _, r := range s {
		m, _, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestOpen_InitializesInputPhase(t *testing.T) {
	t.Parallel()
	m := New()
	cmd := m.Open("/fake", false)
	if !m.IsOpen {
		t.Fatal("expected IsOpen=true after Open")
	}
	if m.phase != phaseInput {
		t.Fatalf("expected phaseInput, got %v", m.phase)
	}
	if !m.InputFocused() {
		t.Fatal("expected InputFocused=true at phaseInput")
	}
	_ = cmd
}

func TestSubmit_StashesAllChanges(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New()
	m.Open(repo, false)
	m = typeString(m, "mystash")

	// Enter advances phaseInput → phaseStashing and fires the stash cmd
	// (batched with a spinner tick).
	m, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != phaseStashing {
		t.Fatalf("expected phaseStashing after enter, got %v", m.phase)
	}
	if result.Done {
		t.Fatal("result should not be Done while stashing")
	}
	if cmd == nil {
		t.Fatal("expected a stash command")
	}

	// Run the stash directly (Enter batches the cmd with a spinner tick) and
	// feed the resulting done message back in.
	msg := m.cmdStash("mystash")()
	m, _, result = m.Update(msg)
	if !result.Done {
		t.Fatal("expected Done after stash finished")
	}
	if result.Outcome != OutcomeStashed {
		t.Fatalf("expected OutcomeStashed, got %v", result.Outcome)
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if m.IsOpen {
		t.Fatal("expected IsOpen=false after success")
	}
	if list := stashListOutput(t, repo); !strings.Contains(list, "mystash") {
		t.Fatalf("expected a stash named mystash, got: %q", list)
	}
}

func TestEsc_CancelsWithoutStashing(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New()
	m.Open(repo, false)

	m, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !result.Done {
		t.Fatal("expected Done after esc")
	}
	if result.Outcome != OutcomeCancelled {
		t.Fatalf("expected OutcomeCancelled, got %v", result.Outcome)
	}
	if m.IsOpen {
		t.Fatal("expected IsOpen=false after esc")
	}
	if cmd != nil {
		t.Fatal("esc should not fire a command")
	}
	if list := stashListOutput(t, repo); strings.TrimSpace(list) != "" {
		t.Fatalf("expected no stash after cancel, got: %q", list)
	}
}

func TestStashError_FailsThenDismisses(t *testing.T) {
	t.Parallel()
	m := New()
	m.Open("/nonexistent-gx-repo", false)

	// Enter fires the stash cmd against a bogus root.
	m, cmd, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a stash command")
	}
	msg := m.cmdStash("")()
	m, _, result := m.Update(msg)
	if result.Done {
		t.Fatal("error should move to phaseFailed, not Done")
	}
	if m.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", m.phase)
	}

	// Dismiss the failure → Done with the error surfaced.
	m, _, result = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !result.Done {
		t.Fatal("expected Done after dismissing failure")
	}
	if result.Err == nil {
		t.Fatal("expected Result.Err to be set")
	}
	if m.IsOpen {
		t.Fatal("expected IsOpen=false after dismissing failure")
	}
}

func TestStagedTitle_RendersStagedVariant(t *testing.T) {
	t.Parallel()
	m := New()
	m.Open("/fake", true)
	view := m.View(120)
	if !strings.Contains(view, "Stash staged changes") {
		t.Fatalf("expected staged title in view, got: %q", view)
	}
}

func TestStagedOnly_StashesOnlyStagedLeavesUnstaged(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "staged.txt", "staged\n")
	if err := git.StagePath(repo, "staged.txt"); err != nil {
		t.Fatalf("stage staged.txt: %v", err)
	}
	testutil.WriteFile(t, repo, "unstaged.txt", "unstaged\n")

	m := New()
	m.Open(repo, true)

	view := m.View(120)
	if !strings.Contains(view, "Stash staged changes") {
		t.Fatalf("expected staged title in view, got: %q", view)
	}

	msg := m.cmdStash("")()
	m, _, result := m.Update(msg)
	if !result.Done {
		t.Fatal("expected Done after stash finished")
	}
	if result.Outcome != OutcomeStashed {
		t.Fatalf("expected OutcomeStashed, got %v", result.Outcome)
	}
	if !result.StagedOnly {
		t.Fatal("expected StagedOnly=true in result")
	}

	// unstaged.txt must still be present (untracked, not stashed)
	files, err := git.ListStageFiles(repo)
	if err != nil {
		t.Fatalf("ListStageFiles: %v", err)
	}
	found := false
	for _, f := range files {
		if f.Path == "unstaged.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected unstaged.txt to remain after staged-only stash")
	}
}

func TestView_AllPhases(t *testing.T) {
	t.Parallel()
	m := New()
	m.Open("/fake", false)
	if m.View(120) == "" {
		t.Error("expected non-empty input view")
	}
	m.phase = phaseStashing
	if m.View(120) == "" {
		t.Error("expected non-empty stashing view")
	}
	m.phase = phaseFailed
	m.failErr = errFake
	if m.View(120) == "" {
		t.Error("expected non-empty failed view")
	}
}

var errFake = fakeErr("test error")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }
