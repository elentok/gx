package pull

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/creds"
)

type phase int

const (
	phaseChecking        phase = iota // checking for uncommitted changes
	phaseStashConfirm                 // uncommitted changes detected — ask user to confirm stash
	phaseStashing                     // git stash (local, no creds)
	phasePulling                      // git pull (network, needs credential poll)
	phaseStashPopping                 // git stash pop after success (local)
	phasePopStashConfirm              // pull failed with stash — offer manual pop
	phaseFailed
)

// Result is returned on each Update call when something changed.
type Result struct {
	Done    bool
	Aborted bool   // user cancelled before the pull completed — no success notification
	Output  string // accumulated git output — store for "g o" viewing
	Err     error
}

// internal messages
type changesCheckMsg struct {
	hasChanges bool
	err        error
}
type stashDoneMsg struct {
	err    error
	output string
}
type pullDoneMsg struct {
	err    error
	output string
}
type stashPopDoneMsg struct {
	err    error
	output string
}

type pollMsg struct{}

func cmdPoll() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return pollMsg{} })
}

// Model owns the entire pull lifecycle.
type Model struct {
	IsOpen bool

	root    string
	phase   phase
	stashed bool
	spinner spinner.Model
	log     *ui.CommandOutputLog
	steps   []components.Step

	activeRunner *components.CommandRunner

	// stash-before-pull confirm
	stashConfirmYes bool

	// pop-stash confirm
	stashYes bool

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

// Open starts the pull (checks for uncommitted changes; stashes only after confirmation).
func (m *Model) Open(root string) tea.Cmd {
	m.root = root
	m.phase = phaseChecking
	m.stashed = false
	m.stashYes = true
	m.failErr = nil
	m.creds = creds.New()
	m.activeRunner = nil
	m.log = ui.NewCommandOutputLog()
	m.steps = nil
	m.IsOpen = true
	return cmdCheckChanges(root)
}

// Update handles all messages while the modal is open.
func (m Model) InputFocused() bool { return m.creds.IsOpen }

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

	case changesCheckMsg:
		return m.handleChangesCheck(msg)

	case stashDoneMsg:
		return m.handleStashDone(msg)

	case pullDoneMsg:
		return m.handlePullDone(msg)

	case stashPopDoneMsg:
		return m.handleStashPopDone(msg)

	case pollMsg:
		return m.handlePoll()
	}

	return m, nil, Result{}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	switch m.phase {
	case phaseStashConfirm:
		next, decided, accepted, handled := components.UpdateConfirm(msg, m.stashConfirmYes)
		if !handled {
			return m, nil, Result{}
		}
		m.stashConfirmYes = next
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Done: true, Aborted: true}
		}
		m.stashed = true
		m.phase = phaseStashing
		m.appendRunningStep(stepStash)
		return m, cmdStash(m.root), Result{}

	case phasePopStashConfirm:
		next, decided, accepted, handled := components.UpdateConfirm(msg, m.stashYes)
		if !handled {
			return m, nil, Result{}
		}
		m.stashYes = next
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Done: true, Aborted: true, Output: m.log.String()}
		}
		m.phase = phaseStashPopping
		m.appendRunningStep(stepStashPop)
		return m, cmdStashPop(m.root), Result{}

	case phaseFailed:
		switch msg.String() {
		case "esc", "enter", "q":
			m.IsOpen = false
			return m, nil, Result{Done: true, Output: m.log.String(), Err: m.failErr}
		}
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
	stepStash    = components.Step{TitleBefore: "stash", RunningTitle: "stashing...", TitleAfter: "stashed", TitleFailed: "stash failed"}
	stepPull     = components.Step{TitleBefore: "pull", RunningTitle: "pulling...", TitleAfter: "pulled", TitleFailed: "pull failed"}
	stepStashPop = components.Step{TitleBefore: "restore stash", RunningTitle: "restoring stash...", TitleAfter: "restored stash", TitleFailed: "stash pop failed"}
)

func (m Model) handleChangesCheck(msg changesCheckMsg) (Model, tea.Cmd, Result) {
	if msg.err != nil {
		m.phase = phaseFailed
		m.failErr = msg.err
		return m, nil, Result{}
	}
	if msg.hasChanges {
		m.phase = phaseStashConfirm
		m.stashConfirmYes = true
		return m, nil, Result{}
	}
	m.appendRunningStep(stepPull)
	return m, m.startPulling(), Result{}
}

func (m Model) handleStashDone(msg stashDoneMsg) (Model, tea.Cmd, Result) {
	m.log.AppendCommand("git", []string{"stash", "push"}, msg.output)
	if msg.err != nil {
		m.failCurrentStep()
		m.phase = phaseFailed
		m.failErr = fmt.Errorf("stash failed: %w", msg.err)
		return m, nil, Result{}
	}
	m.completeCurrentStep()
	m.appendRunningStep(stepPull)
	return m, m.startPulling(), Result{}
}

func (m Model) handlePullDone(msg pullDoneMsg) (Model, tea.Cmd, Result) {
	m.activeRunner = nil
	m.log.AppendCommand("git", []string{"pull"}, msg.output)
	if msg.err != nil {
		m.failCurrentStep()
		if m.stashed {
			m.phase = phasePopStashConfirm
			m.stashYes = true
			return m, nil, Result{}
		}
		m.phase = phaseFailed
		m.failErr = msg.err
		return m, nil, Result{}
	}
	m.completeCurrentStep()
	if m.stashed {
		m.phase = phaseStashPopping
		m.appendRunningStep(stepStashPop)
		return m, cmdStashPop(m.root), Result{}
	}
	m.IsOpen = false
	return m, nil, Result{Done: true, Output: m.log.String()}
}

func (m Model) handleStashPopDone(msg stashPopDoneMsg) (Model, tea.Cmd, Result) {
	m.log.AppendCommand("git", []string{"stash", "pop"}, msg.output)
	if msg.err != nil {
		m.failCurrentStep()
		m.phase = phaseFailed
		m.failErr = fmt.Errorf("stash pop failed: %w", msg.err)
		return m, nil, Result{}
	}
	m.completeCurrentStep()
	m.IsOpen = false
	return m, nil, Result{Done: true, Output: m.log.String()}
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

// startPulling stores the runner in m.activeRunner so handlePoll can surface
// credential prompts, then returns poll + wait commands.
func (m *Model) startPulling() tea.Cmd {
	runner := components.NewCommandRunnerWithPolicy(m.root, "git", components.CredentialPolicyPrompt, "pull")
	m.activeRunner = runner
	m.phase = phasePulling
	runner.Start()
	return tea.Batch(cmdPoll(), m.spinner.Tick, func() tea.Msg {
		err := runner.Wait()
		return pullDoneMsg{err: err, output: runner.Output()}
	})
}

func cmdCheckChanges(root string) tea.Cmd {
	return func() tea.Msg {
		changes, err := git.UncommittedChanges(root)
		return changesCheckMsg{hasChanges: len(changes) > 0, err: err}
	}
}

func cmdStash(root string) tea.Cmd {
	return func() tea.Msg {
		runner := components.NewCommandRunnerWithPolicy(root, "git", components.CredentialPolicyPrompt,
			"stash", "push", "-u", "-m", "gx-pull-auto-stash")
		runner.Start()
		err := runner.Wait()
		return stashDoneMsg{err: err, output: runner.Output()}
	}
}

func cmdStashPop(root string) tea.Cmd {
	return func() tea.Msg {
		runner := components.NewCommandRunnerWithPolicy(root, "git", components.CredentialPolicyPrompt, "stash", "pop")
		runner.Start()
		err := runner.Wait()
		return stashPopDoneMsg{err: err, output: runner.Output()}
	}
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
	case phaseChecking:
		body := m.spinner.View() + " Pulling…"
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Pull",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phaseStashConfirm:
		body := "You have uncommitted changes.\n\nStash and pull?" + "\n\n" + components.RenderConfirmChoices(m.stashConfirmYes, false)
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Pull",
			Body:        body,
			Hint:        components.ConfirmHint,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phaseStashing, phasePulling, phaseStashPopping:
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Pull",
			Body:        stepsStr,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phasePopStashConfirm:
		body := stepsStr + "\n\n" + "Pull failed. Pop the stash?" + "\n\n" + components.RenderConfirmChoices(m.stashYes, false)
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Pull",
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
			Title:       "Pull Failed",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorRed,
			TitleColor:  ui.ColorRed,
			HintColor:   ui.ColorSubtle,
		})
	}
	return ""
}

func modalWidth(width int) int {
	return max(56, min(100, width/2))
}
