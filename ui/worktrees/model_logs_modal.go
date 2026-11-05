package worktrees

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// logsModal shows the last job's output in a scrollable viewport.
type logsModal struct {
	viewport viewport.Model
	label    string
	log      string
}

// enterLogsMode opens the logs modal on the last job output.
func (m Model) enterLogsMode() Model {
	vpW := m.width * 2 / 3
	if vpW < 40 {
		vpW = 40
	}
	if vpW > 100 {
		vpW = 100
	}
	vpH := m.height/2 - 6
	if vpH < 3 {
		vpH = 3
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(m.lastJobLog)
	return m.openModal(logsModal{viewport: vp, label: m.lastJobLabel, log: m.lastJobLog})
}

// Update scrolls the logs viewport or dismisses it.
func (l logsModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "enter", "q":
			return l, nil, modalResult{close: true}
		}
	case tea.MouseWheelMsg:
	default:
		return l, nil, modalResult{unhandled: true}
	}
	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return l, cmd, modalResult{}
}

func (l logsModal) InputFocused() bool { return false }

// View renders a centred modal with the last job's output.
func (l logsModal) View(_, _ int) string {
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
