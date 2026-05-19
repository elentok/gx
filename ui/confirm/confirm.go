package confirm

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// Options configures a confirm modal.
type Options struct {
	Prompt       string
	Items        []string // optional bullet list rendered below the prompt
	AcceptCmd    tea.Cmd  // executed when the user confirms
	SpinnerLabel string   // returned in Result so the parent can start its own spinner
	CancelMsg    string   // emitted as notify.Info when the user cancels
	DefaultYes   bool     // initial cursor position; false = No
}

// Result is returned by Update when the user has made a decision.
type Result struct {
	Done         bool
	Accepted     bool
	SpinnerLabel string
}

type storedOpts struct {
	prompt       string
	items        []string
	acceptCmd    tea.Cmd
	spinnerLabel string
	cancelMsg    string
}

// Model is an embeddable confirm modal sub-model.
type Model struct {
	IsOpen bool

	opts storedOpts
	yes  bool
}

// New returns a zero-value Model.
func New() Model {
	return Model{}
}

// Open opens the modal with the given options and returns the updated model.
func (m Model) Open(opts Options) Model {
	m.IsOpen = true
	m.yes = opts.DefaultYes
	m.opts = storedOpts{
		prompt:       opts.Prompt,
		items:        opts.Items,
		acceptCmd:    opts.AcceptCmd,
		spinnerLabel: opts.SpinnerLabel,
		cancelMsg:    opts.CancelMsg,
	}
	return m
}

// Update handles key events while the modal is open.
// Returns the updated model, a command to run, and a Result.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil, Result{}
	}

	nextYes, decided, accepted, handled := components.UpdateConfirm(keyMsg, m.yes)
	if !handled {
		return m, nil, Result{}
	}
	m.yes = nextYes
	if !decided {
		return m, nil, Result{}
	}

	m.IsOpen = false
	if accepted {
		return m, m.opts.acceptCmd, Result{
			Done:         true,
			Accepted:     true,
			SpinnerLabel: m.opts.spinnerLabel,
		}
	}

	var cmd tea.Cmd
	if m.opts.cancelMsg != "" {
		cmd = notify.Info(m.opts.cancelMsg)
	}
	return m, cmd, Result{Done: true, Accepted: false}
}

// View renders the confirm modal.
func (m Model) View(width int) string {
	return components.RenderConfirmModal(
		m.opts.prompt,
		m.yes,
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorRed,
		ui.ColorGray,
		width,
		m.opts.items...,
	)
}
