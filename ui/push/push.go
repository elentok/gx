package push

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	humanize "github.com/dustin/go-humanize"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/creds"
)

type phase int

const (
	phaseConfirm      phase = iota
	phaseFetching           // git fetch <remote>
	phaseDiverged           // menu: rebase / force / abort
	phaseRebasing           // git rebase <upstream>
	phasePushing            // git push <remote> <branch>
	phaseTagPushing         // git push <remote> <tag>
	phaseForceConfirm       // yes/no: force push?
	phaseForcePushing       // git push --force <remote> <branch>
	phasePRPrompt           // yes/no: open PR URL?
	phaseFailed
)

// Result is returned on each Update call when something noteworthy happened.
type Result struct {
	Done   bool   // operation finished (success, failure, or abort)
	Output string // accumulated git output — store for "g o" viewing
	Err    error  // non-nil on failure
}

type runnerDoneMsg struct {
	phase  phase
	output string
	err    error
}

type pollMsg struct{}

func cmdPoll() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return pollMsg{} })
}

func cmdWaitRunner(r *components.CommandRunner, p phase) tea.Cmd {
	return func() tea.Msg {
		err := r.Wait()
		return runnerDoneMsg{phase: p, output: r.Output(), err: err}
	}
}

// Model owns the entire push lifecycle.
type Model struct {
	IsOpen bool

	root   string
	branch string
	remote string

	phase   phase
	yes     bool
	spinner spinner.Model
	log     *ui.CommandOutputLog
	steps   []components.Step

	activeRunner *components.CommandRunner

	// diverged state
	divergence *git.PushDivergence
	menu       components.MenuState

	// force confirm
	forceYes bool

	// pr prompt
	prURL  string
	prYes  bool

	// tag push (optional, set via OpenWithTag)
	tag string

	// failed
	failErr error

	creds creds.Model
}

// New returns a zero-value Model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{spinner: sp}
}

// OpenWithTag is like Open but also pushes the given tag after a successful branch push.
func (m *Model) OpenWithTag(root, tag string) error {
	if err := m.Open(root); err != nil {
		return err
	}
	m.tag = tag
	return nil
}

// Open resolves branch/remote and opens the initial confirm dialog.
func (m *Model) Open(root string) error {
	branch, err := git.CurrentBranch(root)
	if err != nil {
		return err
	}
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("cannot push: detached HEAD")
	}
	remote := git.BranchRemote(git.Repo{Root: root}, branch)

	m.root = root
	m.branch = branch
	m.remote = remote
	m.phase = phaseConfirm
	m.yes = true
	m.forceYes = true
	m.prYes = true
	m.failErr = nil
	m.creds = creds.New()
	m.log = ui.NewCommandOutputLog()
	m.activeRunner = nil
	m.steps = nil
	m.tag = ""
	m.divergence = nil
	m.menu = components.MenuState{}
	m.prURL = ""
	m.IsOpen = true
	return nil
}

// Update handles all messages while the modal is open.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	if m.creds.IsOpen {
		next, cmd, res := m.creds.Update(msg)
		m.creds = next
		if res.Decided {
			if res.Cancelled {
				if m.activeRunner != nil {
					m.activeRunner.Cancel()
				}
				m.phase = phaseFailed
				m.failErr = fmt.Errorf("cancelled")
				return m, nil, Result{}
			}
			if m.activeRunner != nil {
				_ = m.activeRunner.SubmitPromptInput(res.Value)
			}
			return m, cmdPoll(), Result{}
		}
		return m, cmd, Result{}
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd, Result{}

	case runnerDoneMsg:
		return m.handleRunnerDone(msg)

	case pollMsg:
		return m.handlePoll()
	}

	return m, nil, Result{}
}

func (m *Model) appendRunningStep(s components.Step) {
	s.IsRunning = true
	m.steps = append(m.steps, s)
}

func (m *Model) completeCurrentStep() {
	if len(m.steps) > 0 {
		last := len(m.steps) - 1
		m.steps[last].IsRunning = false
		m.steps[last].IsDone = true
	}
}

func (m *Model) failCurrentStep() {
	if len(m.steps) > 0 {
		last := len(m.steps) - 1
		m.steps[last].IsRunning = false
		m.steps[last].HasFailed = true
	}
}

var (
	stepFetch     = components.Step{TitleBefore: "fetch", RunningTitle: "fetching...", TitleAfter: "fetched", TitleFailed: "fetch failed"}
	stepRebase    = components.Step{TitleBefore: "rebase", RunningTitle: "rebasing...", TitleAfter: "rebased", TitleFailed: "rebase failed"}
	stepPush      = components.Step{TitleBefore: "push", RunningTitle: "pushing...", TitleAfter: "pushed", TitleFailed: "push failed (non-fast-forward)"}
	stepForcePush = components.Step{TitleBefore: "force push", RunningTitle: "force pushing...", TitleAfter: "force pushed", TitleFailed: "force push failed"}
)

func (m Model) stepPushTag() components.Step {
	return components.Step{TitleBefore: "push tag " + m.tag, RunningTitle: "pushing tag " + m.tag + "...", TitleAfter: "pushed tag " + m.tag, TitleFailed: "tag push failed"}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	switch m.phase {
	case phaseConfirm:
		next, decided, accepted, handled := components.UpdateConfirm(msg, m.yes)
		if !handled {
			return m, nil, Result{}
		}
		m.yes = next
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Done: true}
		}
		m.appendRunningStep(stepFetch)
		return m, m.startRunner(phaseFetching, "fetch", m.remote), Result{}

	case phaseDiverged:
		next, decided, accepted, handled := components.UpdateMenu(msg, m.menu)
		if !handled {
			return m, nil, Result{}
		}
		m.menu = next
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Done: true, Output: m.log.String()}
		}
		choice := selectedMenuValue(m.menu)
		switch choice {
		case "rebase":
			upstream := m.divergence.Upstream
			if upstream == "" {
				upstream = m.divergence.Remote
			}
			m.appendRunningStep(stepRebase)
			return m, m.startRunner(phaseRebasing, "rebase", upstream), Result{}
		case "force":
			m.appendRunningStep(stepForcePush)
			return m, m.startRunner(phaseForcePushing, "push", "--force", m.remote, m.branch), Result{}
		default:
			m.IsOpen = false
			return m, nil, Result{Done: true, Output: m.log.String()}
		}

	case phaseForceConfirm:
		next, decided, accepted, handled := components.UpdateConfirm(msg, m.forceYes)
		if !handled {
			return m, nil, Result{}
		}
		m.forceYes = next
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Done: true, Output: m.log.String()}
		}
		m.appendRunningStep(stepForcePush)
		return m, m.startRunner(phaseForcePushing, "push", "--force", m.remote, m.branch), Result{}

	case phasePRPrompt:
		next, decided, accepted, handled := components.UpdateConfirm(msg, m.prYes)
		if !handled {
			return m, nil, Result{}
		}
		m.prYes = next
		if !decided {
			return m, nil, Result{}
		}
		m.IsOpen = false
		out := m.log.String()
		if accepted {
			return m, ui.CmdOpenURL(m.prURL), Result{Done: true, Output: out}
		}
		return m, nil, Result{Done: true, Output: out}

	case phaseFailed:
		switch msg.String() {
		case "esc", "enter", "q":
			m.IsOpen = false
			return m, nil, Result{Done: true, Output: m.log.String(), Err: m.failErr}
		}
	}
	return m, nil, Result{}
}

func (m Model) handleRunnerDone(msg runnerDoneMsg) (Model, tea.Cmd, Result) {
	m.log.AppendCommand("git", m.runnerArgs(msg.phase), msg.output)
	m.activeRunner = nil

	// Non-fast-forward push transitions to force confirm, not hard failure.
	if msg.phase == phasePushing && git.IsNonFastForwardPushError(msg.err) {
		m.failCurrentStep()
		m.phase = phaseForceConfirm
		m.forceYes = true
		return m, nil, Result{}
	}

	if msg.err != nil {
		m.failCurrentStep()
		m.phase = phaseFailed
		m.failErr = msg.err
		return m, nil, Result{}
	}

	m.completeCurrentStep()

	switch msg.phase {
	case phaseFetching:
		div, err := git.DetectPushDivergenceAfterFetch(m.root, m.branch)
		if err != nil {
			m.phase = phaseFailed
			m.failErr = err
			return m, nil, Result{}
		}
		if div != nil {
			m.divergence = div
			m.phase = phaseDiverged
			m.menu = components.MenuState{
				Items: []components.MenuItem{
					{Label: "Rebase", Value: "rebase"},
					{Label: "Push --force", Value: "force"},
					{Label: "Abort", Value: "abort"},
				},
			}
			return m, nil, Result{}
		}
		m.appendRunningStep(stepPush)
		return m, m.startRunner(phasePushing, "push", m.remote, m.branch), Result{}

	case phaseRebasing:
		m.appendRunningStep(stepPush)
		return m, m.startRunner(phasePushing, "push", m.remote, m.branch), Result{}

	case phasePushing:
		// Capture PR URL now; we may still need to push the tag first.
		m.prURL = git.ExtractPRURL(msg.output)
		if m.tag != "" {
			m.appendRunningStep(m.stepPushTag())
			return m, m.startRunner(phaseTagPushing, "push", m.remote, m.tag), Result{}
		}
		if m.prURL != "" {
			m.phase = phasePRPrompt
			m.prYes = true
			return m, nil, Result{}
		}
		m.IsOpen = false
		return m, nil, Result{Done: true, Output: m.log.String()}

	case phaseTagPushing:
		if m.prURL != "" {
			m.phase = phasePRPrompt
			m.prYes = true
			return m, nil, Result{}
		}
		m.IsOpen = false
		return m, nil, Result{Done: true, Output: m.log.String()}

	case phaseForcePushing:
		m.IsOpen = false
		return m, nil, Result{Done: true, Output: m.log.String()}
	}

	return m, nil, Result{}
}

func (m Model) handlePoll() (Model, tea.Cmd, Result) {
	if m.activeRunner == nil {
		return m, nil, Result{}
	}
	if prompt, ok := m.activeRunner.Prompt(); ok {
		m.creds.Open(prompt)
		return m, nil, Result{}
	}
	return m, cmdPoll(), Result{}
}

func (m *Model) startRunner(p phase, args ...string) tea.Cmd {
	runner := components.NewCommandRunnerWithPolicy(m.root, "git", components.CredentialPolicyPrompt, args...)
	m.activeRunner = runner
	m.phase = p
	runner.Start()
	return tea.Batch(cmdPoll(), m.spinner.Tick, cmdWaitRunner(runner, p))
}

// View renders the appropriate modal for the current phase.
func (m Model) View(width int) string {
	w := modalWidth(width)

	if m.creds.IsOpen {
		return m.creds.View(w)
	}

	stepsStr := ""
	if len(m.steps) > 0 {
		stepsStr = components.RenderSteps(m.steps, m.spinner.View())
	}

	switch m.phase {
	case phaseConfirm:
		return components.RenderConfirmModal(m.confirmPrompt(), m.yes,
			ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, w)

	case phaseFetching, phaseRebasing, phasePushing, phaseTagPushing, phaseForcePushing:
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Push",
			Body:        stepsStr,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phaseDiverged:
		divergeInfo := fmt.Sprintf("Branch %s has diverged from the remote branch:\n\n"+
			"Last local commit: %s\n  %s %s\n\n"+
			"Last remote commit: %s\n  %s %s",
			m.divergence.Branch,
			humanizeOrUnknown(m.divergence.Local.Date),
			m.divergence.Local.Hash, m.divergence.Local.Message,
			humanizeOrUnknown(m.divergence.RemoteHead.Date),
			m.divergence.RemoteHead.Hash, m.divergence.RemoteHead.Message,
		)
		prompt := stepsStr + "\n\n" + divergeInfo
		return components.RenderMenuModal(
			"Push Diverged", prompt, m.menu, "",
			ui.ColorYellow, ui.ColorYellow, ui.ColorSubtle, ui.ColorGreen, w,
		)

	case phaseForceConfirm:
		body := stepsStr + "\n\n" + "Push was rejected as non-fast-forward.\n\nForce push with --force?" + "\n\n" + components.RenderConfirmChoices(m.forceYes, false)
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Push",
			Body:        body,
			Hint:        components.ConfirmHint,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phasePRPrompt:
		body := stepsStr + "\n\n" + fmt.Sprintf("Open pull request page?\n\n%s", m.prURL) + "\n\n" + components.RenderConfirmChoices(m.prYes, false)
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Push",
			Body:        body,
			Hint:        components.ConfirmHint,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phaseFailed:
		body := stepsStr
		if body != "" {
			body += "\n\n"
		}
		body += ui.StyleWarning.Render(m.failErr.Error()) + "\n\n" + ui.StyleMuted.Render("press esc to dismiss")
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Push Failed",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorRed,
			TitleColor:  ui.ColorRed,
			HintColor:   ui.ColorSubtle,
		})
	}
	return ""
}


func (m Model) runnerArgs(p phase) []string {
	switch p {
	case phaseFetching:
		return []string{"fetch", m.remote}
	case phaseRebasing:
		if m.divergence != nil && m.divergence.Upstream != "" {
			return []string{"rebase", m.divergence.Upstream}
		}
		return []string{"rebase"}
	case phasePushing:
		return []string{"push", m.remote, m.branch}
	case phaseTagPushing:
		return []string{"push", m.remote, m.tag}
	case phaseForcePushing:
		return []string{"push", "--force", m.remote, m.branch}
	}
	return nil
}

func modalWidth(width int) int {
	return max(56, min(100, width/2))
}

func humanizeOrUnknown(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return humanize.Time(t)
}

var pushBranchStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)
var pushRemoteStyle = lipgloss.NewStyle().Foreground(ui.ColorTeal)

func (m Model) confirmPrompt() string {
	branch := pushBranchStyle.Render(m.branch)
	remote := pushRemoteStyle.Render(m.remote)
	if m.tag != "" {
		return fmt.Sprintf("Push branch %s and tag %s to %s?", branch, pushBranchStyle.Render(m.tag), remote)
	}
	return fmt.Sprintf("Push branch %s to %s?", branch, remote)
}

func selectedMenuValue(state components.MenuState) string {
	if len(state.Items) == 0 {
		return ""
	}
	if state.Cursor < 0 || state.Cursor >= len(state.Items) {
		return ""
	}
	return state.Items[state.Cursor].Value
}

