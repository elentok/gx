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

func cmdTmuxNewWindow(name, path string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command("tmux", "new-window", "-n", name, "-c", path).Run()
		return terminalResultMsg{err: err}
	}
}

func cmdTmuxHSplit(path string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command("tmux", "split-window", "-h", "-c", path).Run()
		return terminalResultMsg{err: err}
	}
}

func cmdTmuxVSplit(path string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command("tmux", "split-window", "-v", "-c", path).Run()
		return terminalResultMsg{err: err}
	}
}
