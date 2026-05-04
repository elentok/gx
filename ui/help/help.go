package help

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
)

const (
	MIN_WIDTH  = 56
	MAX_WIDTH  = 104
	MIN_HEIGHT = 8
)

type Model struct {
	IsOpen      bool
	KeySections []KeySection
	Viewport    viewport.Model
}

func NewModel(keySections []KeySection) Model {
	return Model{IsOpen: false, KeySections: keySections, Viewport: viewport.New()}
}

// This makes HelpModel compatible with tea.Model
func (m Model) Init() tea.Cmd { return nil }
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetContainerSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		m, cmd = m.HandleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	if !m.IsOpen {
		return ""
	}

	m.Viewport.SetContent(RenderView(m.KeySections))

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Keybindings",
		Body:          m.Viewport.View(),
		Hint:          ui.JoinStatus(ui.HintDismissAndScroll()),
		Width:         m.Viewport.Width(),
		BorderColor:   ui.ColorBlue,
		TitleInBorder: true,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorGray,
	})
}

func (m Model) HandleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "q", "?", "esc", "enter":
		m.IsOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return m, cmd

}

// deprecated
func NewViewportModel(containerWidth int, containerHeight int) viewport.Model {
	vpW := min(max(containerWidth*2/3, MIN_WIDTH), MAX_WIDTH)
	vpH := max(containerHeight/2-4, MIN_HEIGHT)
	return viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
}

func (m *Model) Open(containerWidth, containerHeight int) {
	m.IsOpen = true
	m.SetContainerSize(containerWidth, containerHeight)
}

func (m *Model) SetContainerSize(containerWidth, containerHeight int) {
	vpW := min(max(containerWidth*2/3, MIN_WIDTH), MAX_WIDTH)
	vpH := max(containerHeight/2-4, MIN_HEIGHT)
	m.Viewport.SetWidth(vpW)
	m.Viewport.SetHeight(vpH)
}

type KeySection struct {
	Title    string
	Bindings []key.Binding
}

func NewKeySection(title string, bindings ...key.Binding) KeySection {
	return KeySection{Title: title, Bindings: bindings}
}

func RenderView(sections []KeySection) string {
	keyStyle := ui.StyleTitle
	descStyle := ui.StyleHint
	sep := descStyle.Render("  ")

	var parts []string
	for _, section := range sections {
		heading := ui.StyleHelpHeading.Render(section.Title)
		parts = append(parts, heading)
		for _, b := range section.Bindings {
			h := b.Help()
			parts = append(parts, "  "+keyStyle.Render(h.Key)+sep+descStyle.Render(h.Desc))
		}
		parts = append(parts, "")
	}

	var result strings.Builder
	for i, p := range parts {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(p)
	}
	return result.String()
}
