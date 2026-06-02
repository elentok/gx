package terminalrun

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

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

// osExecutable is a seam so tests can exercise the gx-path fallback.
var osExecutable = os.Executable

var (
	gxPathOnce  sync.Once
	gxPathValue string
)

// gxPath returns gx's own absolute path (resolved once, cached). Splits run in
// a fresh shell where `gx` may not be on PATH, so we invoke the absolute path;
// if it can't be resolved we fall back to "gx".
func gxPath() string {
	gxPathOnce.Do(func() { gxPathValue = resolveGxPath() })
	return gxPathValue
}

// resetGxPath clears the cached gx path so tests can re-resolve it.
func resetGxPath() {
	gxPathOnce = sync.Once{}
	gxPathValue = ""
}

func resolveGxPath() string {
	if exe, err := osExecutable(); err == nil && exe != "" {
		return exe
	}
	return "gx"
}

// wrapRun rewrites a command launch so it runs under `gx run`, which keeps the
// pane open (showing the failed command and a prompt) only when the command
// fails. Interactive shells skip this — they use the *Bare variants.
func wrapRun(program string, args []string) (string, []string) {
	wrapped := make([]string, 0, len(args)+2)
	wrapped = append(wrapped, "run", program)
	wrapped = append(wrapped, args...)
	return gxPath(), wrapped
}

// Command launches a command into the default split (vertical for tmux,
// hsplit for kitty) or in place on a plain terminal, wrapped in `gx run` so a
// failure keeps the pane open.
func Command(worktreeRoot string, terminal ui.Terminal, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	program, args = wrapRun(program, args)

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

// CommandWithSplit launches a command into the requested split type, wrapped in
// `gx run` so a failure keeps the pane open. Use CommandWithSplitBare for
// interactive shells, which should not be wrapped.
func CommandWithSplit(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	program, args = wrapRun(program, args)
	return commandWithSplit(worktreeRoot, terminal, splitType, program, args, done)
}

// CommandWithSplitBare launches program into the requested split type without
// the `gx run` wrapper — for interactive shells, where a non-zero exit is
// normal and a "press Enter" prompt would be meaningless.
func CommandWithSplitBare(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	return commandWithSplit(worktreeRoot, terminal, splitType, program, args, done)
}

func commandWithSplit(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
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
