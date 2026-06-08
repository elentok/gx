package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/terminalrun"

	"github.com/spf13/cobra"
)

// termFlags holds the parsed `gx term` direction/cwd flags. The four direction
// flags are mutually exclusive; resolveSplitType enforces that.
type termFlags struct {
	right bool
	below bool
	tab   bool
	here  bool
	cwd   string
}

// launchInSplit and execReplace are seams so tests can assert the launch
// decision (split args / in-place argv) without spawning tmux/kitty or
// replacing the process. execReplace never returns on success — it chdirs and
// hands the process over to the target command.
var (
	launchInSplit = terminalrun.Launch
	execReplace   = func(program string, args []string, cwd string) error {
		bin, err := exec.LookPath(program)
		if err != nil {
			return err
		}
		if cwd != "" {
			if err := os.Chdir(cwd); err != nil {
				return err
			}
		}
		argv := append([]string{program}, args...)
		return syscall.Exec(bin, argv, os.Environ())
	}
)

func newTermCmd(d deps) *cobra.Command {
	var f termFlags
	cmd := &cobra.Command{
		Use:   "term [command...]",
		Short: "launch a command (or shell) into a tmux/kitty split, tab, or in place",
		Long: `Launch a command into a tmux/kitty split or tab, falling back to running
in place when no multiplexer is available.

With no command, opens your $SHELL. Directions (--right/--below/--tab) work on
tmux and kitty (with remote control); elsewhere the command runs in place.

  gx term                  # shell, split below (default)
  gx term --below nvim     # nvim in a split below
  gx term --right lazygit  # lazygit side-by-side
  gx term --tab npm test   # npm test in a new tab
  gx term --here ls        # run in the current terminal`,
		RunE: func(_ *cobra.Command, args []string) error {
			return runTerm(args, f, d)
		},
	}
	// gx's own flags must precede the command; everything from the command name
	// onward (including flag-like tokens) passes through verbatim.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&f.right, "right", false, "open side-by-side (to the right)")
	cmd.Flags().BoolVar(&f.below, "below", false, "open stacked underneath (default)")
	cmd.Flags().BoolVar(&f.tab, "tab", false, "open in a new tab")
	cmd.Flags().BoolVar(&f.here, "here", false, "run in the current terminal (exec-replace)")
	cmd.Flags().StringVar(&f.cwd, "cwd", "", "working directory for the launched command (default: current dir)")
	return cmd
}

// resolveSplitType maps the mutually-exclusive direction flags to a SplitType.
// No flag defaults to --below (HSplit, stacked); more than one is a usage error.
func resolveSplitType(f termFlags) (terminalrun.SplitType, error) {
	st := terminalrun.HSplit // default: --below (stacked)
	n := 0
	if f.right {
		st, n = terminalrun.VSplit, n+1
	}
	if f.below {
		st, n = terminalrun.HSplit, n+1
	}
	if f.tab {
		st, n = terminalrun.Tab, n+1
	}
	if f.here {
		st, n = terminalrun.InPlace, n+1
	}
	if n > 1 {
		return 0, fmt.Errorf("--right, --below, --tab and --here are mutually exclusive")
	}
	return st, nil
}

func runTerm(args []string, f termFlags, d deps) error {
	splitType, err := resolveSplitType(f)
	if err != nil {
		return err
	}

	cwd := f.cwd
	if cwd == "" {
		cwd, err = d.getwd()
		if err != nil {
			return err
		}
	}

	// No command → open $SHELL bare (unwrapped); a shell's exit code is
	// meaningless, so the gx run keep-open prompt would only get in the way.
	// An explicit command is wrapped so a failure keeps the pane open.
	program, pargs, isShell := resolveProgram(args, d)

	terminal := ui.DetectTerminalFrom(d.getenv)

	if splitType != terminalrun.InPlace && terminal.CanSplit() {
		launchProg, launchArgs := program, pargs
		if !isShell {
			launchProg, launchArgs = terminalrun.WrapRun(program, pargs)
		}
		_, err := launchInSplit(cwd, terminal, splitType, launchProg, launchArgs)
		return err
	}

	// In-place fallback (explicit --here, or a split requested on a terminal that
	// can't split). On kitty without remote control we can detect the missed
	// opportunity, so point the user at the fix before handing over the process.
	if splitType != terminalrun.InPlace && terminal == ui.TerminalKitty {
		fmt.Fprintln(d.stderr, "gx: kitty remote control is off — running in place. Enable it with `allow_remote_control yes` and `listen_on` in kitty.conf to use splits.")
	}
	return execReplace(program, pargs, cwd)
}

// resolveProgram picks the program/args to launch: the explicit command if
// given, otherwise $SHELL (falling back to /bin/sh) marked as a shell launch.
func resolveProgram(args []string, d deps) (program string, pargs []string, isShell bool) {
	if len(args) == 0 {
		shell := strings.TrimSpace(d.getenv("SHELL"))
		if shell == "" {
			shell = "/bin/sh"
		}
		return shell, nil, true
	}
	return args[0], args[1:], false
}
