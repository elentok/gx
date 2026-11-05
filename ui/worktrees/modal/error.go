package modal

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Error shows an error in a scrollable viewport.
type Error struct {
	viewport viewport.Model
	// hasLog is true when the last job left output the "o" key can open.
	hasLog bool
}

// NewError builds the error modal for a page of the given size. hasLog says
// the last job left output the "o" key can open.
func NewError(errMsg string, width, height int, hasLog bool) Error {
	vpW := width * 2 / 3
	if vpW < 40 {
		vpW = 40
	}
	if vpW > 80 {
		vpW = 80
	}
	vpH := height/2 - 6
	if vpH < 3 {
		vpH = 3
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(errMsg)
	return Error{viewport: vp, hasLog: hasLog}
}

// Viewport is the scrollable error text.
func (e Error) Viewport() viewport.Model { return e.viewport }

// Update scrolls the error viewport or dismisses it.
func (e Error) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "enter", "q":
			return e, nil, Result{Close: true}
		case "o":
			if e.hasLog {
				return e, nil, Result{OpenLogs: true}
			}
		}
	case tea.MouseWheelMsg:
	default:
		return e, nil, Result{Unhandled: true}
	}
	var cmd tea.Cmd
	e.viewport, cmd = e.viewport.Update(msg)
	return e, cmd, Result{}
}

func (e Error) InputFocused() bool { return false }

func (e Error) hint() string {
	hint := "esc / enter / q  dismiss"
	if e.hasLog {
		hint = ui.JoinStatus(hint, ui.StyleTitle.Render("go")+" "+ui.StyleHint.Render("view output"))
	}
	return ui.StyleDim.Render(hint)
}

// View renders a centred modal with the error text.
func (e Error) View(_, _ int) string {
	return components.RenderOutputModal(
		"Error",
		e.viewport.View(),
		ansi.Strip(e.hint()),
		ui.ColorRed,
		ui.ColorRed,
		ui.ColorGray,
		e.viewport.Width(),
	)
}
