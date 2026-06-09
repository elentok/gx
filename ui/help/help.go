package help

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filter"
	"github.com/elentok/gx/ui/keys"
)

const (
	MIN_WIDTH  = 56
	MARGIN     = 8
	MIN_HEIGHT = 8
	// scrollbarGutter is the width reserved to the right of the body for the
	// scroll indicator (1 gap + 1 bar). It is always reserved so the layout does
	// not shift when content starts/stops overflowing.
	scrollbarGutter = 2
	// frameChromeX is the horizontal space RenderModalFrame consumes outside the
	// body text: a 1-cell border and 1-cell padding on each side. The frame's
	// Width is the outer width, so the body text area is Width - frameChromeX.
	frameChromeX = 4
)

type Model struct {
	IsOpen      bool
	KeySections []KeySection
	Viewport    viewport.Model
	filter      filter.Model
}

func NewModel(keySections []KeySection) Model {
	return Model{IsOpen: false, KeySections: keySections, Viewport: viewport.New(), filter: filter.NewModel()}
}

// InputFocused reports whether the help filter input is currently capturing
// keystrokes. Hosts must OR this into their own InputFocused() so the app shell
// stops intercepting chord keys (e.g. 'g') while the user is typing a filter.
func (m Model) InputFocused() bool {
	return m.IsOpen && m.filter.InputFocused()
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
		Title:      "Keybindings",
		RightTitle: m.rightTitle(),
		Body:       m.bodyWithScrollbar(),
		Hint:       m.hint(),
		// Frame width must hold the body, the scrollbar gutter, and the frame's
		// own border+padding, or the body+bar block overflows and the bar wraps
		// onto its own line.
		Width:           m.Viewport.Width() + scrollbarGutter + frameChromeX,
		BorderColor:     ui.ColorBlue,
		TitleInBorder:   true,
		TitleColor:      ui.ColorBlue,
		RightTitleColor: ui.ColorGray,
		HintColor:       ui.ColorGray,
	})
}

// rightTitle shows the match count while the filter has a query.
func (m Model) rightTitle() string {
	if !m.filter.HasQuery() {
		return ""
	}
	switch n := m.matchCount(); n {
	case 0:
		return "no matches"
	case 1:
		return "1 match"
	default:
		return fmt.Sprintf("%d matches", n)
	}
}

// hint is the footer: while filtering, show how to clear; otherwise show the
// dismiss/scroll keys plus the '/' filter affordance.
func (m Model) hint() string {
	if m.filter.IsActive() {
		return ui.JoinStatus(ui.HintClearFilter(), ui.HintDismissAndScroll())
	}
	return ui.JoinStatus(ui.HintDismissAndScroll(), ui.HintFilter())
}

// bodyWithScrollbar renders the viewport body with a scroll indicator gutter to
// its right when the content overflows. The gutter width is always reserved in
// the layout (see scrollbarGutter), so the body width is stable whether or not
// the bar shows.
func (m Model) bodyWithScrollbar() string {
	body := m.Viewport.View()
	bar := ui.RenderScrollbar(
		m.Viewport.Height(),
		m.Viewport.TotalLineCount(),
		m.Viewport.VisibleLineCount(),
		m.Viewport.YOffset(),
	)
	if bar != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, " ", bar)
	}
	if m.filter.IsActive() {
		// Filter input bar pinned above the body while filtering.
		body = m.filter.View() + "\n\n" + body
	}
	return body
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Route to the filter first while it owns the input (so 'q', '?', etc. type
	// literally instead of closing help) or when '/' should start a filter.
	if m.filter.InputFocused() || msg.String() == "/" {
		var res filter.Result
		var cmd tea.Cmd
		m.filter, cmd, res = m.filter.Update(msg)
		if res.QueryChanged {
			m.applyFilter()
		}
		if res.Handled {
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc":
		// esc clears an active filter first; only a second esc closes help.
		if m.filter.IsActive() {
			m.filter.Clear()
			m.applyFilter()
			return m, nil
		}
		m.IsOpen = false
		return m, nil
	case "q", "?", "enter":
		m.IsOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return m, cmd
}

func (m *Model) Open(containerWidth, containerHeight int) {
	m.IsOpen = true
	m.setContainerSize(containerWidth, containerHeight)
}

// helpWidth returns the viewport (body) width: widen toward the container, less a
// margin, with a MIN_WIDTH fallback for narrow terminals.
func helpWidth(containerWidth int) int {
	return max(containerWidth-MARGIN, MIN_WIDTH)
}

func (m *Model) setContainerSize(containerWidth, containerHeight int) {
	// The viewport content width is the modal width less the frame chrome and the
	// reserved scrollbar gutter, so the body + bar fill the text area exactly
	// (rather than the bar widening the modal or being clipped at the edge).
	vpW := helpWidth(containerWidth) - frameChromeX - scrollbarGutter
	vpH := max(containerHeight/2-4, MIN_HEIGHT)
	m.Viewport.SetWidth(vpW)
	m.Viewport.SetHeight(vpH)
	m.filter.SetWidth(vpW)
	m.Viewport.SetContent(RenderColumns(m.visibleSections(), vpW))
}

// visibleSections narrows the sections to those with bindings matching the
// filter query (case-insensitive substring of the key display OR the title),
// dropping empty sections. With no query it returns all sections unchanged.
func (m Model) visibleSections() []KeySection {
	q := strings.ToLower(strings.TrimSpace(m.filter.Query()))
	if q == "" {
		return m.KeySections
	}
	var out []KeySection
	for _, s := range m.KeySections {
		var kept []keys.Binding
		for _, b := range s.Bindings {
			if strings.Contains(strings.ToLower(b.Keys()), q) || strings.Contains(strings.ToLower(b.Title), q) {
				kept = append(kept, b)
			}
		}
		if len(kept) > 0 {
			out = append(out, KeySection{Title: s.Title, Bindings: kept})
		}
	}
	return out
}

// matchCount totals the surviving bindings under the current filter.
func (m Model) matchCount() int {
	n := 0
	for _, s := range m.visibleSections() {
		n += len(s.Bindings)
	}
	return n
}

// applyFilter re-packs the columns for the current query and resets the scroll
// position to the top so the first matches are visible.
func (m *Model) applyFilter() {
	m.Viewport.SetContent(RenderColumns(m.visibleSections(), m.Viewport.Width()))
	m.Viewport.GotoTop()
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
