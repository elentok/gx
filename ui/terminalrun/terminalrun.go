package terminalrun

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type SplitType int

// SplitType names follow vim's terminology, not tmux's:
//   - HSplit = horizontal split (vim :split / <c-w>s) = stacked top/bottom.
//   - VSplit = vertical split (vim :vsplit / <c-w>v) = side-by-side left/right.
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
	if terminal == ui.TerminalHerdr {
		return func() tea.Msg {
			_, err := launchSplit(worktreeRoot, terminal, HSplit, program, args)
			return done(err, "herdr")
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
			// HSplit = horizontal split (vim :split) = stacked top/bottom.
			tmuxArgs = []string{"split-window", "-v", "-c", worktreeRoot, program}
		case VSplit:
			// VSplit = vertical split (vim :vsplit) = side-by-side left/right.
			tmuxArgs = []string{"split-window", "-h", "-c", worktreeRoot, program}
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
			// HSplit = horizontal split (vim :split) = stacked top/bottom.
			// kitty's "hsplit" produces top/bottom panes, matching tmux -v.
			typeAndLoc = []string{"--type=window", "--location=hsplit"}
		case VSplit:
			// VSplit = vertical split (vim :vsplit) = side-by-side left/right.
			// kitty's "vsplit" produces left/right panes, matching tmux -h.
			typeAndLoc = []string{"--type=window", "--location=vsplit"}
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
	case ui.TerminalHerdr:
		// `herdr agent start` is documented for launching coding agents, but
		// `pane split`/`tab create` (the commands meant for ordinary programs)
		// don't accept a program to exec — they only open a pane running an
		// interactive shell, which `pane run` then has to type a command line
		// into. That adds a visible delay waiting for the shell (and its
		// prompt) to finish starting up before it can process input. `agent
		// start` directly execs the given argv (same as tmux split-window/kitty
		// @ launch) and, when the process exits cleanly, closes the pane
		// automatically — so it's used here for any program, not just agents.
		name := filepath.Base(program)
		herdrArgs := []string{"agent", "start", name}
		switch splitType {
		case HSplit:
			// HSplit (vim :split, stacked) maps to herdr's "down" split.
			herdrArgs = append(herdrArgs, "--cwd", worktreeRoot, "--split", "down")
		case VSplit:
			// VSplit (vim :vsplit, side-by-side) maps to herdr's "right" split.
			herdrArgs = append(herdrArgs, "--cwd", worktreeRoot, "--split", "right")
		case Tab:
			tabID, err := herdrTabCreate(worktreeRoot)
			if err != nil {
				return "herdr", err
			}
			herdrArgs = append(herdrArgs, "--tab", tabID)
		}
		herdrArgs = append(herdrArgs, "--focus", "--")
		herdrArgs = append(herdrArgs, program)
		herdrArgs = append(herdrArgs, args...)
		out, err := runCommand("herdr", herdrArgs...)
		if err != nil {
			err = fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(herdrArgs, " "), err, strings.TrimSpace(string(out)))
		}
		return "herdr", err
	}
	return "", fmt.Errorf("split not supported for terminal %v", terminal)
}

// herdrTabCreate creates a new herdr tab rooted at worktreeRoot and returns
// its tab id, for use with `herdr agent start --tab <id>` (herdr's
// tab-create step doesn't accept a program to launch, unlike tmux
// new-window/kitty --type=tab).
func herdrTabCreate(worktreeRoot string) (string, error) {
	tabArgs := []string{"tab", "create", "--cwd", worktreeRoot}
	out, err := runCommand("herdr", tabArgs...)
	if err != nil {
		return "", fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(tabArgs, " "), err, strings.TrimSpace(string(out)))
	}
	var resp struct {
		Result struct {
			Tab struct {
				TabID string `json:"tab_id"`
			} `json:"tab"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parsing herdr tab create output: %w", err)
	}
	if resp.Result.Tab.TabID == "" {
		return "", fmt.Errorf("herdr tab create returned no tab id: %s", strings.TrimSpace(string(out)))
	}
	return resp.Result.Tab.TabID, nil
}
