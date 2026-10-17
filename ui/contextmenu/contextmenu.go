// Package contextmenu is the shared Context menu: every action a person can
// take on one Target (a ticket row, an epic row, a session), grouped and
// fuzzy-filtered in an "Actions" modal.
package contextmenu

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
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
	Disabled string // non-empty = dim, shown in place of Key
	Run      func(T) tea.Cmd
}

const (
	title = "Actions"
	width = 56
	// inner fits a row inside the frame whether or not width counts the
	// border and padding, so a row never wraps and click rows stay aligned.
	inner = width - 4
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

// step moves the cursor by dir to the next enabled item; it stays put when
// there is none that way.
func (m Menu[T]) step(dir int) int {
	visible := m.visible()
	for i := m.cursor + dir; i >= 0 && i < len(visible); i += dir {
		if visible[i].Disabled == "" {
			return i
		}
	}
	return m.cursor
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

// listTop is the View line of the list's first row: top border, title,
// blank, prompt, blank and, while filtering, the filter line.
func (m Menu[T]) listTop() int {
	if m.filter != "" {
		return 6
	}
	return 5
}

// hitRow maps a View line to a visible item; false off an item row.
func (m Menu[T]) hitRow(y int) (int, bool) {
	rs := rows(m.visible())
	i := y - m.listTop()
	if i < 0 || i >= len(rs) || rs[i].item < 0 {
		return 0, false
	}
	return rs[i].item, true
}

func (m Menu[T]) View() string {
	visible := m.visible()
	var lines []string
	if m.filter != "" {
		lines = append(lines, ui.StyleDim.Render("filter: ")+m.filter)
	}
	if len(visible) == 0 {
		lines = append(lines, ui.StyleDim.Render("  no matching actions"))
	}
	for _, r := range rows(visible) {
		if r.item < 0 {
			lines = append(lines, ui.StyleMuted.Render(r.group.String()))
			continue
		}
		lines = append(lines, m.renderItem(visible[r.item], r.item == m.cursor))
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       title,
		Body:        ansi.Truncate(m.prompt, inner, "…") + "\n\n" + strings.Join(lines, "\n"),
		Hint:        components.MenuHint,
		Width:       width,
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		HintColor:   ui.ColorSubtle,
	})
}

// renderItem lays out "  Label ... key" at exactly inner cells; a dim item
// shows its reason where the key would be.
func (m Menu[T]) renderItem(it Item[T], selected bool) string {
	label, hint := "  "+it.Label, it.Key
	if it.Disabled != "" {
		hint = it.Disabled
	}
	hint = ansi.Truncate(hint, inner/2, "…")
	label = ansi.Truncate(label, inner-ansi.StringWidth(hint)-1, "…")
	gap := strings.Repeat(" ", inner-ansi.StringWidth(label)-ansi.StringWidth(hint))
	if it.Disabled != "" {
		label = ui.StyleDim.Render(label)
	}
	line := label + gap + ui.StyleDim.Render(hint)
	if selected && it.Disabled == "" {
		line = ui.RenderRowHighlight(line)
	}
	return line
}
