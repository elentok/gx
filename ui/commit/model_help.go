package commit

import (
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	commitKeyUp       = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	commitKeyDown     = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	commitKeyTop      = key.NewBinding(key.WithKeys("gg"), key.WithHelp("gg", "top"))
	commitKeyBottom   = key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom"))
	commitKeyTab      = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle pane"))
	commitKeyBack     = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back / exit pane"))
	commitKeyExpand   = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "toggle commit body"))
	commitKeySearch   = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	commitKeyFilePrev = key.NewBinding(key.WithKeys(","), key.WithHelp(",/.", "prev/next commit"))
	commitKeyFileNext = key.NewBinding(key.WithKeys(","), key.WithHelp(",/.", "prev/next file (diff)"))
	commitKeyMode     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle hunk/line mode"))
	commitKeyWrap     = key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "toggle wrap"))
	commitKeyEnter    = key.NewBinding(key.WithKeys("enter", "l"), key.WithHelp("enter/l", "open / expand"))
	commitKeyCollapse = key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "collapse / exit diff"))
	commitKeyYank     = key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank…"))
	commitKeyYankText = key.NewBinding(key.WithKeys("yy"), key.WithHelp("yy", "yank content"))
	commitKeyYankPath = key.NewBinding(key.WithKeys("yl"), key.WithHelp("yl", "yank location"))
	commitKeyYankAll  = key.NewBinding(key.WithKeys("ya"), key.WithHelp("ya", "yank all"))
	commitKeyYankFile = key.NewBinding(key.WithKeys("yf"), key.WithHelp("yf", "yank filename"))
	commitKeyGotoWT   = key.NewBinding(key.WithKeys("gw"), key.WithHelp("gw", "goto worktrees"))
	commitKeyGotoLog  = key.NewBinding(key.WithKeys("gl"), key.WithHelp("gl", "goto log"))
	commitKeyGotoSt   = key.NewBinding(key.WithKeys("gs"), key.WithHelp("gs", "goto status"))
	commitKeyHelp     = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
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

func (m Model) helpFullView(_ int) string {
	keyStyle := ui.StyleTitle
	descStyle := ui.StyleHint
	sep := descStyle.Render("  ")
	heading := func(title string) string {
		return lipgloss.NewStyle().Foreground(ui.ColorOrange).Bold(true).Render(title)
	}

	sections := []struct {
		title    string
		bindings []key.Binding
	}{
		{"Header pane", []key.Binding{commitKeyTab, commitKeyExpand, commitKeyUp, commitKeyDown, commitKeyBack}},
		{"Files pane", []key.Binding{commitKeyTab, commitKeyUp, commitKeyDown, commitKeyTop, commitKeyBottom, commitKeyEnter, commitKeyCollapse, commitKeyFilePrev, commitKeyBack}},
		{"Diff pane", []key.Binding{commitKeyTab, commitKeyUp, commitKeyDown, commitKeyTop, commitKeyBottom, commitKeyMode, commitKeyWrap, commitKeySearch, commitKeyFileNext, commitKeyCollapse}},
		{"Yank", []key.Binding{commitKeyYankText, commitKeyYankPath, commitKeyYankAll, commitKeyYankFile}},
		{"Go to", []key.Binding{commitKeyGotoWT, commitKeyGotoLog, commitKeyGotoSt}},
	}

	var lines []string
	for _, section := range sections {
		lines = append(lines, heading(section.title))
		for _, b := range section.bindings {
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

// ChordHints returns the available chord completions for the given prefix.
// Implements ui.ChordHinter.
func (m Model) ChordHints(prefix string) []key.Binding {
	switch prefix {
	case "g":
		return []key.Binding{
			key.NewBinding(key.WithHelp("g", "top")),
			key.NewBinding(key.WithHelp("w", "goto worktrees")),
			key.NewBinding(key.WithHelp("l", "goto log")),
			key.NewBinding(key.WithHelp("s", "goto status")),
		}
	case "y":
		if m.focusHeader {
			return []key.Binding{
				key.NewBinding(key.WithHelp("y", "yank commit body")),
				key.NewBinding(key.WithHelp("l", "yank location")),
				key.NewBinding(key.WithHelp("a", "yank all")),
				key.NewBinding(key.WithHelp("f", "yank filename")),
			}
		}
		return []key.Binding{
			key.NewBinding(key.WithHelp("y", "yank content")),
			key.NewBinding(key.WithHelp("l", "yank location")),
			key.NewBinding(key.WithHelp("a", "yank all")),
			key.NewBinding(key.WithHelp("f", "yank filename")),
		}
	}
	return nil
}
