package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui/nav"
)

func TestTitleBarAndSwitcherMainFlow(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	// A switcher jump carries no worktree root, so status falls back to the cwd.
	t.Chdir(repoDir)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	lines := waitForAppFrame(t, tm, "worktrees title row", func(lines []string) bool {
		return len(lines) > 2 && strings.Contains(lines[0], "gx › worktrees ▾") &&
			strings.Contains(lines[0], "? help")
	})
	if strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("line 1 should be blank, got %q", lines[1])
	}
	if last := lines[len(lines)-1]; strings.Contains(last, "status") && strings.Contains(last, "stash") {
		t.Fatalf("last line looks like a tab bar: %q", last)
	}

	tm.Send(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	waitForAppFrame(t, tm, "switcher with groups", func(lines []string) bool {
		return frameHas(lines, "go to") && frameHasRow(lines, "git") && frameHasRow(lines, "agents")
	})

	for _, r := range "stat" {
		tm.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	waitForAppFrame(t, tm, "status as the only match", func(lines []string) bool {
		return frameHas(lines, "› stat") && frameHasRow(lines, "status") && !frameHasRow(lines, "stash")
	})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitForAppFrame(t, tm, "status title row", func(lines []string) bool {
		return strings.Contains(lines[0], "gx › status ▾") && !frameHas(lines, "go to")
	})

	tm.Send(tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft})
	waitForAppFrame(t, tm, "switcher reopened by a title click", func(lines []string) bool {
		return frameHas(lines, "go to")
	})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	waitForAppFrame(t, tm, "switcher closed", func(lines []string) bool {
		return !frameHas(lines, "go to")
	})

	waitForAppFrame(t, tm, "status panels below the chrome", func(lines []string) bool {
		return len(lines) == 40 && strings.TrimSpace(lines[1]) == "" &&
			strings.Contains(lines[2], "Changes in")
	})
}

// waitForAppFrame polls the current frame (never the accumulated output) and
// returns its stripped lines once cond holds.
func waitForAppFrame(t *testing.T, tm *teatest.TestModel, what string, cond func(lines []string) bool) []string {
	t.Helper()
	var lines []string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lines = strings.Split(ansi.Strip(string(tm.CurrentFrame())), "\n")
		if cond(lines) {
			return lines
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s. Last frame (%d lines):\n%s", what, len(lines), strings.Join(lines, "\n"))
	return nil
}

func frameHas(lines []string, s string) bool {
	return strings.Contains(strings.Join(lines, "\n"), s)
}

// frameHasRow reports whether a switcher row shows exactly label, which
// keeps "status" from matching inside a longer word.
func frameHasRow(lines []string, label string) bool {
	for _, l := range lines {
		for _, cell := range strings.Split(l, "│") {
			if strings.TrimSpace(cell) == label {
				return true
			}
		}
	}
	return false
}
