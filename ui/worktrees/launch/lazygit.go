package launch

import (
	"os/exec"

	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

// LazygitFinishedMsg reports lazygit exiting.
type LazygitFinishedMsg struct{ Err error }

func Lazygit(wt git.Worktree) tea.Cmd {
	c := exec.Command("lazygit", "-p", wt.Path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return LazygitFinishedMsg{Err: err}
	})
}

func LazygitLog(wt git.Worktree) tea.Cmd {
	c := exec.Command("lazygit", "-p", wt.Path, "log")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return LazygitFinishedMsg{Err: err}
	})
}
