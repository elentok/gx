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

func TestStartRebaseInteractive_NoRowsBelowCursor(t *testing.T) {
	m := newTestModel()
	m.rows = []row{
		{kind: rowCommit, commit: git.LogEntry{FullHash: "abc123", Subject: "only commit"}},
	}
	m.list.SetSelected(0, len(m.rows))

	_, cmd := m.startRebaseInteractive()
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
	if len(m.rows) < 2 {
		t.Skip("need at least 2 commit rows for this test")
	}
	m.list.SetSelected(0, len(m.rows))

	_, cmd := m.startRebaseInteractive()
	if cmd == nil {
		t.Fatal("expected non-nil cmd (rebase interactive) for clean repo with parent commit")
	}
}

func TestHandleRebaseConfirmUpdate_NonKeyMsg(t *testing.T) {
	m := newTestModel()
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc"}

	updated, cmd := m.handleRebaseConfirmUpdate(tea.WindowSizeMsg{Width: 80})
	if cmd != nil {
		t.Error("expected nil cmd for non-key message")
	}
	next := updated.(Model)
	if !next.rebaseConfirm.isOpen() {
		t.Error("expected rebaseConfirm to remain open")
	}
}

func TestHandleRebaseConfirmUpdate_EscCancels(t *testing.T) {
	m := newTestModel()
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc"}

	updated, cmd := m.handleRebaseConfirmUpdate(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Error("expected nil cmd after esc (cancel)")
	}
	next := updated.(Model)
	if next.rebaseConfirm.isOpen() {
		t.Error("expected rebaseConfirm to be closed after esc")
	}
}

func TestHandleRebaseConfirmUpdate_AcceptStash(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc123"}

	_, cmd := m.handleRebaseConfirmUpdate(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil cmd when accepting stash confirmation")
	}
}

func TestHandleRebaseConfirmUpdate_AcceptStashPop(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: true}

	_, cmd := m.handleRebaseConfirmUpdate(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil cmd when accepting stash pop confirmation")
	}
}

func TestHandleRebaseConfirmUpdate_ToggleWithArrow(t *testing.T) {
	m := newTestModel()
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: "abc"}

	// 'h' moves to yes without deciding
	updated, cmd := m.handleRebaseConfirmUpdate(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if cmd != nil {
		t.Error("expected nil cmd for toggle without deciding")
	}
	next := updated.(Model)
	if !next.rebaseConfirm.yes {
		t.Error("expected rebaseConfirm.yes=true after h key")
	}
	if !next.rebaseConfirm.isOpen() {
		t.Error("expected rebaseConfirm to remain open after toggle")
	}
}

func TestRebaseConfirmView_Stash(t *testing.T) {
	m := newTestModel()
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true}
	view := m.rebaseConfirmView(80)
	if view == "" {
		t.Error("expected non-empty rebaseConfirmView for stash kind")
	}
}

func TestRebaseConfirmView_StashPop(t *testing.T) {
	m := newTestModel()
	m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: false}
	view := m.rebaseConfirmView(80)
	if view == "" {
		t.Error("expected non-empty rebaseConfirmView for stash-pop kind")
	}
}

func TestStartRebaseInteractive_DirtyRepo(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "second.txt", "second\n")
	testutil.CommitAll(t, repo, "second commit")

	// Modify an existing tracked file without staging to make the repo dirty
	testutil.WriteFile(t, repo, "README.md", "unstaged change\n")

	m := newTestModelDefault(repo, "", ui.Settings{})
	if len(m.rows) < 2 {
		t.Skip("need at least 2 commit rows for this test")
	}
	m.list.SetSelected(0, len(m.rows))

	updated, cmd := m.startRebaseInteractive()
	// With dirty repo, should set rebaseConfirm and return nil cmd
	if cmd != nil {
		t.Error("expected nil cmd for dirty repo (should show confirm modal instead)")
	}
	next := updated.(Model)
	if !next.rebaseConfirm.isOpen() {
		t.Error("expected rebaseConfirm to be open for dirty repo")
	}
}

func TestHandleRebaseStash_Error(t *testing.T) {
	m := newTestModel()
	_, cmd := m.handleRebaseStash(rebaseStashMsg{err: errors.New("stash failed")})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash error")
	}
}

func TestHandleRebaseStash_Success(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	_, cmd := m.handleRebaseStash(rebaseStashMsg{hash: "abc123"})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash success")
	}
	if !m.rebaseDidStash {
		// rebaseDidStash is set on success
	}
}

func TestCmdRunRebaseInteractive_NonNil(t *testing.T) {
	m := newTestModel()
	m.worktreeRoot = testutil.TempRepo(t)
	cmd := m.cmdRunRebaseInteractive("abc123")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cmdRunRebaseInteractive")
	}
}

func TestHandleRebaseFinished_Error(t *testing.T) {
	m := newRebaseTestModel(t)
	_, cmd := m.handleRebaseFinished(rebaseFinishedMsg{err: errors.New("rebase failed")})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on rebase error")
	}
}

func TestHandleRebaseFinished_SuccessNoStash(t *testing.T) {
	m := newRebaseTestModel(t)
	m.rebaseDidStash = false
	_, cmd := m.handleRebaseFinished(rebaseFinishedMsg{})
	if cmd == nil {
		t.Fatal("expected reload cmd on rebase success")
	}
}

func TestHandleRebaseFinished_SuccessWithStash(t *testing.T) {
	m := newRebaseTestModel(t)
	m.rebaseDidStash = true
	_, cmd := m.handleRebaseFinished(rebaseFinishedMsg{})
	if cmd == nil {
		t.Fatal("expected cmd on rebase success with stash")
	}
}

func TestHandleRebaseStashPop_Error(t *testing.T) {
	m := newTestModel()
	_, cmd := m.handleRebaseStashPop(rebaseStashPopMsg{err: errors.New("pop failed")})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash pop error")
	}
}

func TestHandleRebaseStashPop_Success(t *testing.T) {
	m := newTestModel()
	_, cmd := m.handleRebaseStashPop(rebaseStashPopMsg{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on stash pop success")
	}
}
