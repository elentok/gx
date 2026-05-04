package status

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
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
	actionPushFetch
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
	log    *ui.CommandOutputLog
	done   chan stageActionResult
	res    stageActionResult
	ok     bool
}

func newStageActionRunner(kind stageActionKind, root, remote, branch string) *stageActionRunner {
	return newStageActionRunnerWithOutput(kind, root, remote, branch, "")
}

func newStageActionRunnerWithOutput(kind stageActionKind, root, remote, branch, initialOutput string) *stageActionRunner {
	return &stageActionRunner{
		kind:   kind,
		root:   root,
		remote: remote,
		branch: branch,
		runner: components.NewCommandRunnerWithPolicy(root, "git", components.CredentialPolicyPrompt),
		log:    ui.CommandOutputLogFrom(initialOutput),
		done:   make(chan stageActionResult, 1),
	}
}

func (r *stageActionRunner) Start() {
	go func() {
		res := r.run()
		res.output = r.log.String()
		r.done <- res
	}()
}

func (r *stageActionRunner) Cancel() {
	r.runner.Cancel()
}

func (r *stageActionRunner) Consume() string {
	return r.runner.Consume()
}

func (r *stageActionRunner) Prompt() (components.CredentialPrompt, bool) {
	return r.runner.Prompt()
}

func (r *stageActionRunner) SubmitPromptInput(input string) error {
	return r.runner.SubmitPromptInput(input)
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
	case actionPushFetch:
		res := stageActionResult{kind: r.kind, remote: r.remote, branch: r.branch}
		res.err = r.execGit("fetch", r.remote)
		return res
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
	r.runner = components.NewCommandRunnerWithPolicy(r.root, "git", components.CredentialPolicyPrompt, args...)
	r.runner.Start()
	err := r.runner.Wait()
	out := r.runner.Output()
	r.log.AppendCommand("git", args, out)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return &git.RunError{Args: args, Dir: r.root, Stdout: strings.TrimSpace(out), Stderr: strings.TrimSpace(ee.Error()), Code: ee.ExitCode()}
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

func (m *Model) appendRunningOutput(chunk string) {
	if chunk == "" {
		return
	}
	m.runningContent += sanitizeTerminalOutputForViewport(chunk)
	m.runningVP.SetContent(m.runningContent)
	m.runningVP.GotoBottom()
}

func sanitizeTerminalOutputForViewport(s string) string {
	s = ansiOSCRe.ReplaceAllString(s, "")
	s = ansiCSIRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
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

func newCredentialInput(secret bool) textinput.Model {
	ti := textinput.New()
	ti.Focus()
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	return ti
}

func (m *Model) openCredentialPrompt(prompt components.CredentialPrompt) {
	m.credentialPrompt = prompt.Text
	m.credentialSecret = prompt.Kind == components.PromptKindSecret
	m.credentialInput = newCredentialInput(m.credentialSecret)
	m.credentialOpen = true
}

func (m *Model) handleCredentialKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.runningRunner != nil {
			m.runningRunner.Cancel()
		}
		m.credentialOpen = false
		m.credentialPrompt = ""
		return m, nil
	case "enter":
		if m.runningRunner != nil {
			_ = m.runningRunner.SubmitPromptInput(m.credentialInput.Value())
		}
		m.credentialOpen = false
		m.credentialPrompt = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.credentialInput, cmd = m.credentialInput.Update(msg)
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
			m.setStatus(ui.MessageAborted("push"))
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
			initialOutput := m.pendingActionOutput
			m.pendingActionOutput = ""
			target := strings.TrimSpace(m.confirmUpstream)
			if target == "" {
				target = strings.TrimSpace(m.confirmRemote)
			}
			runner := newStageActionRunnerWithOutput(actionRebase, m.worktreeRoot, target, m.confirmBranch, initialOutput)
			m.openRunning("Rebase on "+target, runner)
			return m, actionPollCmd()
		case "force":
			m.confirmOpen = false
			m.confirmAction = confirmNone
			initialOutput := m.pendingActionOutput
			m.pendingActionOutput = ""
			runner := newStageActionRunnerWithOutput(actionForcePush, m.worktreeRoot, m.confirmRemote, m.confirmBranch, initialOutput)
			m.openRunning("Force push", runner)
			return m, actionPollCmd()
		default:
			m.confirmOpen = false
			m.confirmAction = confirmNone
			m.pendingActionOutput = ""
			m.setStatus(ui.MessageAborted("push"))
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

func stageActionOutputTitle(kind stageActionKind) string {
	switch kind {
	case actionPull:
		return "Pull output"
	case actionPushFetch:
		return "Fetch output"
	case actionPush:
		return "Push output"
	case actionForcePush:
		return "Force push output"
	case actionRebase:
		return "Rebase output"
	case actionPopStashPull, actionPopStashRebase:
		return "Stash output"
	case actionAmend:
		return "Amend output"
	default:
		return "Command output"
	}
}

func (m *Model) recordCommandOutput(title, output string) {
	output = strings.TrimSpace(output)
	if output == "" {
		return
	}
	m.outputTitle = title
	m.outputContent = output
}

func (m *Model) openOutputModal() {
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
	vp.SetContent(m.outputContent)
	m.outputViewport = vp
	m.outputOpen = true
}

func (m Model) handleOutputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.outputOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.outputViewport, cmd = m.outputViewport.Update(msg)
	return m, cmd
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

func (m Model) runningModalView() string {
	title := m.runningTitle
	if title == "" {
		title = "Running"
	}
	hint := ui.HintCancelScroll()
	if m.runningDone {
		hint = ui.HintDismissAndScroll()
	}
	return components.RenderOutputModal(
		title,
		m.runningVP.View(),
		hint,
		ui.ColorYellow,
		ui.ColorYellow,
		ui.ColorSubtle,
		m.runningVP.Width(),
	)
}

func (m Model) outputModalView() string {
	title := m.outputTitle
	if title == "" {
		title = "Command output"
	}
	return components.RenderOutputModal(
		title,
		m.outputViewport.View(),
		ui.HintDismissAndScroll(),
		ui.ColorYellow,
		ui.ColorYellow,
		ui.ColorSubtle,
		m.outputViewport.Width(),
	)
}

func (m Model) credentialModalView() string {
	title := "Credential Required"
	input := m.credentialInput.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		title,
		m.credentialPrompt,
		input,
		ui.HintSubmitCancel(),
		ui.ColorBlue,
		ui.ColorBlue,
		ui.ColorSubtle,
		0,
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
			"",
			ui.ColorYellow,
			ui.ColorYellow,
			ui.ColorSubtle,
			ui.ColorGreen,
			maxInt(56, m.width/2),
		)
	}
	return components.RenderConfirmModal(
		prompt,
		m.confirmYes,
		ui.ColorYellow,
		ui.ColorGreen,
		ui.ColorRed,
		ui.ColorSubtle,
		maxInt(56, m.width/2),
	)
}
