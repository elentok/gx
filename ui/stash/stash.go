package stash

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

type phase int

const (
	phaseInput phase = iota // entering an optional stash name
	phaseStashing
	phaseFailed
)

// Outcome describes how the stash interaction finished.
type Outcome int

const (
	OutcomeNone Outcome = iota
	OutcomeStashed
	OutcomeCancelled
)

// Result is returned on each Update call; only meaningful when Done is true.
type Result struct {
	Done       bool
	Outcome    Outcome
	StagedOnly bool
	Err        error
}

type stashDoneMsg struct {
	output string
	err    error
}

// Model owns the stash-name → run-stash lifecycle.
type Model struct {
	IsOpen bool

	root       string
	stagedOnly bool
	phase      phase
	input      textinput.Model
	spinner    spinner.Model
	failErr    error
	output     string
}

// New returns a zero-value Model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{spinner: sp}
}

// Open resets the model, focuses a fresh text input and opens the modal.
func (m *Model) Open(root string, stagedOnly bool) tea.Cmd {
	m.root = root
	m.stagedOnly = stagedOnly
	m.phase = phaseInput
	m.failErr = nil
	m.output = ""
	ti := textinput.New()
	ti.Focus()
	m.input = ti
	m.IsOpen = true
	return textinput.Blink
}

// InputFocused reports whether the text input is currently capturing keys.
func (m Model) InputFocused() bool { return m.IsOpen && m.phase == phaseInput }

// StagedOnly reports whether the model was opened in staged-only mode.
func (m Model) StagedOnly() bool { return m.stagedOnly }

// Update handles all messages while the modal is open.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case stashDoneMsg:
		if msg.err != nil {
			m.phase = phaseFailed
			m.failErr = msg.err
			m.output = msg.output
			return m, nil, Result{}
		}
		m.IsOpen = false
		return m, nil, Result{Done: true, Outcome: OutcomeStashed, StagedOnly: m.stagedOnly}

	case spinner.TickMsg:
		if m.phase != phaseStashing {
			return m, nil, Result{}
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd, Result{}
	}
	return m, nil, Result{}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	switch m.phase {
	case phaseInput:
		switch msg.String() {
		case "esc":
			m.IsOpen = false
			return m, nil, Result{Done: true, Outcome: OutcomeCancelled}
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			m.phase = phaseStashing
			return m, tea.Batch(m.cmdStash(name), m.spinner.Tick), Result{}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd, Result{}

	case phaseFailed:
		switch msg.String() {
		case "esc", "enter", "q":
			m.IsOpen = false
			return m, nil, Result{Done: true, Err: m.failErr}
		}
	}
	return m, nil, Result{}
}

func (m Model) cmdStash(name string) tea.Cmd {
	root, stagedOnly := m.root, m.stagedOnly
	return func() tea.Msg {
		out, err := git.StashPush(root, name, stagedOnly)
		return stashDoneMsg{output: out, err: err}
	}
}

// View renders the modal for the current phase.
func (m Model) View(width int) string {
	w := modalWidth(width)
	switch m.phase {
	case phaseInput:
		title := "Stash all changes"
		prompt := "Stash all changes (staged + unstaged). Name (optional):"
		if m.stagedOnly {
			title = "Stash staged changes"
			prompt = "Stash staged changes only. Name (optional):"
		}
		input := m.input.View()
		if input == "" {
			input = " "
		}
		return components.RenderInputModal(
			title,
			prompt,
			input,
			ui.HintSubmitCancel(),
			ui.ColorBlue,
			ui.ColorBlue,
			ui.ColorSubtle,
			w,
		)

	case phaseStashing:
		body := m.spinner.View() + " stashing..."
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Stash",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorBlue,
			TitleColor:  ui.ColorBlue,
			HintColor:   ui.ColorSubtle,
		})

	case phaseFailed:
		body := ui.StyleWarning.Render(m.failErr.Error()) + "\n\n" + ui.StyleMuted.Render("press esc to dismiss")
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Stash Failed",
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
