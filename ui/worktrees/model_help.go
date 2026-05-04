package worktrees

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"

	tea "charm.land/bubbletea/v2"
)

func (m Model) enterHelpMode() Model {
	vp := help.NewViewportModel(m.width, m.height)
	vp.SetContent(m.helpFullView(vp.Width() - 2))
	m.helpViewport = vp
	m.mode = modeHelp
	return m
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "enter", "q":
		m.mode = modeNormal
		return m, nil
	}
	var cmd tea.Cmd
	m.helpViewport, cmd = m.helpViewport.Update(msg)
	return m, cmd
}

func (m Model) helpFullView(width int) string {
	h := m.help
	h.ShowAll = true
	if width > 0 {
		h.SetWidth(width)
	}
	return h.View(keys)
}

func (m Model) helpModalView() string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       "Keyboard Help",
		Body:        m.helpViewport.View(),
		Hint:        ui.JoinStatus(ui.RenderInlineBindings(keys.Help), ui.HintDismissAndScroll()),
		Width:       m.helpViewport.Width(),
		BorderColor: ui.ColorBlue,
		TitleColor:  ui.ColorBlue,
		HintColor:   ui.ColorGray,
	})
}
