package terminalrun

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
)

func Command(worktreeRoot string, terminal ui.Terminal, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	if terminal == ui.TerminalTmux {
		return func() tea.Msg {
			tmuxArgs := []string{"split-window", "-v", "-c", worktreeRoot, program}
			tmuxArgs = append(tmuxArgs, args...)
			err := exec.Command("tmux", tmuxArgs...).Run()
			return done(err, "tmux")
		}
	}
	if terminal == ui.TerminalKittyRemote {
		return func() tea.Msg {
			kittyArgs := []string{"@", "launch", "--copy-env", "--location=hsplit", "--cwd=" + worktreeRoot, program}
			kittyArgs = append(kittyArgs, args...)
			out, err := exec.Command("kitty", kittyArgs...).CombinedOutput()
			if err != nil {
				err = fmt.Errorf("$ kitty %s\n\n%w\n\n%s", strings.Join(kittyArgs, " "), err, strings.TrimSpace(string(out)))
			}
			return done(err, "kitty")
		}
	}
	c := exec.Command(program, args...)
	c.Dir = worktreeRoot
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return done(err, "")
	})
}
