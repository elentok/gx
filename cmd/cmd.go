package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/app"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/menu"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/status"
	"github.com/elentok/gx/ui/worktrees"

	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
)

type deps struct {
	stdin                io.Reader
	stdout               io.Writer
	stderr               io.Writer
	getwd                func() (string, error)
	runWorktrees         func(string) error
	runStatus            func(string) error
	confirmForce         func(string) (bool, error)
	choosePushDivergence func(io.Reader, io.Writer, *git.PushDivergence) (int, error)
	initConfig           func() (string, error)
	getenv               func(string) string
	runEditor            func(editor, path string, in io.Reader, out, err io.Writer) error
}

func defaultDeps() deps {
	cfg, _ := config.Load()
	return deps{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		getwd:  os.Getwd,
		confirmForce: func(prompt string) (bool, error) {
			return confirm.RunWithNerd(prompt, cfg.UseNerdFontIcons)
		},
		runWorktrees:         runWorktrees,
		runStatus:            runStatus,
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
		target := ""
		if len(args) > 2 {
			return fmt.Errorf("usage: gx status|s [path]")
		}
		if len(args) == 2 {
			target = args[1]
		}
		return d.runStatus(target)
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
		InputModalBottom: cfg.InputModalBottom,
		NameAliases:      cfg.NameAliases,
		Terminal:         ui.DetectTerminal(),
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteWorktrees},
		ActiveWorktreePath: activeWorktreePath,
		Worktrees:          settings,
		Status: stage.Settings{
			DiffContextLines: cfg.StageDiffContextLines,
			UseNerdFontIcons: cfg.UseNerdFontIcons,
			Terminal:         ui.DetectTerminal(),
			InputModalBottom: cfg.InputModalBottom,
		},
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runStatus(target string) error {
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
	initialPath := ""
	if strings.TrimSpace(target) != "" {
		initialPath, err = resolveStatusTargetPath(root, cwd, target)
		if err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteStatus, WorktreeRoot: root, InitialPath: initialPath},
		ActiveWorktreePath: root,
		Worktrees: worktrees.Settings{
			UseNerdFontIcons: cfg.UseNerdFontIcons,
			InputModalBottom: cfg.InputModalBottom,
			NameAliases:      cfg.NameAliases,
			Terminal:         ui.DetectTerminal(),
		},
		Status: stage.Settings{
			DiffContextLines: cfg.StageDiffContextLines,
			UseNerdFontIcons: cfg.UseNerdFontIcons,
			InitialPath:      initialPath,
			Terminal:         ui.DetectTerminal(),
			InputModalBottom: cfg.InputModalBottom,
		},
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func resolveStatusTargetPath(worktreeRoot, cwd, target string) (string, error) {
	path := filepath.Clean(target)
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	root := filepath.Clean(worktreeRoot)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", fmt.Errorf("status target must be a file inside %s", worktreeRoot)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("status target %q is outside worktree root %s", target, worktreeRoot)
	}
	return filepath.ToSlash(rel), nil
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

	cfg, _ := config.Load()
	nerd := cfg.UseNerdFontIcons

	remote := git.BranchRemote(info.Repo, branch)
	prompt := fmt.Sprintf("Push branch %s to %s?", branch, remote)
	confirmed, err := d.confirmForce(prompt)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("push aborted")
	}

	var div *git.PushDivergence
	fetchLabel := fmt.Sprintf("Fetching %s before checking divergence...", remote)
	printBadge(d.stderr, nerd, fetchLabel, fetchLabel)
	if err := runGitInteractive(pushDir, d.stdin, d.stdout, d.stderr, "fetch", remote); err != nil {
		return err
	}
	checkLabel := fmt.Sprintf("Checking remote divergence for %s...", branch)
	if err := runWithSpinner(d.stdin, d.stderr, checkLabel, func() error {
		var detectErr error
		div, detectErr = git.DetectPushDivergenceAfterFetch(pushDir, branch)
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
			printSuccess(d.stderr, fmt.Sprintf("Rebased %s on %s", branch, div.Upstream))
			return nil
		case 2:
			forceLabel := fmt.Sprintf("Force-pushing %s to %s...", branch, remote)
			printBadge(d.stderr, nerd, forceLabel, forceLabel)
			if err := runGitInteractive(pushDir, d.stdin, d.stdout, d.stderr, "push", "--force", remote, branch); err != nil {
				return err
			}
			printSuccess(d.stderr, fmt.Sprintf("Force-pushed %s to %s with --force", branch, remote))
			return nil
		default:
			return fmt.Errorf("push aborted")
		}
	}

	pushLabel := fmt.Sprintf("Pushing %s to %s...", branch, remote)
	printBadge(d.stderr, nerd, pushLabel, pushLabel)
	if err := runGitInteractive(pushDir, d.stdin, d.stdout, d.stderr, "push", remote, branch); err != nil {
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
		forceLeaseLabel := fmt.Sprintf("Force-pushing %s to %s with lease...", branch, remote)
		printBadge(d.stderr, nerd, forceLeaseLabel, forceLeaseLabel)
		if forceErr := runGitInteractive(pushDir, d.stdin, d.stdout, d.stderr, "push", "--force-with-lease", remote, branch); forceErr != nil {
			prompt := fmt.Sprintf("--force-with-lease failed: %v\nRun plain --force for %s/%s?", forceErr, remote, branch)
			confirmedForce, confirmErr := d.confirmForce(prompt)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmedForce {
				return fmt.Errorf("push aborted after --force-with-lease failure")
			}
			forceLabel := fmt.Sprintf("Force-pushing %s to %s...", branch, remote)
			printBadge(d.stderr, nerd, forceLabel, forceLabel)
			if err := runGitInteractive(pushDir, d.stdin, d.stdout, d.stderr, "push", "--force", remote, branch); err != nil {
				return err
			}
			printSuccess(d.stderr, fmt.Sprintf("Force-pushed %s to %s with --force", branch, remote))
			return nil
		}
		printSuccess(d.stderr, fmt.Sprintf("Force-pushed %s to %s with --force-with-lease", branch, remote))
		return nil
	}

	printSuccess(d.stderr, fmt.Sprintf("Pushed %s to %s", branch, remote))
	return nil
}

func choosePushDivergence(_ io.Reader, _ io.Writer, div *git.PushDivergence) (int, error) {
	if div == nil {
		return 3, nil
	}

	header := fmt.Sprintf(
		"Branch %s has diverged from the remote.\n\n  local   %s  %s %s\n  remote  %s  %s %s",
		div.Branch,
		relativeDate(div.Local.Date), div.Local.Hash, div.Local.Message,
		relativeDate(div.RemoteHead.Date), div.RemoteHead.Hash, div.RemoteHead.Message,
	)

	items := []menu.Item{
		{Label: "Rebase", Detail: fmt.Sprintf("rebase %s onto %s", div.Branch, div.Upstream)},
		{Label: "Force push", Detail: "--force"},
		{Label: "Abort"},
	}

	choice, err := menu.Run(header, items)
	if err != nil {
		return 3, err
	}
	switch choice {
	case 0:
		return 1, nil
	case 1:
		return 2, nil
	default:
		return 3, nil
	}
}

func runGitInteractive(dir string, in io.Reader, out, errOut io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &git.RunError{Args: args, Dir: dir, Code: exitErr.ExitCode()}
		}
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
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
