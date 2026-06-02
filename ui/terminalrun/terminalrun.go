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

// runCommand runs an external command and returns its combined output and exit
// error. It is a seam so tests can assert the exact tmux/kitty argument vectors
// launchSplit builds without spawning a real multiplexer.
var runCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

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

// WrapRun rewrites a command launch so it runs under `gx run`, which keeps the
// pane open (showing the failed command and a prompt) only when the command
// fails. Interactive shells must NOT be wrapped — their exit code is
// meaningless, so they use the *Bare variants. Returns the gx path and the
// rewritten argv.
func WrapRun(program string, args []string) (string, []string) {
	wrapped := make([]string, 0, len(args)+2)
	wrapped = append(wrapped, "run", program)
	wrapped = append(wrapped, args...)
	return gxPath(), wrapped
}

// Command launches a command into the default split (vertical for tmux,
// hsplit for kitty) or in place on a plain terminal, wrapped in `gx run` so a
// failure keeps the pane open.
func Command(worktreeRoot string, terminal ui.Terminal, program string, args []string, done func(err error, splitApp string) tea.Msg) tea.Cmd {
	program, args = WrapRun(program, args)

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
	program, args = WrapRun(program, args)
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

	if terminal.CanSplit() {
		return func() tea.Msg {
			splitApp, err := launchSplit(worktreeRoot, terminal, splitType, program, args)
			return done(err, splitApp)
		}
	}

	return notify.Warning("split not supported for this terminal")
}

// Launch synchronously launches program (with args) into the requested
// split/tab and returns the short app label ("tmux"/"kitty") plus any launch
// error. It is the entry point for callers outside Bubbletea (the `gx term`
// CLI), running the same per-terminal logic the TUI drives through
// CommandWithSplit. splitType must not be InPlace, and terminal must satisfy
// CanSplit(); the caller owns the in-place / non-splittable fallback.
func Launch(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string) (string, error) {
	return launchSplit(worktreeRoot, terminal, splitType, program, args)
}

// launchSplit builds the tmux/kitty argument vector for splitType and runs it,
// returning the app label and (for kitty) a formatted error including the
// command and its output. It is synchronous; the TUI wraps it in a tea.Cmd
// closure and the CLI calls it directly.
func launchSplit(worktreeRoot string, terminal ui.Terminal, splitType SplitType, program string, args []string) (string, error) {
	switch terminal {
	case ui.TerminalTmux:
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
		_, err := runCommand("tmux", tmuxArgs...)
		return "tmux", err
	case ui.TerminalKittyRemote:
		var typeAndLoc []string
		switch splitType {
		case HSplit:
			// HSplit = side-by-side. kitty's "vsplit" produces left/right panes
			// (the opposite of its "hsplit"), matching tmux's split-window -h.
			typeAndLoc = []string{"--type=window", "--location=vsplit"}
		case VSplit:
			// VSplit = stacked. kitty's "hsplit" produces top/bottom panes,
			// matching tmux's split-window -v.
			typeAndLoc = []string{"--type=window", "--location=hsplit"}
		case Tab:
			typeAndLoc = []string{"--type=tab"}
		}
		kittyArgs := []string{"@", "launch", "--copy-env"}
		kittyArgs = append(kittyArgs, typeAndLoc...)
		kittyArgs = append(kittyArgs, "--cwd="+worktreeRoot, program)
		kittyArgs = append(kittyArgs, args...)
		out, err := runCommand("kitty", kittyArgs...)
		if err != nil {
			err = fmt.Errorf("$ kitty %s\n\n%w\n\n%s", strings.Join(kittyArgs, " "), err, strings.TrimSpace(string(out)))
		}
		return "kitty", err
	}
	return "", fmt.Errorf("split not supported for terminal %v", terminal)
}
