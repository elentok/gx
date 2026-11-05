package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/menu"

	humanize "github.com/dustin/go-humanize"
)

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

	// No project registers a fresh clone yet, so its default branch is the
	// only trunk there is.
	branch := git.RemoteDefaultBranch(repo.Root)

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

	cfg, _ := d.dirs().Load()
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
	// The plain process env, not NonInteractiveEnv: fetch and push may prompt for credentials.
	_, _, err := git.RunWith(dir, os.Environ(), git.RunOptions{Stdin: in, Stdout: out, Stderr: errOut}, args...)
	return err
}

func relativeDate(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return humanize.Time(t)
}
