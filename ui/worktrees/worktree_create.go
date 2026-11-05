package worktrees

import (
	"path/filepath"

	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type newResultMsg struct{ err error }

type newOpenResultMsg struct {
	err  error
	name string
	path string
}

func cmdNewWorktree(repo git.Repo, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)
		return newResultMsg{err: git.AddWorktree(repo, newName, newPath, repo.Trunk)}
	}
}

func cmdNewWorktreeAndOpen(repo git.Repo, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)
		err := git.AddWorktree(repo, newName, newPath, repo.Trunk)
		return newOpenResultMsg{err: err, name: newName, path: newPath}
	}
}

func newWorktreeInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "worktree-name"
	ti.Focus()
	return ti
}

func (m Model) enterNewMode() Model {
	return m.openNewPrompt(cmdNewWorktree)
}

func (m Model) enterNewAndOpenMode() Model {
	return m.openNewPrompt(cmdNewWorktreeAndOpen)
}

// openNewPrompt opens the new-worktree prompt; create builds the command for a
// submitted name.
func (m Model) openNewPrompt(create func(repo git.Repo, name string) tea.Cmd) Model {
	return m.openModal(textPrompt{
		title: "New Worktree",
		input: newWorktreeInput(),
		submit: func(m Model, name string) (Model, tea.Cmd) {
			if m.repo.Trunk == "" {
				return m.showError("cannot create worktree: " + m.trunkProblem()), nil
			}
			return m, create(m.repo, name)
		},
	})
}
