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
	return m.openNewPrompt(nameNew)
}

func (m Model) enterNewAndOpenMode() Model {
	return m.openNewPrompt(nameNewAndOpen)
}

// openNewPrompt opens the new-worktree prompt; kind picks the command a
// submitted name runs.
func (m Model) openNewPrompt(kind nameKind) Model {
	return m.openModal(textPrompt{
		title: "New Worktree",
		input: newWorktreeInput(),
		kind:  kind,
	})
}
