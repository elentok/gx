package commit

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keybindings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var commitKeyHelp = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))

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

func (m Model) helpFullView(_ int) string {
	keyStyle := ui.StyleTitle
	descStyle := ui.StyleHint
	sep := descStyle.Render("  ")
	heading := func(title string) string {
		return lipgloss.NewStyle().Foreground(ui.ColorOrange).Bold(true).Render(title)
	}

	var lines []string
	for _, section := range buildKeySections(m.keys) {
		lines = append(lines, heading(section.Title))
		for _, b := range section.Bindings {
			h := b.Help()
			lines = append(lines, "  "+keyStyle.Render(h.Key)+sep+descStyle.Render(h.Desc))
		}
		lines = append(lines, "")
	}

	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

func (m Model) helpModalView() string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       "Keyboard Help",
		Body:        m.helpViewport.View(),
		Hint:        ui.JoinStatus(ui.RenderInlineBindings(commitKeyHelp), ui.HintDismissAndScroll()),
		Width:       m.helpViewport.Width(),
		BorderColor: ui.ColorBlue,
		TitleColor:  ui.ColorBlue,
		HintColor:   ui.ColorGray,
	})
}

func (m Model) ChordHints(_ string) []key.Binding {
	hints := m.keys.ChordHints()
	out := make([]key.Binding, len(hints))
	for i, h := range hints {
		out[i] = key.NewBinding(key.WithHelp(h.Key, h.Desc))
	}
	return out
}

var helpSectionOrder = []string{"Global", "Go to", "Header", "Diff", "Yank", "Navigation"}

func buildKeySections(manager keybindings.Manager) []help.KeySection {
	sections := []help.KeySection{}
	byCategory := map[string][]key.Binding{}
	seen := map[string]map[keybindings.BindingID]bool{}
	for _, b := range manager.Bindings() {
		if b.Title == "" {
			continue
		}
		for _, cat := range b.Categories {
			if cat == "" {
				continue
			}
			if seen[cat] == nil {
				seen[cat] = map[keybindings.BindingID]bool{}
			}
			if seen[cat][b.ID] {
				continue
			}
			seen[cat][b.ID] = true
			byCategory[cat] = append(byCategory[cat], key.NewBinding(key.WithKeys(b.Seq...), key.WithHelp(b.Keys(), b.Title)))
		}
	}
	for _, cat := range helpSectionOrder {
		bindings := byCategory[cat]
		if len(bindings) == 0 {
			continue
		}
		sections = append(sections, help.NewKeySection(cat, bindings...))
	}
	return sections
}
