package help

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
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
		m.setContainerSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		m, cmd = m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	if !m.IsOpen {
		return ""
	}

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

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
	m.setContainerSize(containerWidth, containerHeight)
	m.Viewport.SetContent(RenderView(m.KeySections))
}

func (m *Model) setContainerSize(containerWidth, containerHeight int) {
	vpW := min(max(containerWidth*2/3, MIN_WIDTH), MAX_WIDTH)
	vpH := max(containerHeight/2-4, MIN_HEIGHT)
	m.Viewport.SetWidth(vpW)
	m.Viewport.SetHeight(vpH)
}

type KeySection struct {
	Title    string
	Bindings []keys.Binding
}

func NewKeySection(title string, bindings ...keys.Binding) KeySection {
	return KeySection{Title: title, Bindings: bindings}
}

func BuildSections(managers ...keys.Manager) []KeySection {
	categoryBindings := make(map[string][]keys.Binding)
	// indexInCategory tracks, per category, where each BindingID's entry lives in
	// categoryBindings so twin bindings (same ID) can be merged into one row whose
	// key display joins their sequences with "/" (alternatives), e.g. "1/gw".
	indexInCategory := make(map[string]map[keys.BindingID]int)
	for _, manager := range managers {
		for _, b := range manager.Bindings() {
			if b.Title == "" {
				continue
			}
			for _, cat := range b.Categories {
				if cat == "" {
					continue
				}
				if indexInCategory[cat] == nil {
					indexInCategory[cat] = make(map[keys.BindingID]int)
				}
				if idx, ok := indexInCategory[cat][b.ID]; ok {
					existing := &categoryBindings[cat][idx]
					existing.Display = existing.Keys() + "/" + b.Keys()
					continue
				}
				indexInCategory[cat][b.ID] = len(categoryBindings[cat])
				categoryBindings[cat] = append(categoryBindings[cat], keys.Binding{ID: b.ID, Seq: b.Seq, Title: b.Title, Display: b.Display})
			}
		}
	}
	cats := make([]string, 0, len(categoryBindings))
	for cat := range categoryBindings {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	sections := make([]KeySection, 0, len(cats))
	for _, cat := range cats {
		sections = append(sections, NewKeySection(cat, categoryBindings[cat]...))
	}
	return sections
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
			parts = append(parts, "  "+keyStyle.Render(b.Keys())+sep+descStyle.Render(b.Title))
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
