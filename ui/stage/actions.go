package stage

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gx/git"
	"gx/ui"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	mu      sync.Mutex
	output  bytes.Buffer
	readPos int

	cmd  *exec.Cmd
	done chan stageActionResult
	res  stageActionResult
	ok   bool
}

func newStageActionRunner(kind stageActionKind, root, remote, branch string) *stageActionRunner {
	return &stageActionRunner{
		kind:   kind,
		root:   root,
		remote: remote,
		branch: branch,
		done:   make(chan stageActionResult, 1),
	}
}

func (r *stageActionRunner) Start() {
	go func() {
		res := r.run()
		r.mu.Lock()
		res.output = r.output.String()
		r.mu.Unlock()
		r.done <- res
	}()
}

func (r *stageActionRunner) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
}

func (r *stageActionRunner) Consume() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.output.String()
	if r.readPos >= len(s) {
		return ""
	}
	chunk := s[r.readPos:]
	r.readPos = len(s)
	return chunk
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
			r.mu.Lock()
			res.prURL = git.ExtractPRURL(r.output.String())
			r.mu.Unlock()
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
		if err := r.execGit("fetch", "origin"); err != nil {
			res.err = err
			if stashed {
				res.promptPopStash = true
			}
			return res
		}
		if err := r.execGit("rebase", "origin/master"); err != nil {
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
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.cmd = cmd
	r.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.copyOutput(stdout)
	}()
	go func() {
		defer wg.Done()
		r.copyOutput(stderr)
	}()

	err = cmd.Wait()
	wg.Wait()

	r.mu.Lock()
	r.cmd = nil
	r.mu.Unlock()

	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return &git.RunError{Args: args, Dir: r.root, Stdout: "", Stderr: strings.TrimSpace(ee.Error()), Code: ee.ExitCode()}
		}
		return err
	}
	return nil
}

func (r *stageActionRunner) copyOutput(src io.Reader) {
	buf := make([]byte, 2048)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			r.mu.Lock()
			r.output.Write(buf[:n])
			r.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func actionPollCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return actionPollMsg{}
	})
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
	switch msg.String() {
	case "esc":
		m.confirmOpen = false
		m.confirmAction = confirmNone
		return m, nil
	case "left", "h":
		m.confirmYes = true
	case "right", "l":
		m.confirmYes = false
	case "y":
		m.confirmYes = true
		return m.confirmAccept()
	case "n":
		m.confirmYes = false
		m.confirmOpen = false
		m.confirmAction = confirmNone
		return m, nil
	case "enter":
		return m.confirmAccept()
	}
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
		runner := newStageActionRunner(actionPush, m.worktreeRoot, m.confirmRemote, m.confirmBranch)
		m.openRunning("Push", runner)
		return m, actionPollCmd()
	case confirmRebase:
		runner := newStageActionRunner(actionRebase, m.worktreeRoot, "", m.confirmBranch)
		m.openRunning("Rebase on origin/master", runner)
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
	m.confirmBranch = branch
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

func (m *Model) prepareRebaseConfirm() error {
	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil {
		return err
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot rebase: detached HEAD")
	}
	m.openConfirm(
		fmt.Sprintf("Rebase branch %s on origin/master?", branch),
		nil,
		confirmRebase,
		"",
		branch,
	)
	return nil
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
	titleStyle := lipgloss.NewStyle().Foreground(catYellow).Bold(true)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(catYellow).
		Padding(0, 1).
		Width(m.runningVP.Width())
	hint := "ctrl+c cancel · j/k scroll"
	if m.runningDone {
		hint = "esc / enter dismiss · j/k scroll"
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		"",
		m.runningVP.View(),
		"",
		lipgloss.NewStyle().Foreground(catSubtle).Render(hint),
	)
	return borderStyle.Render(inner)
}

func (m Model) confirmModalView() string {
	titleStyle := lipgloss.NewStyle().Foreground(catYellow).Bold(true)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(catYellow).
		Padding(0, 1).
		Width(maxInt(56, m.width/2))
	yes := " Yes "
	no := " No "
	if m.confirmYes {
		yes = lipgloss.NewStyle().Foreground(catGreen).Bold(true).Render("[Yes]")
		no = lipgloss.NewStyle().Foreground(catSubtle).Render(" No ")
	} else {
		yes = lipgloss.NewStyle().Foreground(catSubtle).Render(" Yes ")
		no = lipgloss.NewStyle().Foreground(catRed).Bold(true).Render("[No]")
	}
	body := strings.Join(m.confirmLines, "\n")
	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(m.confirmTitle),
		"",
		body,
		"",
		yes+"  "+no,
		lipgloss.NewStyle().Foreground(catSubtle).Render("h/l or y/n, enter confirm, esc cancel"),
	)
	return borderStyle.Render(inner)
}
