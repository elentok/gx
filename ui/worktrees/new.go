package worktrees

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type tmuxOpenMode int

const (
	tmuxOpenSession tmuxOpenMode = iota
	tmuxOpenWindow
)

type newResultMsg struct{ err error }

type newTmuxResultMsg struct {
	err      error
	name     string
	path     string
	openMode tmuxOpenMode
}

func cmdNewWorktree(repo git.Repo, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)
		return newResultMsg{err: git.AddWorktree(repo, newName, newPath, repo.MainBranch)}
	}
}

func cmdNewWorktreeAndTmux(repo git.Repo, newName string, openMode tmuxOpenMode) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)
		err := git.AddWorktree(repo, newName, newPath, repo.MainBranch)
		return newTmuxResultMsg{err: err, name: newName, path: newPath, openMode: openMode}
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

func (m Model) enterNewTmuxSessionMode() Model {
	m.mode = modeNewTmuxSession
	m.textInput = newWorktreeInput()
	m.statusMsg = ""
	return m
}

func (m Model) enterNewTmuxWindowMode() Model {
	m.mode = modeNewTmuxWindow
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
		switch prevMode {
		case modeNewTmuxSession:
			return m, cmdNewWorktreeAndTmux(m.repo, newName, tmuxOpenSession)
		case modeNewTmuxWindow:
			return m, cmdNewWorktreeAndTmux(m.repo, newName, tmuxOpenWindow)
		default:
			return m, cmdNewWorktree(m.repo, newName)
		}
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	return m, tiCmd
}

func (m Model) newView() string {
	label := "New worktree"
	switch m.mode {
	case modeNewTmuxSession:
		label = "New worktree + tmux session"
	case modeNewTmuxWindow:
		label = "New worktree + tmux window"
	}
	return "  " + label + ": " + m.textInput.View()
}
