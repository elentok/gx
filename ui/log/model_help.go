package log

import (
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

const (
	MIN_WIDTH  = 56
	MAX_WIDTH  = 104
	MIN_HEIGHT = 8
)

var keySections = []ui.KeySection{
	ui.NewKeySection("Navigation", logKeyUp, logKeyDown, logKeyTop, logKeyBottom, logKeyOpen),
	ui.NewKeySection("Search", logKeySearch, logKeyResultNext, logKeyResultPrev),
	ui.NewKeySection("Jump", logKeyHead, logKeyNextTag, logKeyPrevTag),
	ui.NewKeySection("Go to", logKeyWorktrees, logKeyGotoLog, logKeyStatus),
	ui.NewKeySection("Other", logKeyReload, logKeyBack, logKeyHelp),
}

func (m Model) enterHelpMode() Model {
	vp := ui.HelpViewportModel(m.width, m.height)
	vp.SetContent(ui.RenderHelpView(keySections))
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

func (m Model) helpModalView() string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Keybindings",
		Body:          m.helpViewport.View(),
		Hint:          ui.JoinStatus(ui.RenderInlineBindings(logKeyHelp), ui.HintDismissAndScroll()),
		Width:         m.helpViewport.Width(),
		BorderColor:   ui.ColorBlue,
		TitleInBorder: true,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorGray,
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
