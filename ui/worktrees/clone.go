package worktrees

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/worktrees/modal"

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

// enterCloneMode opens the clone prompt.
func (m Model) enterCloneMode() Model {
	wt := m.cursorWorktree()
	if wt == nil {
		return m
	}
	return m.openModal(modal.NewClonePrompt(wt.Name))
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

// copyFile copies src to dst via a temp file plus rename, rather than writing
// dst in place, so a concurrent reader (e.g. a test's WaitFor poll, or a user
// watching the new worktree) can never observe dst partially written.
func copyFile(src, dst string) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	tmp, err := os.CreateTemp(dir, filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, dst)
}
