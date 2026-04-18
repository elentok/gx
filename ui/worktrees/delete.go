package worktrees

import (
	"fmt"

	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

// deleteResultMsg is sent when a delete operation completes.
type deleteResultMsg struct {
	name string
	err  error
}

// cmdDelete removes the worktree directory and force-deletes its branch.
func cmdDelete(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		if err := git.RemoveWorktree(repo, wt.Name, true); err != nil {
			return deleteResultMsg{err: err}
		}
		if wt.Branch != "" {
			if err := git.DeleteLocalBranch(repo, wt.Branch, true); err != nil {
				return deleteResultMsg{err: err}
			}
		}
		return deleteResultMsg{name: wt.Name}
	}
}

func (m Model) enterDeleteConfirm() Model {
	wt := m.selectedWorktree()
	if wt == nil {
		return m
	}
	prompt := fmt.Sprintf("Delete worktree '%s' (branch: %s)?", wt.Name, wt.Branch)
	return m.enterConfirm(prompt, cmdDelete(m.repo, *wt), "Deleting "+wt.Name+"…")
}
