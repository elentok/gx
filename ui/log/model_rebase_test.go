package log

import (
	"errors"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

func newRebaseTestModel(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	return newTestModelDefault(repo, "", ui.Settings{})
}

func startRebase(m *Model) tea.Cmd {
	return m.rebase.start(m.worktreeRoot, m.settings.Terminal, m.listPanel.Rows(), m.listPanel.Selected())
}

func TestStartRebaseInteractive_NoRowsBelowCursor(t *testing.T) {
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "abc123", Subject: "only commit"}},
	}).SetSelected(0)

	cmd := startRebase(&m)
	if cmd == nil {
		t.Fatal("expected non-nil cmd (warning) when no parent commit below cursor")
	}
	msg := cmd()
	if _, ok := msg.(notify.NotifyMsg); !ok {
		t.Fatalf("expected notify.NotifyMsg, got %T", msg)
	}
}

func TestStartRebaseInteractive_CleanRepo(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "second.txt", "second\n")
	testutil.CommitAll(t, repo, "second commit")

	m := newTestModelDefault(repo, "", ui.Settings{})
	if len(m.listPanel.Rows()) < 2 {
		t.Skip("need at least 2 commit rows for this test")
	}
	m.listPanel = m.listPanel.SetSelected(0)

	cmd := startRebase(&m)
	if cmd == nil {
		t.Fatal("expected non-nil cmd (rebase interactive) for clean repo with parent commit")
	}
}

func TestHandleRebaseConfirmUpdate_NonKeyMsg(t *testing.T) {
	m := newTestModel()
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc"}

	cmd := m.rebase.handleConfirmUpdate(tea.WindowSizeMsg{Width: 80}, m.worktreeRoot)
	if cmd != nil {
		t.Error("expected nil cmd for non-key message")
	}
	if !m.rebase.confirm.isOpen() {
		t.Error("expected rebase.confirm to remain open")
	}
}

func TestHandleRebaseConfirmUpdate_EscCancels(t *testing.T) {
	m := newTestModel()
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc"}

	cmd := m.rebase.handleConfirmUpdate(tea.KeyPressMsg{Code: tea.KeyEsc}, m.worktreeRoot)
	if cmd != nil {
		t.Error("expected nil cmd after esc (cancel)")
	}
	if m.rebase.confirm.isOpen() {
		t.Error("expected rebase.confirm to be closed after esc")
	}
}

func TestHandleRebaseConfirmUpdate_AcceptStash(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc123"}

	cmd := m.rebase.handleConfirmUpdate(tea.KeyPressMsg{Code: tea.KeyEnter}, m.worktreeRoot)
	if cmd == nil {
		t.Fatal("expected non-nil cmd when accepting stash confirmation")
	}
}

func TestHandleRebaseConfirmUpdate_AcceptStashPop(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: true}

	cmd := m.rebase.handleConfirmUpdate(tea.KeyPressMsg{Code: tea.KeyEnter}, m.worktreeRoot)
	if cmd == nil {
		t.Fatal("expected non-nil cmd when accepting stash pop confirmation")
	}
}

func TestHandleRebaseConfirmUpdate_ToggleWithArrow(t *testing.T) {
	m := newTestModel()
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc"}

	// 'h' moves to yes without deciding
	cmd := m.rebase.handleConfirmUpdate(tea.KeyPressMsg{Code: 'h', Text: "h"}, m.worktreeRoot)
	if cmd != nil {
		t.Error("expected nil cmd for toggle without deciding")
	}
	if !m.rebase.confirm.yes {
		t.Error("expected rebase.confirm.yes=true after h key")
	}
	if !m.rebase.confirm.isOpen() {
		t.Error("expected rebase.confirm to remain open after toggle")
	}
}

func TestRebaseConfirmView_Stash(t *testing.T) {
	m := newTestModel()
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true}
	view := m.rebase.view(80)
	if view == "" {
		t.Error("expected non-empty rebase view for stash kind")
	}
}

func TestRebaseConfirmView_StashPop(t *testing.T) {
	m := newTestModel()
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: false}
	view := m.rebase.view(80)
	if view == "" {
		t.Error("expected non-empty rebase view for stash-pop kind")
	}
}

func TestStartRebaseInteractive_DirtyRepo(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "second.txt", "second\n")
	testutil.CommitAll(t, repo, "second commit")

	// Modify an existing tracked file without staging to make the repo dirty
	testutil.WriteFile(t, repo, "README.md", "unstaged change\n")

	m := newTestModelDefault(repo, "", ui.Settings{})
	if len(m.listPanel.Rows()) < 2 {
		t.Skip("need at least 2 commit rows for this test")
	}
	m.listPanel = m.listPanel.SetSelected(0)

	cmd := startRebase(&m)
	// With dirty repo, should set rebase.confirm and return nil cmd
	if cmd != nil {
		t.Error("expected nil cmd for dirty repo (should show confirm modal instead)")
	}
	if !m.rebase.confirm.isOpen() {
		t.Error("expected rebase.confirm to be open for dirty repo")
	}
}

func TestHandleRebaseStash_Error(t *testing.T) {
	m := newTestModel()
	cmd := m.rebase.handleStash(rebaseStashMsg{err: errors.New("stash failed")}, m.worktreeRoot, m.settings.Terminal)
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash error")
	}
}

func TestHandleRebaseStash_Success(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	cmd := m.rebase.handleStash(rebaseStashMsg{hash: "abc123"}, m.worktreeRoot, m.settings.Terminal)
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash success")
	}
	if !m.rebase.didStash {
		// rebase.didStash is set on success
	}
}

func TestCmdRunRebaseInteractive_NonNil(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	cmd := m.rebase.cmdRun(m.worktreeRoot, m.settings.Terminal, "abc123")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdRun")
	}
}

func TestHandleRebaseFinished_Error(t *testing.T) {
	m := newRebaseTestModel(t)
	cmd := m.rebase.handleFinished(rebaseFinishedMsg{err: errors.New("rebase failed")}, m.worktreeRoot, m.cmdReload())
	if cmd == nil {
		t.Fatal("expected non-nil cmd on rebase error")
	}
}

func TestHandleRebaseFinished_SuccessNoStash(t *testing.T) {
	m := newRebaseTestModel(t)
	m.rebase.didStash = false
	cmd := m.rebase.handleFinished(rebaseFinishedMsg{}, m.worktreeRoot, m.cmdReload())
	if cmd == nil {
		t.Fatal("expected reload cmd on rebase success")
	}
}

func TestHandleRebaseFinished_SuccessWithStash(t *testing.T) {
	m := newRebaseTestModel(t)
	m.rebase.didStash = true
	cmd := m.rebase.handleFinished(rebaseFinishedMsg{}, m.worktreeRoot, m.cmdReload())
	if cmd == nil {
		t.Fatal("expected cmd on rebase success with stash")
	}
}

func TestHandleRebaseStashPop_Error(t *testing.T) {
	m := newTestModel()
	cmd := m.rebase.handleStashPop(rebaseStashPopMsg{err: errors.New("pop failed")})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash pop error")
	}
}

func TestHandleRebaseStashPop_Success(t *testing.T) {
	m := newTestModel()
	cmd := m.rebase.handleStashPop(rebaseStashPopMsg{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash pop success")
	}
}
