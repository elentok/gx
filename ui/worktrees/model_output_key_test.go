package worktrees

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func TestGGJumpsToTop(t *testing.T) {
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

	// First g sets keyPrefix
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd == nil {
		// just set keyPrefix, no cmd yet
	}
	m = updated.(Model)
	if p := m.keyManager.Prefix(); len(p) != 1 || p[0] != "g" {
		t.Fatalf("expected prefix=[g] after first g, got %v", p)
	}

	// Second g jumps to top
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd == nil {
		t.Fatalf("gg should load sidebar data after jumping to top")
	}
	m = updated.(Model)
	if m.table.Cursor() != 0 {
		t.Fatalf("expected gg to jump to top, got cursor=%d", m.table.Cursor())
	}
}

func TestGOOpensLogsMode(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true
	m.lastJobLog = "hello"

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if p := m.keyManager.Prefix(); len(p) != 1 || p[0] != "g" {
		t.Fatalf("expected prefix=[g] after g, got %v", p)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("go should not launch command")
	}
	m = updated.(Model)
	if m.mode != modeLogs {
		t.Fatalf("expected go to open logs mode, got mode=%v", m.mode)
	}
}

func TestGShowsChordHint(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd != nil {
		t.Fatalf("first g should not launch command")
	}
	m = updated.(Model)

	if p := m.keyManager.Prefix(); len(p) != 1 || p[0] != "g" {
		t.Fatalf("expected prefix=[g], got %v", p)
	}
	// Chord hints are now shown in the chord overlay (via ChordHints), not statusMsg.
	hints := ui.ChordBindingsFromHints(m.keyManager.ChordHints())
	allDescs := ""
	for _, h := range hints {
		allDescs += " " + h.Keys() + " " + h.Title
	}
	for _, want := range []string{"g", "top", "o", "view output", "w", "goto worktrees", "l", "goto log", "s", "goto status"} {
		if !strings.Contains(allDescs, want) {
			t.Fatalf("expected chord hint %q in ChordHints descriptions %q", want, allDescs)
		}
	}
}

func TestLTriggersLazygitLogCommand(t *testing.T) {
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'L', Text: "L", ShiftedCode: 'L', Mod: tea.ModShift})
	if cmd == nil {
		t.Fatalf("L should launch lazygit log command and emit a notification")
	}
	// Status notifications are now emitted as notify.NotifyMsg via tea.Cmd (not m.statusMsg).
	_ = updated
}

func TestGLNavigatesToLogWhenNavigationEnabled(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := NewWithSettings(*repo, "", ui.Settings{EnableNavigation: true})
	m.ready = true
	m.worktrees = []git.Worktree{{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch}}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatalf("gl should navigate to log when navigation is enabled")
	}
	route, ok := nav.IsReplace(cmd())
	if !ok {
		t.Fatalf("expected nav replace message")
	}
	if route.Kind != nav.RouteLog {
		t.Fatalf("expected log route, got %q", route.Kind)
	}
	m = updated.(Model)
	if p := m.keyManager.Prefix(); len(p) != 0 {
		t.Fatalf("expected prefix reset after gl, got %v", p)
	}
}

func TestEnterNavigatesToLogWhenNavigationEnabled(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := NewWithSettings(*repo, "", ui.Settings{EnableNavigation: true})
	m.ready = true
	m.worktrees = []git.Worktree{{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch}}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("enter should navigate to log when navigation is enabled")
	}
	route, ok := nav.IsReplace(cmd())
	if !ok {
		t.Fatalf("expected nav replace message")
	}
	if route.Kind != nav.RouteLog {
		t.Fatalf("expected log route, got %q", route.Kind)
	}
	_ = updated
}

func TestOEntersTerminalMenuMode(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := NewWithSettings(*repo, "", ui.Settings{Terminal: ui.TerminalTmux})
	m.ready = true
	m.worktrees = []git.Worktree{{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch}}
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("o should not launch command")
	}
	m = updated.(Model)
	if m.mode != modeTerminalMenu {
		t.Fatalf("expected o to enter terminal menu mode, got mode=%v", m.mode)
	}
}
