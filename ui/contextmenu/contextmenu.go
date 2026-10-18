// Package contextmenu is the shared Context menu: every action a person can
// take on one Target (a ticket row, an epic row, a session), grouped and
// fuzzy-filtered in an "Actions" modal.
package contextmenu

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
)

// Group places an item under a header; groups render in declaration order.
type Group int

const (
	Suggested Group = iota
	Status
	Server
	Agent
	Queue
	EditCopy
)

var groupNames = map[Group]string{
	Suggested: "Suggested",
	Status:    "Status",
	Server:    "Server",
	Agent:     "Agent",
	Queue:     "Queue",
	EditCopy:  "Edit/copy",
}

func (g Group) String() string { return groupNames[g] }

type Item[T any] struct {
	Label    string
	Key      string // direct-key hint, e.g. "e e"; display only
	Group    Group
	Disabled string // non-empty = dim; shown under the list while selected
	Run      func(T) tea.Cmd
}

const (
	title = "Actions"
	width = 56
	// inner fits a row inside the frame whether or not width counts the
	// border and padding, so a row never wraps and click rows stay aligned.
	inner = width - 4
	// listTop is the View line of the list's first row: top border (with
	// the title), filter line, blank.
	listTop = 3
)

var (
	hint = ui.RenderInlineBindings(
		key.NewBinding(key.WithHelp("↑/↓", "move")),
		key.NewBinding(key.WithHelp("type", "filter")),
		key.NewBinding(key.WithHelp("enter", "run")),
		key.NewBinding(key.WithHelp("esc", "close")),
	)
	dimStyle    = lipgloss.NewStyle().Foreground(ui.ColorOverlay)
	keyStyle    = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	markerStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	headerStyle = lipgloss.NewStyle().Foreground(ui.ColorMauve).Bold(true)
	ruleStyle   = lipgloss.NewStyle().Foreground(ui.ColorBorder)
)

// Menu is an embeddable Context menu over Target type T. cursor indexes
// visible().
type Menu[T any] struct {
	IsOpen bool

	target T
	prompt string
	items  []Item[T]
	cursor int
	filter string
}

// Open opens the menu on target; prompt names the target.
func (m Menu[T]) Open(target T, prompt string, items []Item[T]) Menu[T] {
	m.IsOpen = true
	m.target = target
	m.prompt = prompt
	m.items = slices.Clone(items)
	slices.SortStableFunc(m.items, func(a, b Item[T]) int { return int(a.Group) - int(b.Group) })
	m.filter = ""
	m.cursor = m.firstEnabled()
	return m
}

// Items is the open menu's items in group order, filter aside.
func (m Menu[T]) Items() []Item[T] { return m.items }

func (m Menu[T]) visible() []Item[T] {
	if m.filter == "" {
		return m.items
	}
	var out []Item[T]
	for _, it := range m.items {
		if ui.FuzzyMatch(m.filter, it.Label) {
			out = append(out, it)
		}
	}
	return out
}

func (m Menu[T]) firstEnabled() int {
	return max(0, slices.IndexFunc(m.visible(), func(it Item[T]) bool { return it.Disabled == "" }))
}

// step moves the cursor by dir, landing on dim items too so their reason
// shows; it stays put at either end.
func (m Menu[T]) step(dir int) int {
	return max(0, min(m.cursor+dir, len(m.visible())-1))
}

// Update handles keys and left clicks while the menu is open. Clicks are in
// the frame View renders into (0,0 = its top-left); see UpdateMouse for
// screen coordinates.
func (m Menu[T]) Update(msg tea.Msg) (Menu[T], tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		if i, ok := m.hitRow(msg.Y); ok {
			return m.run(i)
		}
	}
	return m, nil
}

// UpdateMouse hit-tests a screen-coordinate click against View placed with
// ui.OverlayCenter on a screenW x screenH screen.
func (m Menu[T]) UpdateMouse(msg tea.MouseClickMsg, screenW, screenH int) (Menu[T], tea.Cmd) {
	view := m.View()
	ox, oy := ui.OverlayCenterOrigin(lipgloss.Width(view), lipgloss.Height(view), screenW, screenH)
	msg.X -= ox
	msg.Y -= oy
	return m.Update(msg)
}

// updateKey treats every printable key as filter text, so the menu has no
// letter shortcuts.
func (m Menu[T]) updateKey(msg tea.KeyPressMsg) (Menu[T], tea.Cmd) {
	switch msg.String() {
	case "ctrl+n", "down":
		m.cursor = m.step(1)
	case "ctrl+p", "up":
		m.cursor = m.step(-1)
	case "enter":
		return m.run(m.cursor)
	case "esc":
		m.IsOpen = false
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
			m.cursor = m.firstEnabled()
		}
	default:
		if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			m.filter += msg.Text
			m.cursor = m.firstEnabled()
		}
	}
	return m, nil
}

// run closes the menu and returns visible item i's command; a dim or
// missing item does nothing and leaves the menu open.
func (m Menu[T]) run(i int) (Menu[T], tea.Cmd) {
	visible := m.visible()
	if i < 0 || i >= len(visible) || visible[i].Disabled != "" {
		return m, nil
	}
	m.IsOpen = false
	if visible[i].Run == nil {
		return m, nil
	}
	return m, visible[i].Run(m.target)
}

// row is a group header (item < 0) or a visible item.
type row struct {
	group Group
	item  int
}

// rows gives each non-empty group a header; empty groups never appear since
// visible() holds only matching items.
func rows[T any](visible []Item[T]) []row {
	var out []row
	for i, it := range visible {
		if i == 0 || visible[i-1].Group != it.Group {
			out = append(out, row{group: it.Group, item: -1})
		}
		out = append(out, row{group: it.Group, item: i})
	}
	return out
}

// hitRow maps a View line to a visible item; false off an item row.
func (m Menu[T]) hitRow(y int) (int, bool) {
	rs := rows(m.visible())
	i := y - listTop
	if i < 0 || i >= len(rs) || rs[i].item < 0 {
		return 0, false
	}
	return rs[i].item, true
}

func (m Menu[T]) View() string {
	visible := m.visible()
	filter := dimStyle.Render("filter…")
	if m.filter != "" {
		filter = m.filter
	}
	lines := []string{markerStyle.Render("› ") + filter, ""}
	if len(visible) == 0 {
		lines = append(lines, dimStyle.Render("  no matching actions"))
	}
	for _, r := range rows(visible) {
		if r.item < 0 {
			lines = append(lines, renderHeader(r.group))
			continue
		}
		lines = append(lines, m.renderItem(visible[r.item], r.item == m.cursor))
	}
	// The reason line is always there, blank on enabled items, so the frame
	// keeps its height as the cursor moves.
	reason := ""
	if m.cursor < len(visible) && visible[m.cursor].Disabled != "" {
		it := visible[m.cursor]
		reason = ui.StyleMuted.Render(ansi.Truncate(it.Label+": "+it.Disabled, inner, "…"))
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         ansi.Truncate(title+" · "+m.prompt, inner-2, "…"),
		TitleInBorder: true,
		Body:          strings.Join(lines, "\n"),
		Hint:          reason + "\n" + hint,
		Width:         width,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
	})
}

// renderHeader draws "── Group ───…" across exactly inner cells.
func renderHeader(g Group) string {
	name := " " + g.String() + " "
	return ruleStyle.Render("──") + headerStyle.Render(name) +
		ruleStyle.Render(strings.Repeat("─", inner-2-ansi.StringWidth(name)))
}

// renderItem lays out "  Label ... key" at exactly inner cells, with a bar
// marking the cursor. A dim item drops its key; its reason shows under the
// list instead.
func (m Menu[T]) renderItem(it Item[T], selected bool) string {
	dim := it.Disabled != ""
	key := ""
	if !dim {
		key = ansi.Truncate(it.Key, inner/2, "…")
	}
	label := ansi.Truncate(it.Label, inner-2-ansi.StringWidth(key)-1, "…")
	gap := strings.Repeat(" ", inner-2-ansi.StringWidth(label)-ansi.StringWidth(key))
	marker := "  "
	if selected {
		marker = markerStyle.Render("▌ ")
	}
	if dim {
		label = dimStyle.Render(label)
	}
	line := marker + label + gap + keyStyle.Render(key)
	if selected {
		line = ui.RenderRowHighlight(line)
	}
	return line
}
