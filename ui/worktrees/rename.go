package worktrees

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/key"
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

// enterRenameMode transitions the model into rename mode.
func (m Model) enterRenameMode() Model {
	wt := m.selectedWorktree()
	if wt == nil {
		return m
	}
	m.mode = modeRename
	m.textInput = newRenameInput(wt.Name)
	m.statusMsg = ""
	return m
}

// handleRenameKey handles key events while in rename-input mode.
func (m Model) handleRenameKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cancel := key.NewBinding(key.WithKeys("esc"))
	submit := key.NewBinding(key.WithKeys("enter"))

	switch {
	case key.Matches(msg, cancel):
		m.mode = modeNormal
		m.statusMsg = ""
		return m, nil

	case key.Matches(msg, submit):
		newName := strings.TrimSpace(m.textInput.Value())
		wt := m.selectedWorktree()
		if newName == "" || wt == nil || newName == wt.Name {
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		}
		m.mode = modeNormal
		m.statusMsg = "Renaming…"
		return m, cmdRename(m.repo, *wt, newName)
	}

	// Pass other keys (typing, backspace, etc.) to the textinput
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	return m, tiCmd
}

// renameView returns the one-line status bar text for rename mode.
func (m Model) renameView() string {
	return "  Rename to: " + m.textInput.View()
}
