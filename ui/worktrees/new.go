package worktrees

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/key"
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
		return newResultMsg{err: git.AddWorktree(repo, newName, newPath, repo.MainBranch)}
	}
}

func cmdNewWorktreeAndOpen(repo git.Repo, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)
		err := git.AddWorktree(repo, newName, newPath, repo.MainBranch)
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
	m.mode = modeNew
	m.textInput = newWorktreeInput()
	m.statusMsg = ""
	return m
}

func (m Model) enterNewAndOpenMode() Model {
	m.mode = modeNewAndOpen
	m.textInput = newWorktreeInput()
	m.statusMsg = ""
	return m
}

func (m Model) handleNewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cancel := key.NewBinding(key.WithKeys("esc"))
	submit := key.NewBinding(key.WithKeys("enter"))

	switch {
	case key.Matches(msg, cancel):
		m.mode = modeNormal
		m.statusMsg = ""
		return m, nil
	case key.Matches(msg, submit):
		newName := strings.TrimSpace(m.textInput.Value())
		if newName == "" {
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		}
		prevMode := m.mode
		m.mode = modeNormal
		m.statusMsg = "Creating…"
		if prevMode == modeNewAndOpen {
			return m, cmdNewWorktreeAndOpen(m.repo, newName)
		}
		return m, cmdNewWorktree(m.repo, newName)
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	return m, tiCmd
}
