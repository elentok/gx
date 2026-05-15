package push

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

type phase int

const (
	phaseConfirm      phase = iota
	phaseFetching           // git fetch <remote>
	phaseDiverged           // menu: rebase / force / abort
	phaseRebasing           // git rebase <upstream>
	phasePushing            // git push <remote> <branch>
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

	activeRunner *components.CommandRunner

	// diverged state
	divergence *git.PushDivergence
	menu       components.MenuState

	// force confirm
	forceYes bool

	// pr prompt
	prURL  string
	prYes  bool

	// failed
	failErr error

	// credential prompt
	credOpen   bool
	credPrompt string
	credSecret bool
	credInput  textinput.Model
}

// New returns a zero-value Model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{spinner: sp}
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
	m.credOpen = false
	m.log = ui.NewCommandOutputLog()
	m.activeRunner = nil
	m.IsOpen = true
	return nil
}

// Update handles all messages while the modal is open.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	if m.credOpen {
		return m.handleCredKey(msg)
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
			return m, m.startRunner(phaseRebasing, "rebase", upstream), Result{}
		case "force":
			m.phase = phaseForceConfirm
			m.forceYes = true
			return m, nil, Result{}
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

	// Non-fast-forward push is handled as a prompt, not a hard failure.
	if msg.phase == phasePushing && git.IsNonFastForwardPushError(msg.err) {
		m.phase = phaseForceConfirm
		m.forceYes = true
		return m, nil, Result{}
	}

	if msg.err != nil {
		m.phase = phaseFailed
		m.failErr = msg.err
		return m, nil, Result{}
	}

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
		return m, m.startRunner(phasePushing, "push", m.remote, m.branch), Result{}

	case phaseRebasing:
		return m, m.startRunner(phasePushing, "push", m.remote, m.branch), Result{}

	case phasePushing:
		prURL := git.ExtractPRURL(msg.output)
		if prURL != "" {
			m.prURL = prURL
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
		m.credOpen = true
		m.credPrompt = prompt.Text
		m.credSecret = prompt.Kind == components.PromptKindSecret
		ti := textinput.New()
		ti.Focus()
		if m.credSecret {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '*'
		}
		m.credInput = ti
		return m, nil, Result{}
	}
	return m, cmdPoll(), Result{}
}

func (m Model) handleCredKey(msg tea.Msg) (Model, tea.Cmd, Result) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.credInput, cmd = m.credInput.Update(msg)
		return m, cmd, Result{}
	}
	switch kp.String() {
	case "esc":
		if m.activeRunner != nil {
			m.activeRunner.Cancel()
		}
		m.credOpen = false
		m.phase = phaseFailed
		m.failErr = fmt.Errorf("cancelled")
		return m, nil, Result{}
	case "enter":
		if m.activeRunner != nil {
			_ = m.activeRunner.SubmitPromptInput(m.credInput.Value())
		}
		m.credOpen = false
		return m, cmdPoll(), Result{}
	}
	var cmd tea.Cmd
	m.credInput, cmd = m.credInput.Update(msg)
	return m, cmd, Result{}
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

	if m.credOpen {
		input := m.credInput.View()
		if input == "" {
			input = " "
		}
		return components.RenderInputModal(
			"Credential Required",
			m.credPrompt,
			input,
			ui.HintSubmitCancel(),
			ui.ColorBlue,
			ui.ColorBlue,
			ui.ColorSubtle,
			w,
		)
	}

	switch m.phase {
	case phaseConfirm:
		prompt := fmt.Sprintf("Push branch %s to %s?", m.branch, m.remote)
		return components.RenderConfirmModal(prompt, m.yes,
			ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, w)

	case phaseFetching:
		return m.spinnerModal("Checking push status", "Fetching remote…", w)

	case phaseDiverged:
		prompt := fmt.Sprintf("Branch %s has diverged from the remote branch:\n\n"+
			"Last local commit: %s\n  %s %s\n\n"+
			"Last remote commit: %s\n  %s %s",
			m.divergence.Branch,
			humanizeOrUnknown(m.divergence.Local.Date),
			m.divergence.Local.Hash, m.divergence.Local.Message,
			humanizeOrUnknown(m.divergence.RemoteHead.Date),
			m.divergence.RemoteHead.Hash, m.divergence.RemoteHead.Message,
		)
		return components.RenderMenuModal(
			"Push Diverged", prompt, m.menu, "",
			ui.ColorYellow, ui.ColorYellow, ui.ColorSubtle, ui.ColorGreen, w,
		)

	case phaseRebasing:
		upstream := ""
		if m.divergence != nil {
			upstream = m.divergence.Upstream
		}
		return m.spinnerModal("Rebasing", "Rebasing on "+upstream+"…", w)

	case phasePushing:
		return m.spinnerModal("Pushing", fmt.Sprintf("Pushing %s to %s…", m.branch, m.remote), w)

	case phaseForceConfirm:
		prompt := "Push was rejected as non-fast-forward.\n\nForce push with --force?"
		return components.RenderConfirmModal(prompt, m.forceYes,
			ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, w)

	case phaseForcePushing:
		return m.spinnerModal("Force Pushing", fmt.Sprintf("Force pushing %s to %s…", m.branch, m.remote), w)

	case phasePRPrompt:
		prompt := fmt.Sprintf("Open pull request page?\n\n%s", m.prURL)
		return components.RenderConfirmModal(prompt, m.prYes,
			ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, w)

	case phaseFailed:
		body := ui.StyleWarning.Render(m.failErr.Error()) + "\n\n" + ui.StyleMuted.Render("press esc to dismiss")
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

func (m Model) spinnerModal(title, label string, width int) string {
	body := m.spinner.View() + " " + label
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       title,
		Body:        body,
		Width:       width,
		BorderColor: ui.ColorYellow,
		TitleColor:  ui.ColorYellow,
		HintColor:   ui.ColorSubtle,
	})
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

func selectedMenuValue(state components.MenuState) string {
	if len(state.Items) == 0 {
		return ""
	}
	if state.Cursor < 0 || state.Cursor >= len(state.Items) {
		return ""
	}
	return state.Items[state.Cursor].Value
}

