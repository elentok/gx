package worktrees

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/worktrees/modal"

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
		{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.Trunk},
	}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())
	return m
}

func TestInputFocused_TrueInRenameMode(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	m.modal = modal.NewRenamePrompt("")
	if !m.InputFocused() {
		t.Error("expected InputFocused=true with the rename prompt open")
	}
}

func TestInputFocused_TrueInCloneMode(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	m.modal = modal.NewRenamePrompt("")
	if !m.InputFocused() {
		t.Error("expected InputFocused=true with the clone prompt open")
	}
}

func TestInputFocused_TrueInNewMode(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	m.modal = modal.NewRenamePrompt("")
	if !m.InputFocused() {
		t.Error("expected InputFocused=true with the new prompt open")
	}
}

func TestInputFocused_TrueInSearchMode(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	m.modal = searchModal{}
	if !m.InputFocused() {
		t.Error("expected InputFocused=true with search open")
	}
}

func TestInputFocused_FalseInNormalMode(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	m.modal = nil
	if m.InputFocused() {
		t.Error("expected InputFocused=false with no modal")
	}
}

func TestUpdate_RenameResultMsg_Error(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	updated, _ := m.Update(renameResultMsg{err: errTest})
	next := updated.(Model)
	if _, ok := next.modal.(modal.Error); !ok {
		t.Errorf("expected error modal after rename error, got %T", next.modal)
	}
}

func TestUpdate_CloneResultMsg_Error(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	updated, _ := m.Update(cloneResultMsg{err: errTest})
	next := updated.(Model)
	if _, ok := next.modal.(modal.Error); !ok {
		t.Errorf("expected error modal after clone error, got %T", next.modal)
	}
}

func TestUpdate_NewResultMsg_Error(t *testing.T) {
	t.Parallel()
	m := newUpdateTestModel(t)
	updated, _ := m.Update(newResultMsg{err: errTest})
	next := updated.(Model)
	if _, ok := next.modal.(modal.Error); !ok {
		t.Errorf("expected error modal after new error, got %T", next.modal)
	}
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	t.Parallel()
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
