package terminalrun

import (
	"encoding/json"
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
		// herdr panes always run an interactive shell — there's no split
		// command that execs a program directly (unlike tmux split-window or
		// kitty @ launch). So splitting opens a shell pane, then `pane run`
		// types the command line into it and presses Enter, same as the
		// documented agent-launch flow (see herdr's SKILL.md "Run an ordinary
		// command in another pane").
		var paneID string
		var err error
		switch splitType {
		case HSplit:
			// HSplit (vim :split, stacked) maps to herdr's "down" split.
			paneID, err = herdrPaneSplit(worktreeRoot, "down")
		case VSplit:
			// VSplit (vim :vsplit, side-by-side) maps to herdr's "right" split.
			paneID, err = herdrPaneSplit(worktreeRoot, "right")
		case Tab:
			paneID, err = herdrTabCreate(worktreeRoot)
		}
		if err != nil {
			return "herdr", err
		}
		err = herdrPaneRun(paneID, program, args)
		return "herdr", err
	}
	return "", fmt.Errorf("split not supported for terminal %v", terminal)
}

// herdrPaneSplit splits the calling herdr pane (via --current, i.e.
// $HERDR_PANE_ID) in the given direction ("right"/"down"), focuses the new
// pane, and returns its pane id.
func herdrPaneSplit(worktreeRoot, direction string) (string, error) {
	splitArgs := []string{"pane", "split", "--current", "--direction", direction, "--cwd", worktreeRoot, "--focus"}
	out, err := runCommand("herdr", splitArgs...)
	if err != nil {
		return "", fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(splitArgs, " "), err, strings.TrimSpace(string(out)))
	}
	var resp struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parsing herdr pane split output: %w", err)
	}
	if resp.Result.Pane.PaneID == "" {
		return "", fmt.Errorf("herdr pane split returned no pane id: %s", strings.TrimSpace(string(out)))
	}
	return resp.Result.Pane.PaneID, nil
}

// herdrTabCreate creates a new herdr tab rooted at worktreeRoot, focuses it,
// and returns the pane id of its root pane.
func herdrTabCreate(worktreeRoot string) (string, error) {
	tabArgs := []string{"tab", "create", "--cwd", worktreeRoot, "--focus"}
	out, err := runCommand("herdr", tabArgs...)
	if err != nil {
		return "", fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(tabArgs, " "), err, strings.TrimSpace(string(out)))
	}
	var resp struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parsing herdr tab create output: %w", err)
	}
	if resp.Result.RootPane.PaneID == "" {
		return "", fmt.Errorf("herdr tab create returned no pane id: %s", strings.TrimSpace(string(out)))
	}
	return resp.Result.RootPane.PaneID, nil
}

// herdrPaneRun types program (with args) as a command line into paneID's
// shell and presses Enter, then appends `; exit` so the pane closes once the
// command finishes — matching tmux split-window/kitty @ launch, which exec
// the program directly and close the pane on exit.
func herdrPaneRun(paneID, program string, args []string) error {
	tokens := make([]string, 0, len(args)+1)
	tokens = append(tokens, shellQuote(program))
	for _, arg := range args {
		tokens = append(tokens, shellQuote(arg))
	}
	cmdLine := strings.Join(tokens, " ") + "; exit"
	runArgs := []string{"pane", "run", paneID, cmdLine}
	out, err := runCommand("herdr", runArgs...)
	if err != nil {
		return fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(runArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// shellQuote wraps s in single quotes for safe use as one token in a shell
// command line, escaping any single quotes it contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
