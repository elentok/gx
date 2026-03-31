package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gx/config"
	"gx/git"
	"gx/ui/confirm"
	"gx/ui/stage"
	"gx/ui/worktrees"

	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
)

type deps struct {
	stdin                io.Reader
	stdout               io.Writer
	stderr               io.Writer
	getwd                func() (string, error)
	runWorktrees         func(string) error
	runStatus            func() error
	confirmForce         func(string) (bool, error)
	choosePushDivergence func(io.Reader, io.Writer, *git.PushDivergence) (int, error)
	initConfig           func() (string, error)
	getenv               func(string) string
	runEditor            func(editor, path string, in io.Reader, out, err io.Writer) error
}

func defaultDeps() deps {
	return deps{
		stdin:                os.Stdin,
		stdout:               os.Stdout,
		stderr:               os.Stderr,
		getwd:                os.Getwd,
		runWorktrees:         runWorktrees,
		runStatus:            runStatus,
		confirmForce:         confirm.Run,
		choosePushDivergence: choosePushDivergence,
		initConfig:           config.Init,
		getenv:               os.Getenv,
		runEditor:            runEditorCommand,
	}
}

// Execute runs gx with the provided arguments.
func Execute(args []string) error {
	return execute(args, defaultDeps())
}

func execute(args []string, d deps) error {
	if len(args) == 0 {
		return d.runWorktrees("")
	}

	switch args[0] {
	case "worktrees", "wt":
		return runWorktreeCmd(args[1:], d)
	case "push", "ps":
		return runPush(d)
	case "status", "s":
		return d.runStatus()
	case "init":
		return runInit(d)
	case "edit-config":
		return runEditConfig(d)
	case "bump":
		return runBump(args[1:], d)
	case "stashify":
		return runStashify(args[1:], d)
	case "doctor":
		return runDoctor(args[1:], d)
	case "version", "--version", "-v":
		return runVersion(d.stdout)
	case "-h", "--help", "help":
		printUsage(d.stdout)
		return nil
	default:
		printUsage(d.stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gx [worktrees|wt]            open the worktree UI")
	fmt.Fprintln(w, "  gx wt list                   list worktree names")
	fmt.Fprintln(w, "  gx wt abs-path <name>        print absolute path of a worktree")
	fmt.Fprintln(w, "  gx wt clone <url> [dir]      clone using the .bare trick")
	fmt.Fprintln(w, "  gx push|ps")
	fmt.Fprintln(w, "  gx status|s")
	fmt.Fprintln(w, "  gx init")
	fmt.Fprintln(w, "  gx edit-config")
	fmt.Fprintln(w, "  gx bump [major|minor|patch]  create a version tag and optionally push")
	fmt.Fprintln(w, "  gx stashify <cmd...>         stash, run command, pop")
	fmt.Fprintln(w, "  gx doctor [--fix]")
	fmt.Fprintln(w, "  gx version")
}

func runWorktreeCmd(args []string, d deps) error {
	if len(args) == 0 {
		return d.runWorktrees("")
	}
	switch args[0] {
	case "list":
		return runListWorktrees(d)
	case "abs-path":
		if len(args) < 2 {
			return fmt.Errorf("usage: gx wt abs-path <name>")
		}
		return runWorktreeAbsPath(args[1], d)
	case "clone":
		return runCloneWT(args[1:], d)
	default:
		return fmt.Errorf("unknown wt subcommand %q", args[0])
	}
}

func runWorktrees(_ string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}

	if problem := git.CheckFetchConfig(repo.Root); problem != nil {
		cmdList := strings.Join(problem.Commands, "\n  ")
		prompt := fmt.Sprintf(
			"%s\n\nWorktree statuses may not show correctly without this.\n\nFix by running:\n  %s",
			problem.Description, cmdList,
		)
		confirmed, err := confirm.Run(prompt)
		if err != nil {
			return err
		}
		if confirmed {
			if err := git.FixFetchConfig(repo.Root); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to fix fetch config: %v\n", err)
			}
		}
	}

	// Detect which worktree the user launched from, if any.
	var activeWorktreePath string
	if info, err := git.IdentifyDir(cwd); err == nil {
		activeWorktreePath = info.WorktreeRoot
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	settings := worktrees.Settings{
		UseNerdFontIcons: cfg.UseNerdFontIcons,
	}
	m := worktrees.NewWithSettings(*repo, activeWorktreePath, settings)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runStatus() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx status must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := stage.NewWithSettings(root, stage.Settings{DiffContextLines: cfg.StageDiffContextLines, UseNerdFontIcons: cfg.UseNerdFontIcons})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runCloneWT(args []string, d deps) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: gx wt clone <repo-url> [directory]")
	}

	cwd, err := d.getwd()
	if err != nil {
		return err
	}

	repoURL := args[0]
	target := ""
	if len(args) == 2 {
		target = args[1]
	}

	outerDir, err := git.CloneBare(repoURL, target, cwd)
	if err != nil {
		return err
	}

	repo, err := git.FindRepo(outerDir)
	if err != nil {
		return fmt.Errorf("clone succeeded but could not open repo: %w", err)
	}

	branch := repo.MainBranch
	if branch == "" {
		return fmt.Errorf("unable to determine default branch for %s", outerDir)
	}

	wtPath := filepath.Join(repo.LinkedWorktreeDir(), branch)
	if err := git.AddWorktreeFromRemote(*repo, wtPath, branch, "origin/"+branch); err != nil {
		return fmt.Errorf("clone succeeded but initial worktree creation failed: %w", err)
	}

	fmt.Fprintf(d.stdout, "Cloned to %s and created worktree %s\n", outerDir, wtPath)
	return nil
}

func runListWorktrees(d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}
	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		return err
	}
	for _, wt := range wts {
		fmt.Fprintln(d.stdout, filepath.Base(wt.Path))
	}
	return nil
}

func runWorktreeAbsPath(name string, d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}
	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		return err
	}
	for _, wt := range wts {
		if filepath.Base(wt.Path) == name {
			fmt.Fprintln(d.stdout, wt.Path)
			return nil
		}
	}
	return fmt.Errorf("worktree %q not found", name)
}

func runPush(d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx push must be run from a regular repo or linked worktree")
	}

	pushDir := cwd
	if info.WorktreeRoot != "" {
		pushDir = info.WorktreeRoot
	}

	branch, err := git.CurrentBranch(pushDir)
	if err != nil {
		return err
	}
	if branch == "HEAD" {
		return fmt.Errorf("cannot push from detached HEAD")
	}

	remote := git.BranchRemote(info.Repo, branch)
	var div *git.PushDivergence
	checkLabel := fmt.Sprintf("Checking remote divergence for %s...", branch)
	if err := runWithSpinner(d.stdin, d.stderr, checkLabel, func() error {
		var detectErr error
		div, detectErr = git.DetectPushDivergence(pushDir, branch)
		return detectErr
	}); err != nil {
		return err
	}
	if div != nil {
		chooser := d.choosePushDivergence
		if chooser == nil {
			chooser = choosePushDivergence
		}
		choice, err := chooser(d.stdin, d.stdout, div)
		if err != nil {
			return err
		}
		switch choice {
		case 1:
			rebaseLabel := fmt.Sprintf("Rebasing %s on %s...", branch, div.Upstream)
			if err := runWithSpinner(d.stdin, d.stderr, rebaseLabel, func() error {
				_, err := git.Rebase(pushDir, div.Upstream)
				return err
			}); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Rebased %s on %s\n", branch, div.Upstream)
			return nil
		case 2:
			forceLabel := fmt.Sprintf("Force-pushing %s to %s...", branch, remote)
			if err := runWithSpinner(d.stdin, d.stderr, forceLabel, func() error {
				_, err := git.PushBranchForce(pushDir, remote, branch)
				return err
			}); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Force-pushed %s to %s with --force\n", branch, remote)
			return nil
		default:
			return fmt.Errorf("push aborted")
		}
	}

	pushLabel := fmt.Sprintf("Pushing %s to %s...", branch, remote)
	if err := runWithSpinner(d.stdin, d.stderr, pushLabel, func() error {
		_, _, err := git.PushBranch(pushDir, remote, branch)
		return err
	}); err != nil {
		if !git.IsNonFastForwardPushError(err) {
			return err
		}

		prompt := fmt.Sprintf("Push rejected for %s/%s. Force push with lease?", remote, branch)
		confirmed, confirmErr := d.confirmForce(prompt)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			return fmt.Errorf("push aborted")
		}
		forceLabel := fmt.Sprintf("Force-pushing %s to %s with lease...", branch, remote)
		if forceErr := runWithSpinner(d.stdin, d.stderr, forceLabel, func() error {
			return git.PushBranchForceWithLease(pushDir, remote, branch)
		}); forceErr != nil {
			prompt := fmt.Sprintf("--force-with-lease failed: %v\nRun plain --force for %s/%s?", forceErr, remote, branch)
			confirmedForce, confirmErr := d.confirmForce(prompt)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmedForce {
				return fmt.Errorf("push aborted after --force-with-lease failure")
			}
			forceLabel = fmt.Sprintf("Force-pushing %s to %s...", branch, remote)
			if err := runWithSpinner(d.stdin, d.stderr, forceLabel, func() error {
				_, err := git.PushBranchForce(pushDir, remote, branch)
				return err
			}); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Force-pushed %s to %s with --force\n", branch, remote)
			return nil
		}
		fmt.Fprintf(d.stdout, "Force-pushed %s to %s with --force-with-lease\n", branch, remote)
		return nil
	}

	fmt.Fprintf(d.stdout, "Pushed %s to %s\n", branch, remote)
	return nil
}

func choosePushDivergence(in io.Reader, out io.Writer, div *git.PushDivergence) (int, error) {
	if div == nil {
		return 3, nil
	}
	fmt.Fprintf(out, "Branch %s has diverged from the remote branch:\n\n", div.Branch)
	fmt.Fprintf(out, "Last local commit: %s\n", relativeDate(div.Local.Date))
	fmt.Fprintf(out, "  %s %s\n\n", div.Local.Hash, div.Local.Message)
	fmt.Fprintf(out, "Last remote commit: %s\n", relativeDate(div.RemoteHead.Date))
	fmt.Fprintf(out, "  %s %s\n\n", div.RemoteHead.Hash, div.RemoteHead.Message)
	fmt.Fprintln(out, "Choose an option:")
	fmt.Fprintln(out, "1. Rebase")
	fmt.Fprintln(out, "2. Push --force")
	fmt.Fprintln(out, "3. Abort")
	fmt.Fprint(out, "> ")

	r := bufio.NewReader(in)
	line, err := r.ReadString('\n')
	if err != nil && len(line) == 0 {
		return 3, err
	}
	line = strings.TrimSpace(line)
	switch line {
	case "1":
		return 1, nil
	case "2":
		return 2, nil
	default:
		return 3, nil
	}
}

func relativeDate(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return humanize.Time(t)
}

func runInit(d deps) error {
	path, err := d.initConfig()
	if err != nil {
		return err
	}
	fmt.Fprintf(d.stdout, "Created config file at %s\n", path)
	return nil
}

func runEditConfig(d deps) error {
	path, err := config.FilePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		createdPath, initErr := d.initConfig()
		if initErr != nil {
			return initErr
		}
		fmt.Fprintf(d.stdout, "Created config file at %s\n", createdPath)
	} else if err != nil {
		return err
	}

	editor := d.getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		return fmt.Errorf("$EDITOR is not set")
	}
	return d.runEditor(editor, path, d.stdin, d.stdout, d.stderr)
}

func runEditorCommand(editor, path string, in io.Reader, out, errOut io.Writer) error {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("$EDITOR is empty")
	}
	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", editor, err)
	}
	return nil
}
