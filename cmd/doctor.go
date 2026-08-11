package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ui"
)

func runDoctor(args []string, d deps) error {
	var fix bool
	var pause bool
	var blockedFormTarget string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--fix":
			fix = true
		case "--pause":
			pause = true
		case "--check-blocked-form":
			i++
			if i >= len(args) {
				return fmt.Errorf("--check-blocked-form requires a pane target")
			}
			blockedFormTarget = args[i]
		default:
			return fmt.Errorf("unknown doctor flag %q", args[i])
		}
	}

	if blockedFormTarget != "" {
		return runDoctorBlockedFormCheck(blockedFormTarget, d)
	}

	getenv := d.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	stdin := d.stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	cwd, err := d.getwd()
	if err != nil {
		return err
	}

	repo, err := git.FindRepo(cwd)
	if err != nil {
		// FindRepo can fail when the outer .git file is itself broken.
		// Fall back to checking whether cwd contains a .bare directory.
		repo, err = findRepoWithFallback(cwd)
		if err != nil {
			return err
		}
	}

	issues, err := git.CheckRepo(*repo)
	if err != nil {
		return err
	}

	printDoctorRuntime(d.stdout, getenv)

	if len(issues) == 0 {
		fmt.Fprintln(d.stdout, "No issues found.")
		if pause {
			pauseDoctor(d.stdout, stdin)
		}
		return nil
	}

	for i, issue := range issues {
		fmt.Fprintf(d.stdout, "[%d/%d] %s\n", i+1, len(issues), issue.Description)

		if !issue.CanFix() {
			fmt.Fprintln(d.stdout, "  No automatic fix available.")
			fmt.Fprintln(d.stdout)
			continue
		}

		if !fix {
			fmt.Fprintf(d.stdout, "  Fix: %s\n", issue.FixDescription)
			fmt.Fprintln(d.stdout)
			continue
		}

		confirmed, err := d.confirmForce(issue.FixDescription + "?")
		if err != nil {
			return err
		}
		if confirmed {
			if err := issue.Fix(); err != nil {
				fmt.Fprintf(d.stderr, "  error: %v\n", err)
			} else {
				fmt.Fprintln(d.stdout, "  Fixed.")
			}
		} else {
			fmt.Fprintln(d.stdout, "  Skipped.")
		}
		fmt.Fprintln(d.stdout)
	}

	if !fix {
		fmt.Fprintln(d.stdout, "Run 'gx doctor --fix' to apply fixes.")
	}

	if pause {
		pauseDoctor(d.stdout, stdin)
	}

	return nil
}

// runDoctorBlockedFormCheck is gx doctor's interactive regression check for
// the orchestrator gate (ralphloop.parkOnBlockedPane): it asks the operator
// to drive target into a live blocked form by hand, then asserts herdr's own
// pane monitor still recognizes it via the "live_blocked_form" rule the gate
// depends on. See docs/runbooks/blocked-form-regression-check.md for the
// full procedure and what to do on failure. It only runs when
// --check-blocked-form is passed explicitly, so it never fires (or fails) in
// a headless `gx doctor` invocation.
func runDoctorBlockedFormCheck(target string, d deps) error {
	stdin := d.stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	explainAgent := d.explainAgent
	if explainAgent == nil {
		explainAgent = herdr.AgentExplain
	}

	fmt.Fprintln(d.stdout, "Interactive check: blocked-form detection")
	fmt.Fprintf(d.stdout, "In pane %q, drive the agent into a form the orchestrator gate must\n", target)
	fmt.Fprintln(d.stdout, "recognize as blocked (see docs/runbooks/blocked-form-regression-check.md).")
	fmt.Fprint(d.stdout, "Once the prompt is showing, press Enter...")
	_, _ = bufio.NewReader(stdin).ReadString('\n')

	result, err := explainAgent(target)
	if err != nil {
		return fmt.Errorf("explaining %s: %w", target, err)
	}

	if result.State != "blocked" || result.MatchedRuleID != "live_blocked_form" {
		fmt.Fprintf(d.stdout, "FAIL: herdr matched rule %q (state %q); want \"live_blocked_form\" (state \"blocked\").\n", result.MatchedRuleID, result.State)
		fmt.Fprintln(d.stdout, "See docs/runbooks/blocked-form-regression-check.md for what to do next.")
		return fmt.Errorf("blocked-form check failed: matched rule %q, not live_blocked_form", result.MatchedRuleID)
	}

	fmt.Fprintln(d.stdout, "PASS: herdr matched live_blocked_form.")
	return nil
}

// findRepoWithFallback tries FindRepo first, then checks for a .bare directory
// in dir (used when the outer .git file is itself corrupted).
func findRepoWithFallback(dir string) (*git.Repo, error) {
	bareDir := filepath.Join(dir, ".bare")
	info, err := os.Stat(bareDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no git repo found at %q", dir)
	}
	repo := &git.Repo{
		Root:        bareDir,
		WorktreeDir: dir,
		IsBare:      true,
		MainBranch:  git.RemoteDefaultBranch(bareDir),
	}
	return repo, nil
}

func printDoctorRuntime(w io.Writer, getenv func(string) string) {
	terminal := ui.DetectTerminalFrom(getenv)
	label := terminal.String()
	if label == "" {
		label = "plain"
	}
	fmt.Fprintf(w, "Runtime:\n")
	fmt.Fprintf(w, "  terminal: %s\n", label)
	fmt.Fprintf(w, "  TMUX=%q\n", getenv("TMUX"))
	fmt.Fprintf(w, "  KITTY_WINDOW_ID=%q\n", getenv("KITTY_WINDOW_ID"))
	fmt.Fprintf(w, "  KITTY_LISTEN_ON=%q\n", getenv("KITTY_LISTEN_ON"))
	fmt.Fprintf(w, "  HERDR_ENV=%q\n\n", getenv("HERDR_ENV"))
}

func pauseDoctor(stdout io.Writer, stdin io.Reader) {
	fmt.Fprint(stdout, "Press Enter to exit...")
	_, _ = bufio.NewReader(stdin).ReadString('\n')
}
