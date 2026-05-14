package worktrees

import (
	"testing"

	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

func TestPasteMsgUpdatesNewWorktreeInput(t *testing.T) {
	m := Model{}.enterNewMode()

	updated, _ := m.Update(tea.PasteMsg{Content: "feature-pasted"})
	m = updated.(Model)

	if got := m.textInput.Value(); got != "feature-pasted" {
		t.Fatalf("new mode paste value = %q, want %q", got, "feature-pasted")
	}
}

func TestPasteMsgUpdatesRenameInput(t *testing.T) {
	m := Model{
		worktrees: []git.Worktree{{Name: "feature-a"}},
	}
	m = m.enterRenameMode()

	updated, _ := m.Update(tea.PasteMsg{Content: "feature-renamed"})
	m = updated.(Model)

	if got := m.textInput.Value(); got != "feature-afeature-renamed" {
		t.Fatalf("rename mode paste value = %q, want %q", got, "feature-afeature-renamed")
	}
}

func TestPasteMsgUpdatesSearchQuery(t *testing.T) {
	m := Model{
		worktrees: []git.Worktree{
			{Name: "feature-a", Branch: "feature-a"},
			{Name: "bugfix-b", Branch: "bugfix-b"},
		},
	}
	m = m.enterSearchMode()

	updated, _ := m.Update(tea.PasteMsg{Content: "bug"})
	m = updated.(Model)

	if got := m.search.Query(); got != "bug" {
		t.Fatalf("search query after paste = %q, want %q", got, "bug")
	}
	if got := m.search.Matches(); len(got) != 1 || got[0].Index != 1 {
		t.Fatalf("search matches after paste = %v, want [{Index:1}]", got)
	}
}
