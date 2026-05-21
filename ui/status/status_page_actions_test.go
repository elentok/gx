package status

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
)

func TestExecGitCaptureRunsCommand(t *testing.T) {
	repo := testutil.TempRepo(t)
	out, _, err := execGitCapture(repo, "rev-parse", "--git-dir")
	if err != nil {
		t.Fatalf("execGitCapture err: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty output from rev-parse")
	}
}

func TestExecGitCaptureReturnsErrorOnBadCommand(t *testing.T) {
	repo := testutil.TempRepo(t)
	_, _, err := execGitCapture(repo, "no-such-git-subcommand-xyzzy")
	if err == nil {
		t.Fatal("expected error for bad git subcommand")
	}
}

func TestAmendPreviewReturnsSubjectAndFiles(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "my subject")

	subject, files, err := amendPreview(repo)
	if err != nil {
		t.Fatalf("amendPreview err: %v", err)
	}
	if subject != "my subject" {
		t.Errorf("subject = %q, want 'my subject'", subject)
	}
	if len(files) != 1 || files[0] != "a.txt" {
		t.Errorf("files = %v, want [a.txt]", files)
	}
}

func TestOpenAmendConfirmOpensWithDetails(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "my commit subject")

	m := newTestModelDefault(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A", ShiftedCode: 'A'})
	m = updated.(Model)

	if !m.confirmOpen {
		t.Fatal("expected amend confirm dialog to be open after A key")
	}
	if !strings.Contains(m.confirmTitle, "Amend") {
		t.Errorf("unexpected confirmTitle: %q", m.confirmTitle)
	}
}

func TestHandleActionResultRecordsOutput(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true

	_ = m.handleActionResult(stageActionResult{
		kind:   actionAmend,
		output: "some action output",
	})

	if !m.output.HasContent() {
		t.Fatal("expected output to be recorded when result has output")
	}
}

func TestHandleActionResultWithErrorClosesRunning(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.runningOpen = true

	_ = m.handleActionResult(stageActionResult{
		kind: actionAmend,
		err:  errors.New("git failed"),
	})

	if m.runningOpen {
		t.Fatal("expected runningOpen=false after error result")
	}
}

func TestHandleActionResultPromptPopStashOpensConfirm(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.runningOpen = true

	_ = m.handleActionResult(stageActionResult{
		kind:           actionRebase,
		promptPopStash: true,
	})

	if !m.confirmOpen {
		t.Fatal("expected confirmOpen=true when promptPopStash is set")
	}
	if m.runningOpen {
		t.Fatal("expected runningOpen=false after promptPopStash")
	}
}

func TestHandleActionResultSuccessReturnsCmd(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.ready = true
	m.runningOpen = true

	cmd := m.handleActionResult(stageActionResult{kind: actionRebase})

	if m.runningOpen {
		t.Fatal("expected runningOpen=false after successful result")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd from successful action result")
	}
}

func TestHandleActionResultAmendNotifyCmd(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.ready = true
	m.runningOpen = true

	cmd := m.handleActionResult(stageActionResult{kind: actionAmend})

	if cmd == nil {
		t.Fatal("expected non-nil cmd from successful amend result")
	}
}

func TestHandleActionResultPopStashNotifyCmd(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.ready = true
	m.runningOpen = true

	cmd := m.handleActionResult(stageActionResult{kind: actionPopStashRebase})

	if cmd == nil {
		t.Fatal("expected non-nil cmd from successful pop stash rebase result")
	}
}
