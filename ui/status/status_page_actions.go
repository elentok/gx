package status

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
)

func (m *Model) openCheckingDivergence() {
	vpW := m.width * 2 / 3
	if vpW < 56 {
		vpW = 56
	}
	if vpW > 110 {
		vpW = 110
	}
	vpH := m.height/2 - 4
	if vpH < 8 {
		vpH = 8
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent("Fetching remote and checking branch divergence…")
	m.runningVP = vp
	m.runningContent = "Fetching remote and checking branch divergence…\n"
	m.runningOpen = true
	m.runningDone = false
	m.runningTitle = "Checking push status"
	m.runningRunner = nil
}

func (m *Model) handleActionResult(res stageActionResult) {
	if strings.TrimSpace(res.output) != "" {
		m.recordCommandOutput(stageActionOutputTitle(res.kind), res.output)
	}
	if res.kind == actionPushFetch {
		m.pendingActionOutput = ""
		if res.err != nil {
			m.showGitError(fmt.Errorf("%w\n%s", res.err, strings.TrimSpace(res.output)))
			m.runningOpen = false
			return
		}
		div, err := git.DetectPushDivergenceAfterFetch(m.worktreeRoot, res.branch)
		if err != nil {
			m.showGitError(err)
			m.runningOpen = false
			return
		}
		if div != nil {
			m.pendingActionOutput = res.output
			m.openConfirm(
				fmt.Sprintf("Branch %s has diverged from the remote branch:", div.Branch),
				[]string{
					"",
					fmt.Sprintf("Last local commit: %s", humanizeOrUnknown(div.Local.Date)),
					fmt.Sprintf("  %s %s", div.Local.Hash, div.Local.Message),
					"",
					fmt.Sprintf("Last remote commit: %s", humanizeOrUnknown(div.RemoteHead.Date)),
					fmt.Sprintf("  %s %s", div.RemoteHead.Hash, div.RemoteHead.Message),
				},
				confirmPushDiverged,
				div.Remote,
				res.branch,
			)
			m.confirmUpstream = div.Upstream
			m.confirmMenu = components.MenuState{
				Items:  []components.MenuItem{{Label: "Rebase", Value: "rebase"}, {Label: "Push --force", Value: "force"}, {Label: "Abort", Value: "abort"}},
				Cursor: 0,
			}
			m.runningOpen = false
			return
		}
		runner := newStageActionRunnerWithOutput(actionPush, m.worktreeRoot, res.remote, res.branch, res.output)
		m.openRunning("Push", runner)
		return
	}
	if res.promptForce {
		m.openConfirm("Force push?", []string{"Push was rejected as non-fast-forward.", "Force push with --force?"}, confirmForcePush, res.remote, res.branch)
		m.runningOpen = false
		return
	}
	if res.promptPopStash {
		a := confirmPullPopStash
		title := "Pop stash after pull failure?"
		if res.kind == actionRebase {
			a = confirmRebasePopStash
			title = "Pop stash after rebase failure?"
		}
		m.openConfirm(title, []string{"The action failed after stashing changes.", "Pop the stash now?"}, a, "", "")
		m.runningOpen = false
		return
	}

	if res.err != nil {
		m.showGitError(fmt.Errorf("%w\n%s", res.err, strings.TrimSpace(res.output)))
		m.runningOpen = false
		return
	}

	switch res.kind {
	case actionPull:
		m.setStatus(ui.MessageComplete("pull"))
	case actionPush:
		m.setStatus(ui.MessageComplete("push"))
		if res.prURL != "" {
			m.openConfirm(
				fmt.Sprintf("Open pull request page?\n\n%s", res.prURL),
				nil,
				confirmOpenPR,
				res.prURL,
				"",
			)
			m.confirmYes = true
		}
	case actionForcePush:
		m.setStatus(ui.MessageComplete("force push"))
	case actionRebase:
		m.setStatus(ui.MessageComplete("rebase"))
	case actionPopStashPull, actionPopStashRebase:
		m.setStatus("stash restored")
	case actionAmend:
		m.setStatus("amended last commit")
	}
	m.runningOpen = false
	m.refresh()
}

func (m Model) confirmAccept() (tea.Model, tea.Cmd) {
	if !m.confirmYes {
		m.confirmOpen = false
		m.confirmAction = confirmNone
		return m, nil
	}
	a := m.confirmAction
	m.confirmOpen = false
	m.confirmAction = confirmNone

	switch a {
	case confirmPush:
		runner := newStageActionRunner(actionPushFetch, m.worktreeRoot, m.confirmRemote, m.confirmBranch)
		m.openRunning("Checking push status", runner)
		return m, actionPollCmd()
	case confirmRebase:
		titleTarget := strings.TrimSpace(m.confirmRemote)
		if titleTarget == "" {
			titleTarget = detectRebaseTarget(m.worktreeRoot)
		}
		runner := newStageActionRunner(actionRebase, m.worktreeRoot, titleTarget, m.confirmBranch)
		m.openRunning("Rebase on "+titleTarget, runner)
		return m, actionPollCmd()
	case confirmForcePush:
		runner := newStageActionRunner(actionForcePush, m.worktreeRoot, m.confirmRemote, m.confirmBranch)
		m.openRunning("Force push", runner)
		return m, actionPollCmd()
	case confirmPullPopStash:
		runner := newStageActionRunner(actionPopStashPull, m.worktreeRoot, "", "")
		m.openRunning("Pop stash", runner)
		return m, actionPollCmd()
	case confirmRebasePopStash:
		runner := newStageActionRunner(actionPopStashRebase, m.worktreeRoot, "", "")
		m.openRunning("Pop stash", runner)
		return m, actionPollCmd()
	case confirmAmend:
		runner := newStageActionRunner(actionAmend, m.worktreeRoot, "", "")
		m.openRunning("Amend commit", runner)
		return m, actionPollCmd()
	case confirmOpenPR:
		m.setStatus(ui.MessageOpening("PR URL"))
		return m, ui.CmdOpenURL(m.confirmRemote)
	case confirmDiscardStatus:
		if m.confirmDiscardUntracked {
			if err := git.DiscardUntrackedPath(m.worktreeRoot, m.confirmPaths[0]); err != nil {
				m.showGitError(err)
				return m, nil
			}
		} else {
			if err := git.RestorePaths(m.worktreeRoot, m.confirmPaths); err != nil {
				m.showGitError(err)
				return m, nil
			}
		}
		m.setStatus("discarded " + m.confirmPaths[0])
		m.reload(m.confirmPaths[0])
		return m, nil
	case confirmDiscardUnstaged:
		if err := git.ApplyPatchToWorktree(m.worktreeRoot, m.confirmPatch, true, m.confirmPatchUnidiffZero); err != nil {
			m.showGitError(err)
			return m, nil
		}
		if m.section == sectionUnstaged {
			m.unstaged.visualActive = false
			m.unstaged.visualAnchor = m.unstaged.activeLine
		}
		m.setStatus("discarded " + m.confirmPaths[0])
		m.reload(m.confirmPaths[0])
		if m.focus == focusDiff {
			m.ensureActiveVisible(m.currentSection())
		}
		return m, nil
	}
	return m, nil
}

func (m Model) startPullAction() (tea.Model, tea.Cmd) {
	runner := newStageActionRunner(actionPull, m.worktreeRoot, "", "")
	m.openRunning("Pull", runner)
	return m, actionPollCmd()
}

func (m *Model) preparePushConfirm() error {
	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil {
		return err
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot push: detached HEAD")
	}
	remote := git.BranchRemote(git.Repo{Root: m.worktreeRoot}, branch)
	m.openConfirm(
		fmt.Sprintf("Push branch %s to %s?", branch, remote),
		nil,
		confirmPush,
		remote,
		branch,
	)
	return nil
}

func humanizeOrUnknown(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return humanize.Time(t)
}

func (m *Model) prepareRebaseConfirm() error {
	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil {
		return err
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot rebase: detached HEAD")
	}
	target := detectRebaseTarget(m.worktreeRoot)
	m.openConfirm(
		fmt.Sprintf("Rebase branch %s on %s?", branch, target),
		nil,
		confirmRebase,
		target,
		branch,
	)
	return nil
}

func detectRebaseTarget(root string) string {
	repo, err := git.FindRepo(root)
	if err != nil {
		return "origin/main"
	}
	main := strings.TrimSpace(repo.MainBranch)
	if main == "" {
		main = "main"
	}
	return "origin/" + main
}

func (m *Model) openAmendConfirm() error {
	subject, files, err := amendPreview(m.worktreeRoot)
	if err != nil {
		return err
	}
	lines := []string{fmt.Sprintf("Commit: %s", subject), "", "Files in last commit:"}
	limit := 10
	if len(files) < limit {
		limit = len(files)
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, "- "+files[i])
	}
	if len(files) > limit {
		lines = append(lines, "...")
	}
	m.openConfirm("Amend last commit?", lines, confirmAmend, "", "")
	return nil
}

func amendPreview(root string) (subject string, files []string, err error) {
	out, _, err := execGitCapture(root, "log", "-1", "--pretty=%s")
	if err != nil {
		return "", nil, err
	}
	nameOut, _, err := execGitCapture(root, "show", "--name-only", "--pretty=format:", "HEAD")
	if err != nil {
		return "", nil, err
	}
	for _, line := range strings.Split(nameOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	return strings.TrimSpace(out), files, nil
}

func execGitCapture(root string, args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return stdout, stderr, &git.RunError{Args: args, Dir: root, Stdout: stdout, Stderr: stderr, Code: ee.ExitCode()}
		}
		return stdout, stderr, err
	}
	return stdout, stderr, nil
}
