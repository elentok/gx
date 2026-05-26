package terminalrun

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type SplitType int

const (
	InPlace SplitType = iota
	HSplit
	VSplit
	Tab
)

func splitShellCommand(command string, keepOpen bool) (program string, args []string) {
	if !keepOpen {
		return command, nil
	}

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "sh"
	}

	var script string
	if strings.HasSuffix(shell, "fish") {
		parts := []string{
			command,
			"set code $status",
			"if test $code -ne 0",
			"echo 'gx: COMMAND FAILED, press Enter to close'",
			"else",
			"echo '--- gx: command finished, press Enter to close ---'",
			"end",
			"read -P '' _",
		}
		script = strings.Join(parts, "; ")
	} else {
		parts := []string{
			command,
			"code=$?",
			"if [ \"$code\" -ne 0 ]",
			"then echo 'gx: COMMAND FAILED, press Enter to close'",
			"else echo '--- gx: command finished, press Enter to close ---'",
			"fi",
			"read -r _",
		}
		script = strings.Join(parts, "; ")
	}
	return shell, []string{"-lc", script}
}

func Command(worktreeRoot string, terminal ui.Terminal, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	return CommandCustom(worktreeRoot, terminal, program, args, false, done)
}

func CommandCustom(worktreeRoot string, terminal ui.Terminal, program string, args []string, keepOpen bool, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	cmdProgram := program
	cmdArgs := append([]string{}, args...)
	if keepOpen {
		escaped := make([]string, 0, len(args)+1)
		escaped = append(escaped, escapeShellArg(program))
		for _, arg := range args {
			escaped = append(escaped, escapeShellArg(arg))
		}
		cmdProgram, cmdArgs = splitShellCommand(strings.Join(escaped, " "), true)
	}

	if terminal == ui.TerminalTmux {
		return func() tea.Msg {
			tmuxArgs := []string{"split-window", "-v", "-c", worktreeRoot, cmdProgram}
			tmuxArgs = append(tmuxArgs, cmdArgs...)
			err := exec.Command("tmux", tmuxArgs...).Run()
			return done(err, "tmux")
		}
	}
	if terminal == ui.TerminalKittyRemote {
		return func() tea.Msg {
			kittyArgs := []string{"@", "launch", "--copy-env", "--location=hsplit", "--cwd=" + worktreeRoot, cmdProgram}
			kittyArgs = append(kittyArgs, cmdArgs...)
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

func escapeShellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func CommandWithSplit(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	if splitType == InPlace {
		c := exec.Command(program, args...)
		c.Dir = worktreeRoot
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return done(err, "")
		})
	}

	if terminal == ui.TerminalTmux {
		return func() tea.Msg {
			var tmuxArgs []string
			switch splitType {
			case HSplit:
				tmuxArgs = []string{"split-window", "-h", "-c", worktreeRoot, program}
			case VSplit:
				tmuxArgs = []string{"split-window", "-v", "-c", worktreeRoot, program}
			case Tab:
				tmuxArgs = []string{"new-window", "-c", worktreeRoot, program}
			}
			tmuxArgs = append(tmuxArgs, args...)
			err := exec.Command("tmux", tmuxArgs...).Run()
			return done(err, "tmux")
		}
	}

	if terminal == ui.TerminalKittyRemote {
		return func() tea.Msg {
			var typeAndLoc []string
			switch splitType {
			case HSplit:
				typeAndLoc = []string{"--type=window", "--location=hsplit"}
			case VSplit:
				typeAndLoc = []string{"--type=window", "--location=vsplit"}
			case Tab:
				typeAndLoc = []string{"--type=tab"}
			}
			kittyArgs := []string{"@", "launch", "--copy-env"}
			kittyArgs = append(kittyArgs, typeAndLoc...)
			kittyArgs = append(kittyArgs, "--cwd="+worktreeRoot, program)
			kittyArgs = append(kittyArgs, args...)
			out, err := exec.Command("kitty", kittyArgs...).CombinedOutput()
			if err != nil {
				err = fmt.Errorf("$ kitty %s\n\n%w\n\n%s", strings.Join(kittyArgs, " "), err, strings.TrimSpace(string(out)))
			}
			return done(err, "kitty")
		}
	}

	return notify.Warning("split not supported for this terminal")
}
