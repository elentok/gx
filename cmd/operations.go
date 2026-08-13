package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
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

func runConfigShow(d deps) error {
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.stdout, "%s\n", b)
	return nil
}

func runConfigDefaults(d deps) error {
	b, err := json.MarshalIndent(config.Default(), "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.stdout, "%s\n", b)
	return nil
}

// runConfigTestNotifications sends a fixed test message to every notification
// service that has credentials configured, reporting each one's outcome as
// it goes rather than failing fast, so a Slack success is still visible even
// if Telegram is misconfigured.
func runConfigTestNotifications(d deps) error {
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}

	configured := 0
	failed := 0

	if cfg.Notifications.Telegram.BotToken != "" {
		configured++
		if err := ralphloop.SendTelegramTestMessage(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID); err != nil {
			failed++
			fmt.Fprintf(d.stderr, "telegram: failed: %v\n", err)
		} else {
			fmt.Fprintf(d.stdout, "telegram: sent\n")
		}
	}

	if cfg.Notifications.Slack.WebhookURL != "" {
		configured++
		if err := ralphloop.SendSlackTestMessage(cfg.Notifications.Slack.WebhookURL); err != nil {
			failed++
			fmt.Fprintf(d.stderr, "slack: failed: %v\n", err)
		} else {
			fmt.Fprintf(d.stdout, "slack: sent\n")
		}
	}

	if configured == 0 {
		fmt.Fprintf(d.stdout, "no notification service configured (see `gx config edit`)\n")
		return nil
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d notification(s) failed to send", failed, configured)
	}
	return nil
}

// notifyTransports is the ordered set of transport names --enable/--disable/
// --status operate on, matching ralphloop.NotificationState's keys and
// SendMessage's sent-to strings.
var notifyTransports = []string{"telegram", "slack"}

func isKnownNotifyTransport(name string) bool {
	return slices.Contains(notifyTransports, name)
}

// runNotify dispatches `gx notify` to whichever of its mutually exclusive
// modes was requested: send a message, flip a transport's manual mute, or
// report status. Args holds at most one positional message (Args:
// cobra.MaximumNArgs(1)).
func runNotify(args []string, enable, disable string, status bool, d deps) error {
	hasMessage := len(args) == 1
	controlFlags := 0
	if enable != "" {
		controlFlags++
	}
	if disable != "" {
		controlFlags++
	}
	if status {
		controlFlags++
	}

	switch {
	case hasMessage && controlFlags > 0:
		return fmt.Errorf("a message and --enable/--disable/--status are mutually exclusive")
	case controlFlags > 1:
		return fmt.Errorf("--enable, --disable, and --status are mutually exclusive")
	case enable != "":
		return runNotifySetMute(enable, false, d)
	case disable != "":
		return runNotifySetMute(disable, true, d)
	case status:
		return runNotifyStatus(d)
	case hasMessage:
		return sendNotify(args[0], d)
	default:
		return fmt.Errorf("notify requires a message or one of --enable/--disable/--status")
	}
}

// sendNotify sends text to every notification service configured in
// ~/.config/gx/config.json (Telegram and/or Slack), no-op'ing per-service
// when unconfigured, and reports which service(s) it sent to.
func sendNotify(text string, d deps) error {
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}

	sent, err := ralphloop.SendMessage(cfg.Notifications, text)
	if len(sent) == 0 {
		if err == nil {
			fmt.Fprintf(d.stdout, "no notification service configured (see `gx config edit`)\n")
		}
		return err
	}
	fmt.Fprintf(d.stdout, "sent to: %s\n", strings.Join(sent, ", "))
	return err
}

// runNotifySetMute flips transport's manual mute in the notification state
// file (ticket 02's module) and reports the new state. This is the same
// manual path a planned quiet period would use.
func runNotifySetMute(transport string, muted bool, d deps) error {
	if !isKnownNotifyTransport(transport) {
		return fmt.Errorf("unknown transport %q (expected telegram or slack)", transport)
	}

	err := ralphloop.UpdateNotificationState(func(state *ralphloop.NotificationState) {
		ts := state.Transports[transport]
		ts.Muted = muted
		if muted {
			ts.Reason = "manual-disable"
			ts.TrippedAt = time.Now()
		} else {
			ts.Reason = ""
			ts.TrippedAt = time.Time{}
		}
		state.Transports[transport] = ts
	})
	if err != nil {
		return err
	}

	if muted {
		fmt.Fprintf(d.stdout, "%s: muted\n", transport)
	} else {
		fmt.Fprintf(d.stdout, "%s: active\n", transport)
	}
	return nil
}

// runNotifyStatus reports every transport's mute state plus every ticket
// under the current repo's tracker root carrying a non-empty Mutes.
func runNotifyStatus(d deps) error {
	state, err := ralphloop.LoadNotificationState()
	if err != nil {
		return err
	}
	for _, transport := range notifyTransports {
		ts := state.Transports[transport]
		if ts.Muted {
			fmt.Fprintf(d.stdout, "%s: muted (%s)\n", transport, ts.Reason)
		} else {
			fmt.Fprintf(d.stdout, "%s: active\n", transport)
		}
	}

	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	epics, err := tickets.Load(info.Repo.ScratchRoot())
	if err != nil {
		return err
	}

	var muted []string
	for _, epic := range epics {
		for _, t := range epic.Tickets {
			for _, m := range t.Mutes {
				muted = append(muted, fmt.Sprintf("%s/%s: %s (tripped %s)",
					epic.Name, t.DisplayNumber(), m.EventType, m.TrippedAt.Format(time.RFC3339)))
			}
		}
	}

	if len(muted) == 0 {
		fmt.Fprintf(d.stdout, "\nno tickets with active mutes\n")
		return nil
	}
	fmt.Fprintf(d.stdout, "\nmuted tickets:\n")
	for _, line := range muted {
		fmt.Fprintf(d.stdout, "  %s\n", line)
	}
	return nil
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
