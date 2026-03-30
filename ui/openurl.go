package ui

import (
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// CmdOpenURL opens a URL with the platform's default handler.
func CmdOpenURL(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}
