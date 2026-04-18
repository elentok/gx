package worktrees

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// cloneResultMsg is sent when a clone operation completes.
type cloneResultMsg struct{ err error }

// cmdClone creates a new worktree at the same HEAD as src, then copies all
// untracked and modified files from the source working tree into it.
func cmdClone(repo git.Repo, src git.Worktree, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(repo.LinkedWorktreeDir(), newName)

		fromRef := src.Branch
		if fromRef == "" {
			fromRef = src.Head
		}
		if err := git.AddWorktree(repo, newName, newPath, fromRef); err != nil {
			return cloneResultMsg{err: err}
		}

		// Copy untracked and modified files from source into the new worktree.
		changes, err := git.UncommittedChanges(src.Path)
		if err != nil {
			return cloneResultMsg{err: err}
		}
		for _, change := range changes {
			// git status --porcelain marks untracked directories with a trailing slash
			relPath := strings.TrimSuffix(change.Path, "/")
			srcPath := filepath.Join(src.Path, relPath)
			dstPath := filepath.Join(newPath, relPath)
			if err := copyPath(srcPath, dstPath); err != nil {
				return cloneResultMsg{err: fmt.Errorf("copy %s: %w", relPath, err)}
			}
		}

		return cloneResultMsg{}
	}
}

// newCloneInput creates a focused textinput pre-filled with the source name.
func newCloneInput(sourceName string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(sourceName)
	ti.CursorEnd()
	ti.Focus()
	return ti
}

// enterCloneMode transitions the model into clone mode.
func (m Model) enterCloneMode() Model {
	wt := m.selectedWorktree()
	if wt == nil {
		return m
	}
	m.mode = modeClone
	m.textInput = newCloneInput(wt.Name)
	m.statusMsg = ""
	return m
}

// handleCloneKey handles key events while in clone-input mode.
func (m Model) handleCloneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.statusMsg = "Cloning…"
		return m, cmdClone(m.repo, *wt, newName)
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	return m, tiCmd
}

// cloneView returns the one-line status bar text for clone mode.
func (m Model) cloneView() string {
	return "  Clone as: " + m.textInput.View()
}

// ── file copy helpers ─────────────────────────────────────────────────────────

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil // file was deleted in source; nothing to copy
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}
		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = io.Copy(w, r)
	return err
}
