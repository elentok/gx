package worktrees

import (
	"path/filepath"
	"testing"

	"gx/git"
	"gx/testutil"

	tea "charm.land/bubbletea/v2"
)

func TestGJumpsToTop(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true
	m.worktrees = []git.Worktree{
		{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch},
		{Name: "feature-a", Path: filepath.Join(repoDir, "feature-a"), Branch: "feature-a"},
	}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())
	m.table.SetCursor(1)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd == nil {
		t.Fatalf("g should load sidebar data after jumping to top")
	}
	m = updated.(Model)
	if m.table.Cursor() != 0 {
		t.Fatalf("expected g to jump to top, got cursor=%d", m.table.Cursor())
	}
}

func TestOOOpensLogsMode(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true
	m.lastJobLog = "hello"

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("first o should not launch command")
	}
	m = updated.(Model)
	if m.keyPrefix != "o" {
		t.Fatalf("expected keyPrefix=o after first o, got %q", m.keyPrefix)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("oo should not launch command")
	}
	m = updated.(Model)
	if m.mode != modeLogs {
		t.Fatalf("expected oo to open logs mode, got mode=%v", m.mode)
	}
}

func TestOLTriggersLazygitLogCommand(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true
	m.worktrees = []git.Worktree{{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch}}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("first o should not launch command")
	}
	m = updated.(Model)
	if m.keyPrefix != "o" {
		t.Fatalf("expected keyPrefix=o after first o, got %q", m.keyPrefix)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatalf("ol should launch lazygit log command")
	}
	m = updated.(Model)
	if m.keyPrefix != "" {
		t.Fatalf("expected keyPrefix reset after ol, got %q", m.keyPrefix)
	}
}
