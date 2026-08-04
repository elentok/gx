package confirm

import (
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

// Update handles key and mouse-click events while the modal is open.
// Returns the updated model, a command to run, and a Result.
//
// Mouse clicks are hit-tested in the modal's own coordinate frame, i.e. the
// same frame View() renders into (row/col 0 = the modal's top-left corner).
// Callers placing the modal on screen via ui.OverlayCenter must translate an
// absolute mouse position into that frame first - see UpdateMouse.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		nextYes, decided, accepted, handled := components.UpdateConfirm(msg, m.yes)
		if !handled {
			return m, nil, Result{}
		}
		m.yes = nextYes
		if !decided {
			return m, nil, Result{}
		}
		return m.decide(accepted)

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil, Result{}
		}
		if accepted, hit := m.hitTest(m.View(0), msg.X, msg.Y); hit {
			return m.decide(accepted)
		}
	}
	return m, nil, Result{}
}

// UpdateMouse translates an absolute-screen mouse message into the modal's
// local coordinate frame - matching where the caller placed View(width) via
// ui.OverlayCenter(_, _, screenW, screenH) - and hit-tests it there.
func (m Model) UpdateMouse(msg tea.MouseClickMsg, width, screenW, screenH int) (Model, tea.Cmd, Result) {
	if msg.Button != tea.MouseLeft {
		return m, nil, Result{}
	}
	view := m.View(width)
	ox, oy := ui.OverlayCenterOrigin(lipgloss.Width(view), lipgloss.Height(view), screenW, screenH)
	if accepted, hit := m.hitTest(view, msg.X-ox, msg.Y-oy); hit {
		return m.decide(accepted)
	}
	return m, nil, Result{}
}

// hitTest reports which button (if any) is under the local-frame coordinate
// (x, y) in view, an already-rendered View() output. It searches the
// rendered text for the literal padded button labels rather than tracking
// bounds separately, since the body's line count shifts with prompt wrapping
// at different widths and duplicating that layout math would drift out of
// sync.
func (m Model) hitTest(view string, x, y int) (accepted bool, hit bool) {
	lines := strings.Split(view, "\n")
	if y < 0 || y >= len(lines) {
		return false, false
	}
	plain := ansi.Strip(lines[y])
	if col, width, ok := findButtonColumn(plain, " Yes "); ok && x >= col && x < col+width {
		return true, true
	}
	if col, width, ok := findButtonColumn(plain, " No "); ok && x >= col && x < col+width {
		return false, true
	}
	return false, false
}

// findButtonColumn locates label's display column within plain (an
// ansi.Strip'd line), using rune-display width rather than a byte offset -
// the leading border/padding characters may be multi-byte, so a byte index
// would drift from the terminal column a real mouse click reports.
func findButtonColumn(plain, label string) (col, width int, ok bool) {
	before, _, found := strings.Cut(plain, label)
	if !found {
		return 0, 0, false
	}
	return ansi.StringWidth(before), ansi.StringWidth(label), true
}

func (m Model) decide(accepted bool) (Model, tea.Cmd, Result) {
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
