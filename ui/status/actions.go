package stage

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gx/git"
	"gx/ui"
	"gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
)

type stageConfirmAction int

const (
	confirmNone stageConfirmAction = iota
	confirmPush
	confirmRebase
	confirmForcePush
	confirmPullPopStash
	confirmRebasePopStash
	confirmAmend
	confirmOpenPR
	confirmDiscardStatus
	confirmDiscardUnstaged
	confirmPushDiverged
)

type stageActionKind int

const (
	actionPull stageActionKind = iota
	actionPush
	actionForcePush
	actionRebase
	actionPopStashPull
	actionPopStashRebase
	actionAmend
)

type stageActionResult struct {
	kind           stageActionKind
	err            error
	output         string
	promptForce    bool
	promptPopStash bool
	remote         string
	branch         string
	prURL          string
}

type stageActionRunner struct {
	kind stageActionKind
	root string

	remote string
	branch string

	runner *components.CommandRunner
	done   chan stageActionResult
	res    stageActionResult
	ok     bool
}

func newStageActionRunner(kind stageActionKind, root, remote, branch string) *stageActionRunner {
	return &stageActionRunner{
		kind:   kind,
		root:   root,
		remote: remote,
		branch: branch,
		runner: components.NewCommandRunner(root, "git"),
		done:   make(chan stageActionResult, 1),
	}
}

func (r *stageActionRunner) Start() {
	go func() {
		res := r.run()
		res.output = r.runner.Output()
		r.done <- res
	}()
}

func (r *stageActionRunner) Cancel() {
	r.runner.Cancel()
}

func (r *stageActionRunner) Consume() string {
	return r.runner.Consume()
}

func (r *stageActionRunner) Result() (stageActionResult, bool) {
	if r.ok {
		return r.res, true
	}
	select {
	case res := <-r.done:
		r.res = res
		r.ok = true
		return res, true
	default:
		return stageActionResult{}, false
	}
}

func (r *stageActionRunner) run() stageActionResult {
	switch r.kind {
	case actionPull:
		return r.runPullLike(actionPull)
	case actionRebase:
		return r.runPullLike(actionRebase)
	case actionPush:
		res := stageActionResult{kind: r.kind, remote: r.remote, branch: r.branch}
		if err := r.execGit("push", r.remote, r.branch); err != nil {
			res.err = err
			if git.IsNonFastForwardPushError(err) {
				res.promptForce = true
				res.err = nil
			}
		} else {
			res.prURL = git.ExtractPRURL(r.runner.Output())
		}
		return res
	case actionForcePush:
		res := stageActionResult{kind: r.kind}
		res.err = r.execGit("push", "--force", r.remote, r.branch)
		return res
	case actionPopStashPull:
		res := stageActionResult{kind: r.kind}
		res.err = r.execGit("stash", "pop")
		return res
	case actionPopStashRebase:
		res := stageActionResult{kind: r.kind}
		res.err = r.execGit("stash", "pop")
		return res
	case actionAmend:
		res := stageActionResult{kind: r.kind}
		res.err = r.execGit("commit", "--amend", "--no-edit")
		return res
	default:
		return stageActionResult{kind: r.kind, err: fmt.Errorf("unsupported action")}
	}
}

func (r *stageActionRunner) runPullLike(kind stageActionKind) stageActionResult {
	res := stageActionResult{kind: kind}
	changes, err := git.UncommittedChanges(r.root)
	if err != nil {
		res.err = err
		return res
	}
	stashed := len(changes) > 0
	if stashed {
		if err := r.execGit("stash", "push", "-u", "-m", "gx-stage-auto-stash"); err != nil {
			res.err = fmt.Errorf("stash failed: %w", err)
			return res
		}
	}

	if kind == actionRebase {
		target := strings.TrimSpace(r.remote)
		if target == "" {
			target = detectRebaseTarget(r.root)
		}
		fetchRemote := "origin"
		if i := strings.Index(target, "/"); i > 0 {
			fetchRemote = strings.TrimSpace(target[:i])
		}
		if err := r.execGit("fetch", fetchRemote); err != nil {
			res.err = err
			if stashed {
				res.promptPopStash = true
			}
			return res
		}
		if err := r.execGit("rebase", target); err != nil {
			res.err = err
			if stashed {
				res.promptPopStash = true
			}
			return res
		}
	} else {
		if err := r.execGit("pull"); err != nil {
			res.err = err
			if stashed {
				res.promptPopStash = true
			}
			return res
		}
	}

	if stashed {
		if err := r.execGit("stash", "pop"); err != nil {
			res.err = err
			return res
		}
	}
	return res
}

func (r *stageActionRunner) execGit(args ...string) error {
	r.runner = components.NewCommandRunner(r.root, "git", args...)
	r.runner.Start()
	err := r.runner.Wait()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return &git.RunError{Args: args, Dir: r.root, Stdout: strings.TrimSpace(r.runner.Output()), Stderr: strings.TrimSpace(ee.Error()), Code: ee.ExitCode()}
		}
		return err
	}
	return nil
}

func actionPollCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return actionPollMsg{}
	})
}

func cmdPushPreflight(root string) tea.Cmd {
	return func() tea.Msg {
		branch, err := git.CurrentBranch(root)
		if err != nil {
			return pushPreflightMsg{err: err}
		}
		if branch == "" || branch == "HEAD" {
			return pushPreflightMsg{err: fmt.Errorf("cannot push: detached HEAD")}
		}
		remote := git.BranchRemote(git.Repo{Root: root}, branch)
		div, err := git.DetectPushDivergence(root, branch)
		if err != nil {
			return pushPreflightMsg{err: err}
		}
		return pushPreflightMsg{branch: branch, remote: remote, divergence: div}
	}
}

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

func (m *Model) appendRunningOutput(chunk string) {
	if chunk == "" {
		return
	}
	m.runningContent += chunk
	m.runningVP.SetContent(m.runningContent)
	m.runningVP.GotoBottom()
}

func (m *Model) handleActionResult(res stageActionResult) {
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
		m.setStatus("pull complete")
	case actionPush:
		m.setStatus("push complete")
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
		m.setStatus("force push complete")
	case actionRebase:
		m.setStatus("rebase complete")
	case actionPopStashPull, actionPopStashRebase:
		m.setStatus("stash restored")
	case actionAmend:
		m.setStatus("amended last commit")
	}
	m.runningOpen = false
	m.refresh()
}

func (m *Model) handleRunningKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.runningDone {
		switch msg.String() {
		case "esc", "enter":
			m.runningOpen = false
			m.runningRunner = nil
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.runningVP, cmd = m.runningVP.Update(msg)
	return m, cmd
}

func (m *Model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmAction == confirmPushDiverged {
		next, decided, accepted, handled := components.UpdateMenu(msg, m.confirmMenu)
		if !handled {
			return m, nil
		}
		m.confirmMenu = next
		if !decided {
			return m, nil
		}
		if !accepted {
			m.confirmOpen = false
			m.confirmAction = confirmNone
			m.setStatus("push aborted")
			return m, nil
		}
		choice := "abort"
		if len(m.confirmMenu.Items) > 0 && m.confirmMenu.Cursor >= 0 && m.confirmMenu.Cursor < len(m.confirmMenu.Items) {
			choice = m.confirmMenu.Items[m.confirmMenu.Cursor].Value
		}
		switch choice {
		case "rebase":
			m.confirmOpen = false
			m.confirmAction = confirmNone
			target := strings.TrimSpace(m.confirmUpstream)
			if target == "" {
				target = strings.TrimSpace(m.confirmRemote)
			}
			runner := newStageActionRunner(actionRebase, m.worktreeRoot, target, m.confirmBranch)
			m.openRunning("Rebase on "+target, runner)
			return m, actionPollCmd()
		case "force":
			m.confirmOpen = false
			m.confirmAction = confirmNone
			runner := newStageActionRunner(actionForcePush, m.worktreeRoot, m.confirmRemote, m.confirmBranch)
			m.openRunning("Force push", runner)
			return m, actionPollCmd()
		default:
			m.confirmOpen = false
			m.confirmAction = confirmNone
			m.setStatus("push aborted")
			return m, nil
		}
	}

	nextYes, decided, accepted, handled := components.UpdateConfirm(msg, m.confirmYes)
	if !handled {
		return m, nil
	}
	m.confirmYes = nextYes
	if !decided {
		return m, nil
	}
	if accepted {
		return m.confirmAccept()
	}
	m.confirmOpen = false
	m.confirmAction = confirmNone
	return m, nil
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
		m.openCheckingDivergence()
		return m, cmdPushPreflight(m.worktreeRoot)
	case confirmRebase:
		runner := newStageActionRunner(actionRebase, m.worktreeRoot, m.confirmRemote, m.confirmBranch)
		titleTarget := strings.TrimSpace(m.confirmRemote)
		if titleTarget == "" {
			titleTarget = detectRebaseTarget(m.worktreeRoot)
		}
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
		m.setStatus("opening PR URL")
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

func (m *Model) openRunning(title string, runner *stageActionRunner) {
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
	vp.SetContent("")
	m.runningVP = vp
	m.runningContent = ""
	m.runningOpen = true
	m.runningDone = false
	m.runningTitle = title
	m.runningRunner = runner
	runner.Start()
}

func (m *Model) openConfirm(title string, lines []string, action stageConfirmAction, remote, branch string) {
	m.confirmOpen = true
	m.confirmTitle = title
	m.confirmLines = append([]string{}, lines...)
	m.confirmAction = action
	m.confirmRemote = remote
	m.confirmUpstream = ""
	m.confirmBranch = branch
	m.confirmPaths = nil
	m.confirmPatch = ""
	m.confirmPatchUnidiffZero = false
	m.confirmDiscardUntracked = false
	m.confirmMenu = components.MenuState{}
	m.confirmYes = true
}

func (m *Model) startPullAction() {
	m.openRunning("Pull", newStageActionRunner(actionPull, m.worktreeRoot, "", ""))
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

func (m Model) runningModalView() string {
	title := m.runningTitle
	if title == "" {
		title = "Running"
	}
	hint := "ctrl+c cancel · j/k scroll"
	if m.runningDone {
		hint = "esc / enter dismiss · j/k scroll"
	}
	return components.RenderOutputModal(
		title,
		m.runningVP.View(),
		hint,
		catYellow,
		catYellow,
		catSubtle,
		m.runningVP.Width(),
	)
}

func (m Model) confirmModalView() string {
	prompt := m.confirmTitle
	if len(m.confirmLines) > 0 {
		prompt = prompt + "\n\n" + strings.Join(m.confirmLines, "\n")
	}
	if m.confirmAction == confirmPushDiverged {
		return components.RenderMenuModal(
			"Push Diverged",
			prompt,
			m.confirmMenu,
			"j/k or ↑/↓ navigate · enter select · esc cancel",
			catYellow,
			catYellow,
			catSubtle,
			catGreen,
			maxInt(56, m.width/2),
		)
	}
	return components.RenderConfirmModal(
		prompt,
		m.confirmYes,
		catYellow,
		catGreen,
		catRed,
		catSubtle,
		maxInt(56, m.width/2),
	)
}
