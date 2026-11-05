package worktrees

import (
	"path/filepath"

	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/textinput"
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

// newRenameInput creates a focused textinput pre-filled with the current name.
func newRenameInput(currentName string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(currentName)
	// Position cursor at end so the user can backspace or edit inline
	ti.CursorEnd()
	ti.Focus()
	return ti
}

// enterRenameMode opens the rename prompt.
func (m Model) enterRenameMode() Model {
	wt := m.cursorWorktree()
	if wt == nil {
		return m
	}
	return m.openModal(textPrompt{
		title: "Rename Worktree",
		input: newRenameInput(wt.Name),
		submit: func(m Model, newName string) (Model, tea.Cmd) {
			wt := m.cursorWorktree()
			if wt == nil || newName == wt.Name {
				return m, nil
			}
			return m, cmdRename(m.repo, *wt, newName)
		},
	})
}
