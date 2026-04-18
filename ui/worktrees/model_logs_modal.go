package worktrees

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// enterLogsMode switches the model into logs mode to display the last job output.
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
	m.logsViewport = vp
	m.mode = modeLogs
	return m
}

// handleLogsKey scrolls the logs viewport or dismisses it.
func (m Model) handleLogsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.mode = modeNormal
		return m, nil
	}
	var cmd tea.Cmd
	m.logsViewport, cmd = m.logsViewport.Update(msg)
	return m, cmd
}

// logsModalView renders a centred modal with the last job's output.
func (m Model) logsModalView() string {
	content := m.lastJobLog
	if content == "" {
		content = ui.StyleDim.Render("(" + ui.MessageNoOutput() + ")")
	}
	m.logsViewport.SetContent(content)
	return components.RenderOutputModal(
		m.lastJobLabel,
		m.logsViewport.View(),
		ui.HintDismiss(),
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorGray,
		m.logsViewport.Width(),
	)
}
