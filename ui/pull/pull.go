package pull

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

type phase int

const (
	phaseRunning phase = iota
	phasePopStashConfirm
	phaseFailed
)

// Result is returned on each Update call when something changed.
type Result struct {
	Done   bool
	Output string // accumulated git output — store for "g o" viewing
	Err    error
}

type runnerDoneMsg struct {
	phase       phase
	output      string
	err         error
	stashed     bool
	promptStash bool // pull failed after stash — offer pop
}

type pollMsg struct{}

func cmdPoll() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return pollMsg{} })
}

// Model owns the entire pull lifecycle (no confirm — starts immediately).
type Model struct {
	IsOpen bool

	root string

	phase   phase
	spinner spinner.Model
	log     *ui.CommandOutputLog

	activeRunner *components.CommandRunner

	// pop-stash confirm
	stashYes bool
	stashed  bool

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

// Open starts the pull immediately (no confirm step).
func (m *Model) Open(root string) tea.Cmd {
	m.root = root
	m.phase = phaseRunning
	m.failErr = nil
	m.credOpen = false
	m.stashed = false
	m.stashYes = true
	m.log = ui.NewCommandOutputLog()
	m.activeRunner = nil
	m.IsOpen = true
	return m.startPull()
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
			return m, nil, Result{Done: true, Output: m.log.String()}
		}
		return m, m.startStashPop(), Result{}

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
	m.activeRunner = nil

	switch msg.phase {
	case phaseRunning:
		m.log.AppendCommand("git", []string{"pull"}, msg.output)
		m.stashed = msg.stashed
		if msg.err != nil {
			if msg.promptStash {
				m.phase = phasePopStashConfirm
				m.stashYes = true
				return m, nil, Result{}
			}
			m.phase = phaseFailed
			m.failErr = msg.err
			return m, nil, Result{}
		}
		m.IsOpen = false
		return m, nil, Result{Done: true, Output: m.log.String()}

	case phasePopStashConfirm: // stash pop
		m.log.AppendCommand("git", []string{"stash", "pop"}, msg.output)
		if msg.err != nil {
			m.phase = phaseFailed
			m.failErr = fmt.Errorf("stash pop failed: %w", msg.err)
			return m, nil, Result{}
		}
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

func (m *Model) startPull() tea.Cmd {
	return func() tea.Msg {
		log := ui.NewCommandOutputLog()

		changes, err := git.UncommittedChanges(m.root)
		if err != nil {
			return runnerDoneMsg{phase: phaseRunning, err: err}
		}

		stashed := false
		if len(changes) > 0 {
			runner := components.NewCommandRunnerWithPolicy(m.root, "git", components.CredentialPolicyPrompt,
				"stash", "push", "-u", "-m", "gx-pull-auto-stash")
			runner.Start()
			if err := runner.Wait(); err != nil {
				log.AppendCommand("git", []string{"stash", "push"}, runner.Output())
				return runnerDoneMsg{phase: phaseRunning, output: log.String(), err: fmt.Errorf("stash failed: %w", err)}
			}
			log.AppendCommand("git", []string{"stash", "push"}, runner.Output())
			stashed = true
		}

		runner := components.NewCommandRunnerWithPolicy(m.root, "git", components.CredentialPolicyPrompt, "pull")
		runner.Start()
		pullErr := runner.Wait()
		log.AppendCommand("git", []string{"pull"}, runner.Output())

		if pullErr != nil {
			return runnerDoneMsg{phase: phaseRunning, output: log.String(), err: pullErr, stashed: stashed, promptStash: stashed}
		}

		if stashed {
			popRunner := components.NewCommandRunnerWithPolicy(m.root, "git", components.CredentialPolicyPrompt, "stash", "pop")
			popRunner.Start()
			if err := popRunner.Wait(); err != nil {
				log.AppendCommand("git", []string{"stash", "pop"}, popRunner.Output())
				return runnerDoneMsg{phase: phaseRunning, output: log.String(), stashed: true, promptStash: true}
			}
			log.AppendCommand("git", []string{"stash", "pop"}, popRunner.Output())
		}

		return runnerDoneMsg{phase: phaseRunning, output: log.String(), stashed: stashed}
	}
}

func (m *Model) startStashPop() tea.Cmd {
	root := m.root
	return func() tea.Msg {
		runner := components.NewCommandRunnerWithPolicy(root, "git", components.CredentialPolicyPrompt, "stash", "pop")
		runner.Start()
		err := runner.Wait()
		return runnerDoneMsg{phase: phasePopStashConfirm, output: runner.Output(), err: err}
	}
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
	case phaseRunning:
		body := m.spinner.View() + " Pulling…"
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Pull",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phasePopStashConfirm:
		prompt := "Pull failed. Pop the stash?"
		return components.RenderConfirmModal(prompt, m.stashYes,
			ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, w)

	case phaseFailed:
		body := ui.StyleWarning.Render(m.failErr.Error()) + "\n\n" + ui.StyleMuted.Render("press esc to dismiss")
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
