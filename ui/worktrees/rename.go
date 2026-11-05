package worktrees

import (
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
)

// renameResultMsg is sent when a rename operation completes.
type renameResultMsg struct{ err error }

// cmdRename moves the worktree directory to a new path and renames the branch.
func cmdRename(repo git.Repo, wt git.Worktree, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)
		if err := git.MoveWorktree(repo, wt.Path, newPath); err != nil {
			return renameResultMsg{err: err}
		}
		if wt.Branch != "" && wt.Branch != newName {
			if err := git.RenameBranch(repo, wt.Branch, newName); err != nil {
				return renameResultMsg{err: err}
			}
		}
		return renameResultMsg{}
	}
}

// enterRenameMode opens the rename prompt.
func (m Model) enterRenameMode() Model {
	wt := m.cursorWorktree()
	if wt == nil {
		return m
	}
	return m.openModal(modal.NewRenamePrompt(wt.Name))
}
