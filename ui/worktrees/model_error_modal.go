package worktrees

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// errorModal shows an error in a scrollable viewport.
type errorModal struct {
	viewport viewport.Model
	// hasLog is true when the last job left output the "o" key can open.
	hasLog bool
}

// showError opens the error modal.
func (m Model) showError(errMsg string) Model {
	vpW := m.width * 2 / 3
	if vpW < 40 {
		vpW = 40
	}
	if vpW > 80 {
		vpW = 80
	}
	vpH := m.height/2 - 6
	if vpH < 3 {
		vpH = 3
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(errMsg)
	return m.openModal(errorModal{viewport: vp, hasLog: m.lastJobLog != ""})
}

// Update scrolls the error viewport or dismisses it.
func (e errorModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "enter", "q":
			return e, nil, modalResult{close: true}
		case "o":
			if e.hasLog {
				return e, nil, modalResult{openLogs: true}
			}
		}
	case tea.MouseWheelMsg:
	default:
		return e, nil, modalResult{unhandled: true}
	}
	var cmd tea.Cmd
	e.viewport, cmd = e.viewport.Update(msg)
	return e, cmd, modalResult{}
}

func (e errorModal) InputFocused() bool { return false }

func (e errorModal) hint() string {
	hint := "esc / enter / q  dismiss"
	if e.hasLog {
		hint = ui.JoinStatus(hint, ui.StyleTitle.Render("go")+" "+ui.StyleHint.Render("view output"))
	}
	return ui.StyleDim.Render(hint)
}

// View renders a centred modal with the error text.
func (e errorModal) View(_, _ int) string {
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
