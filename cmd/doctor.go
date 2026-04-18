package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

func runDoctor(args []string, d deps) error {
	var fix bool
	var pause bool
	for _, arg := range args {
		switch arg {
		case "--fix":
			fix = true
		case "--pause":
			pause = true
		default:
			return fmt.Errorf("unknown doctor flag %q", arg)
		}
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

func printDoctorUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gx doctor [--fix] [--pause]")
	fmt.Fprintln(w, "  Checks the current repo for common configuration issues.")
	fmt.Fprintln(w, "  --fix    Prompt to apply each fix interactively.")
	fmt.Fprintln(w, "  --pause  Wait for Enter before exiting.")
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
	fmt.Fprintf(w, "  KITTY_LISTEN_ON=%q\n\n", getenv("KITTY_LISTEN_ON"))
}

func pauseDoctor(stdout io.Writer, stdin io.Reader) {
	fmt.Fprint(stdout, "Press Enter to exit...")
	_, _ = bufio.NewReader(stdin).ReadString('\n')
}
