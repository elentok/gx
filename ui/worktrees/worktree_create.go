package worktrees

import (
	"path/filepath"

	"github.com/elentok/gx/git"

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
