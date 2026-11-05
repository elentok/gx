package worktrees

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
)

// yankDataMsg is sent when the file list for yank mode has been loaded.
type yankDataMsg struct {
	worktreePath string
	changes      []git.Change
	err          error
}

// cmdLoadYankData loads uncommitted changes for the given worktree.
func cmdLoadYankData(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		changes, err := git.UncommittedChanges(wt.Path)
		return yankDataMsg{worktreePath: wt.Path, changes: changes, err: err}
	}
}

// enterYankMode opens the yank modal and loads the file list for the selected
// worktree.
func (m Model) enterYankMode() (Model, tea.Cmd) {
	wt := m.cursorWorktree()
	if wt == nil {
		return m, nil
	}
	return m.openModal(modal.NewYank(*wt)), cmdLoadYankData(*wt)
}

// changesToChecklistItems converts git changes into checklist items.
func changesToChecklistItems(changes []git.Change) []components.Item {
	items := make([]components.Item, len(changes))
	for i, c := range changes {
		path := strings.TrimSuffix(c.Path, "/") // untracked dirs have trailing slash
		items[i] = components.Item{
			Label:   string(c.Kind) + "  " + path,
			Value:   filepath.ToSlash(path),
			Checked: true,
		}
	}
	return items
}
