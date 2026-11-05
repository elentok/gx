package modal

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// Logs shows the last job's output in a scrollable viewport.
type Logs struct {
	viewport viewport.Model
	label    string
	log      string
}

// NewLogs builds the logs modal for a page of the given size.
func NewLogs(log, label string, width, height int) Logs {
	vpW := width * 2 / 3
	if vpW < 40 {
		vpW = 40
	}
	if vpW > 100 {
		vpW = 100
	}
	vpH := height/2 - 6
	if vpH < 3 {
		vpH = 3
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(log)
	return Logs{viewport: vp, label: label, log: log}
}

// Viewport is the scrollable job output.
func (l Logs) Viewport() viewport.Model { return l.viewport }

// Update scrolls the logs viewport or dismisses it.
func (l Logs) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "enter", "q":
			return l, nil, Result{Close: true}
		}
	case tea.MouseWheelMsg:
	default:
		return l, nil, Result{Unhandled: true}
	}
	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return l, cmd, Result{}
}

func (l Logs) InputFocused() bool { return false }

// View renders a centred modal with the last job's output.
func (l Logs) View(_, _ int) string {
	content := l.log
	if content == "" {
		content = ui.StyleDim.Render("(" + ui.MessageNoOutput() + ")")
	}
	l.viewport.SetContent(content)
	return components.RenderOutputModal(
		l.label,
		l.viewport.View(),
		ui.HintDismiss(),
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorGray,
		l.viewport.Width(),
	)
}
