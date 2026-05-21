package worktrees

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
)

var errTest = errors.New("test error")

func newUpdateTestModel(t *testing.T) Model {
	t.Helper()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	m := New(*repo, "")
	m.ready = true
	m.worktrees = []git.Worktree{
		{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch},
	}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())
	return m
}

func TestInputFocused_TrueInRenameMode(t *testing.T) {
	m := newUpdateTestModel(t)
	m.mode = modeRename
	if !m.InputFocused() {
		t.Error("expected InputFocused=true in modeRename")
	}
}

func TestInputFocused_TrueInCloneMode(t *testing.T) {
	m := newUpdateTestModel(t)
	m.mode = modeClone
	if !m.InputFocused() {
		t.Error("expected InputFocused=true in modeClone")
	}
}

func TestInputFocused_TrueInNewMode(t *testing.T) {
	m := newUpdateTestModel(t)
	m.mode = modeNew
	if !m.InputFocused() {
		t.Error("expected InputFocused=true in modeNew")
	}
}

func TestInputFocused_TrueInSearchMode(t *testing.T) {
	m := newUpdateTestModel(t)
	m.mode = modeSearch
	if !m.InputFocused() {
		t.Error("expected InputFocused=true in modeSearch")
	}
}

func TestInputFocused_FalseInNormalMode(t *testing.T) {
	m := newUpdateTestModel(t)
	m.mode = modeNormal
	if m.InputFocused() {
		t.Error("expected InputFocused=false in modeNormal")
	}
}

func TestUpdate_RenameResultMsg_Error(t *testing.T) {
	m := newUpdateTestModel(t)
	updated, _ := m.Update(renameResultMsg{err: errTest})
	next := updated.(Model)
	if next.mode != modeError {
		t.Errorf("expected modeError after rename error, got %v", next.mode)
	}
}

func TestUpdate_CloneResultMsg_Error(t *testing.T) {
	m := newUpdateTestModel(t)
	updated, _ := m.Update(cloneResultMsg{err: errTest})
	next := updated.(Model)
	if next.mode != modeError {
		t.Errorf("expected modeError after clone error, got %v", next.mode)
	}
}

func TestUpdate_NewResultMsg_Error(t *testing.T) {
	m := newUpdateTestModel(t)
	updated, _ := m.Update(newResultMsg{err: errTest})
	next := updated.(Model)
	if next.mode != modeError {
		t.Errorf("expected modeError after new error, got %v", next.mode)
	}
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := newUpdateTestModel(t)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	if cmd != nil {
		t.Error("expected nil cmd from WindowSizeMsg")
	}
	next := updated.(Model)
	if next.width != 200 || next.height != 50 {
		t.Errorf("expected width=200 height=50, got %d %d", next.width, next.height)
	}
}
