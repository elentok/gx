package log

import (
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	m.helpOpen = true
	return m
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "enter", "q":
		m.helpOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.helpViewport, cmd = m.helpViewport.Update(msg)
	return m, cmd
}

func (m Model) helpFullView(width int) string {
	keyStyle := ui.StyleTitle
	descStyle := ui.StyleHint
	sep := descStyle.Render("  ")

	sections := []struct {
		title string
		bindings []key.Binding
	}{
		{"Navigation", []key.Binding{logKeyUp, logKeyDown, logKeyTop, logKeyBottom, logKeyOpen}},
		{"Search", []key.Binding{logKeySearch, logKeyResultNext, logKeyResultPrev}},
		{"Jump", []key.Binding{logKeyHead, logKeyNextTag, logKeyPrevTag}},
		{"Go to", []key.Binding{logKeyWorktrees, logKeyGotoLog, logKeyStatus}},
		{"Other", []key.Binding{logKeyReload, logKeyBack, logKeyHelp}},
	}

	var parts []string
	for _, section := range sections {
		heading := lipgloss.NewStyle().Foreground(ui.ColorOrange).Bold(true).Render(section.title)
		parts = append(parts, heading)
		for _, b := range section.bindings {
			h := b.Help()
			parts = append(parts, "  "+keyStyle.Render(h.Key)+sep+descStyle.Render(h.Desc))
		}
		parts = append(parts, "")
	}

	_ = width
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}

func (m Model) helpModalView() string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       "Keyboard Help",
		Body:        m.helpViewport.View(),
		Hint:        ui.JoinStatus(ui.RenderInlineBindings(logKeyHelp), ui.HintDismissScroll()),
		Width:       m.helpViewport.Width(),
		BorderColor: ui.ColorBlue,
		TitleColor:  ui.ColorBlue,
		HintColor:   ui.ColorGray,
	})
}

// ChordHints returns the available chord completions for the given prefix.
// Implements ui.ChordHinter.
func (m Model) ChordHints(prefix string) []key.Binding {
	switch prefix {
	case "g":
		return []key.Binding{
			key.NewBinding(key.WithHelp("g", "top")),
			key.NewBinding(key.WithHelp("h", "goto HEAD")),
			key.NewBinding(key.WithHelp("w", "goto worktrees")),
			key.NewBinding(key.WithHelp("l", "goto log")),
			key.NewBinding(key.WithHelp("s", "goto status")),
		}
	case "]":
		return []key.Binding{key.NewBinding(key.WithHelp("t", "next tag"))}
	case "[":
		return []key.Binding{key.NewBinding(key.WithHelp("t", "prev tag"))}
	}
	return nil
}
