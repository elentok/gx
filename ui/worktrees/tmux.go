package worktrees

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

type terminalResultMsg struct{ err error }

func cmdTmuxNewSession(name, path string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", path).Run(); err != nil {
			return terminalResultMsg{err: err}
		}
		err := exec.Command("tmux", "switch-client", "-t", name).Run()
		return terminalResultMsg{err: err}
	}
}

