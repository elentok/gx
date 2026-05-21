package amend

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/components"
)

func headHash(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestNew(t *testing.T) {
	m := New()
	if m.IsOpen {
		t.Error("expected IsOpen=false initially")
	}
}

func TestHasFailed_Empty(t *testing.T) {
	m := New()
	if m.hasFailed() {
		t.Error("expected hasFailed=false with no steps")
	}
}

func TestHasFailed_WithFailed(t *testing.T) {
	m := New()
	m.steps = []execStep{
		{Step: components.Step{TitleFailed: "amend failed", HasFailed: true}},
	}
	if !m.hasFailed() {
		t.Error("expected hasFailed=true when step has failed")
	}
}

func TestStepErr_NoFailure(t *testing.T) {
	m := New()
	if m.stepErr() != nil {
		t.Error("expected nil stepErr with no steps")
	}
}

func TestStepErr_WithFailure(t *testing.T) {
	m := New()
	m.steps = []execStep{
		{Step: components.Step{TitleFailed: "amend failed", HasFailed: true}},
	}
	err := m.stepErr()
	if err == nil {
		t.Fatal("expected non-nil stepErr when step failed")
	}
	if err.Error() != "amend failed" {
		t.Errorf("stepErr() = %q, want 'amend failed'", err.Error())
	}
}

func TestStepError_Error(t *testing.T) {
	e := &StepError{Title: "test failure"}
	if e.Error() != "test failure" {
		t.Errorf("StepError.Error() = %q, want 'test failure'", e.Error())
	}
}

func TestView_Basic(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.Hash = "abc1234567"
	m.Subject = "my commit"
	m.files = []string{"foo.go"}
	view := m.View(80)
	if view == "" {
		t.Error("expected non-empty View")
	}
}

func TestUpdate_NonKeyMsg(t *testing.T) {
	m := New()
	m.IsOpen = true
	next, cmd, result := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if result.Done || result.Decided {
		t.Error("window size msg should not trigger any result")
	}
	_ = next
	_ = cmd
}

func TestUpdate_EscCloses(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.yes = true
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.IsOpen {
		t.Error("expected IsOpen=false after esc")
	}
	if !result.Done {
		t.Error("expected Done=true after esc")
	}
}

func TestBuildSteps_IsHEAD(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "file.txt", "hello\n")
	testutil.CommitAll(t, repo, "initial")

	hash := headHash(t, repo)
	steps, err := buildSteps(repo, hash)
	if err != nil {
		t.Fatalf("buildSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step for HEAD commit, got %d", len(steps))
	}
}

func TestBuildSteps_NotHEAD(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "file.txt", "v1\n")
	testutil.CommitAll(t, repo, "first")
	firstHash := headHash(t, repo)

	testutil.WriteFile(t, repo, "file.txt", "v2\n")
	testutil.CommitAll(t, repo, "second")

	steps, err := buildSteps(repo, firstHash)
	if err != nil {
		t.Fatalf("buildSteps: %v", err)
	}
	if len(steps) < 2 {
		t.Errorf("expected multiple steps for non-HEAD commit, got %d", len(steps))
	}
}
