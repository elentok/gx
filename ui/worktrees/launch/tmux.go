package launch

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// TerminalResultMsg reports the outcome of a terminal opener.
type TerminalResultMsg struct{ Err error }

func TmuxNewSession(name, path string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", path).Run(); err != nil {
			return TerminalResultMsg{Err: err}
		}
		err := exec.Command("tmux", "switch-client", "-t", name).Run()
		return TerminalResultMsg{Err: err}
	}
}
