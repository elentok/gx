package worktrees

import (
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func (m Model) enterHelpMode() Model {
	vpW := m.width * 2 / 3
	if vpW < 56 {
		vpW = 56
	}
	if vpW > 104 {
		vpW = 104
	}
	vpH := m.height/2 - 4
	if vpH < 8 {
		vpH = 8
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(m.helpFullView(vpW - 2))
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
