package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"testing"
)

func newTestModel(worktreeRoot string, settings ui.Settings, initialPath string) Model {
	return NewModel(worktreeRoot, false, settings, initialPath, keys.Manager{})
}

func newTestModelDefault(worktreeRoot string) Model {
	return newTestModel(worktreeRoot, DefaultSettings(), "")
}

// statusModelWithDir creates a status model with an unstaged "docs/" directory
// containing two files. The docs dir row is pre-selected.
// Precondition asserts the dir is expanded so callers start from a known state.
func statusModelWithDir(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/docs")
	testutil.WriteFile(t, repo, "docs/a.md", "a\n")
	testutil.WriteFile(t, repo, "docs/b.md", "b\n")
	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	dirIdx := -1
	for i, entry := range m.filetree.data.statusEntries {
		if entry.Kind == statusEntryDir && entry.Path == "docs" {
			dirIdx = i
			break
		}
	}
	if dirIdx < 0 {
		t.Fatalf("precondition: expected docs dir in status tree")
	}
	m.filetree.setSelection(dirIdx)
	entry, ok := m.filetree.selectedEntry()
	if !ok || !entry.Expanded {
		t.Fatalf("precondition: expected docs dir to be expanded")
	}
	return m
}

func runStatusCmds(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, cmd := m.Update(msg)
	m = updated.(Model)
	return runStatusCmd(t, m, cmd)
}

func runStatusCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runStatusCmd(t, m, c)
		}
		return m
	}
	updated, next := m.Update(msg)
	m = updated.(Model)
	return runStatusCmd(t, m, next)
}

func pressKey(m Model, code rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
	return updated.(Model)
}

func pressSpecialKey(m Model, code rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: code})
	return updated.(Model)
}
