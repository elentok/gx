package amend

import (
	"os/exec"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
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

func TestOpen_WithStagedFiles(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	hash := headHash(t, repo)
	// Stage a new file
	testutil.WriteFile(t, repo, "staged.txt", "content\n")
	testutil.MustGitExported(t, repo, "add", "staged.txt")

	m := New()
	if err := m.Open(repo, hash, "initial commit"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !m.IsOpen {
		t.Error("expected IsOpen=true")
	}
	if m.Hash != hash {
		t.Errorf("expected Hash=%q, got %q", hash, m.Hash)
	}
	if m.Subject != "initial commit" {
		t.Errorf("expected Subject='initial commit', got %q", m.Subject)
	}
	if len(m.steps) == 0 {
		t.Error("expected steps to be non-empty")
	}
}

func TestOpen_NoStagedFiles_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	hash := headHash(t, repo)
	m := New()
	err := m.Open(repo, hash, "initial")
	if err == nil {
		t.Fatal("expected error when no staged files")
	}
}

func TestUpdate_StepResult_Success_SingleStep_Closes(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.running = true
	m.steps = []execStep{
		{
			Step: components.Step{TitleBefore: "amend HEAD", IsRunning: true},
			run:  func() (string, error) { return "", nil },
		},
	}
	next, _, result := m.Update(stepResultMsg{idx: 0, err: nil})
	if next.IsOpen {
		t.Error("expected IsOpen=false after single step completes")
	}
	if !result.Done {
		t.Error("expected Done=true")
	}
}

func TestUpdate_StepResult_Success_MultiStep_AdvancesToNext(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.running = true
	m.steps = []execStep{
		{
			Step: components.Step{TitleBefore: "step1", IsRunning: true},
			run:  func() (string, error) { return "", nil },
		},
		{
			Step: components.Step{TitleBefore: "step2"},
			run:  func() (string, error) { return "", nil },
		},
	}
	next, cmd, result := m.Update(stepResultMsg{idx: 0, err: nil})
	if next.stepIdx != 1 {
		t.Errorf("expected stepIdx=1, got %d", next.stepIdx)
	}
	if !next.steps[1].IsRunning {
		t.Error("expected next step to be running")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd for next step")
	}
	if result.Done {
		t.Error("expected Done=false while more steps remain")
	}
}

func TestUpdate_StepResult_Failure_SetsHasFailed(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.running = true
	fakeStepErr := fakeErr("step failed")
	m.steps = []execStep{
		{
			Step: components.Step{TitleBefore: "amend HEAD", IsRunning: true},
			run:  func() (string, error) { return "", fakeStepErr },
		},
	}
	next, _, _ := m.Update(stepResultMsg{idx: 0, err: fakeStepErr})
	if !next.steps[0].HasFailed {
		t.Error("expected HasFailed=true after step error")
	}
	if next.steps[0].IsRunning {
		t.Error("expected IsRunning=false after step error")
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestCmdRunStep_ReturnsNonNilCmd(t *testing.T) {
	m := New()
	m.steps = []execStep{
		{
			Step: components.Step{TitleBefore: "test"},
			run:  func() (string, error) { return "", nil },
		},
	}
	cmd := m.cmdRunStep(0)
	if cmd == nil {
		t.Error("expected non-nil cmd from cmdRunStep")
	}
}

func TestUpdate_StepResult_WrongIdx_Ignored(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.running = true
	m.stepIdx = 0
	m.steps = []execStep{
		{Step: components.Step{TitleBefore: "step1", IsRunning: true}},
	}
	next, cmd, _ := m.Update(stepResultMsg{idx: 1, err: nil})
	if cmd != nil {
		t.Error("expected nil cmd for wrong idx")
	}
	if next.steps[0].IsDone {
		t.Error("expected step not to be completed on wrong idx")
	}
}

func TestUpdate_SpinnerTick_WhenRunning(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.running = true
	_, cmd, result := m.Update(spinner.TickMsg{})
	if result.Done {
		t.Fatal("spinner tick should not emit done")
	}
	_ = cmd
}

func TestUpdate_SpinnerTick_WhenNotRunning(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.running = false
	_, cmd, result := m.Update(spinner.TickMsg{})
	if cmd != nil || result.Done {
		t.Fatal("spinner tick when not running should be no-op")
	}
}

func TestView_Running(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.Hash = "abc1234567"
	m.Subject = "my commit"
	m.files = []string{"foo.go"}
	m.running = true
	m.steps = []execStep{
		{Step: components.Step{TitleBefore: "amend HEAD", IsRunning: true}},
	}
	view := m.View(80)
	if view == "" {
		t.Error("expected non-empty View when running")
	}
}

func TestBuildSteps_NotHEAD_WithUnstagedChanges(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "file.txt", "v1\n")
	testutil.CommitAll(t, repo, "first")
	firstHash := headHash(t, repo)

	testutil.WriteFile(t, repo, "file.txt", "v2\n")
	testutil.CommitAll(t, repo, "second")

	// create an unstaged change so needStash=true
	testutil.WriteFile(t, repo, "file.txt", "v3-unstaged\n")

	steps, err := buildSteps(repo, firstHash)
	if err != nil {
		t.Fatalf("buildSteps: %v", err)
	}
	// fixup + stash + rebase + stash-pop = 4 steps
	if len(steps) < 4 {
		t.Errorf("expected 4 steps with unstaged changes, got %d", len(steps))
	}
}
