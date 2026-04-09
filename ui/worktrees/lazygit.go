package worktrees

import (
	"os/exec"

	"gx/git"

	tea "charm.land/bubbletea/v2"
)

type lazygitFinishedMsg struct{ err error }

func cmdLazygit(wt git.Worktree) tea.Cmd {
	c := exec.Command("lazygit", "-p", wt.Path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return lazygitFinishedMsg{err: err}
	})
}

func cmdLazygitLog(wt git.Worktree) tea.Cmd {
	c := exec.Command("lazygit", "-p", wt.Path, "log")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return lazygitFinishedMsg{err: err}
	})
}
