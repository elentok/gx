package components

import (
	"slices"
	"strings"

	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PickerHint is the picker's hint line; it shares the menu's keys.
var PickerHint = MenuHint

var stylePickerBadge = lipgloss.NewStyle().Foreground(ui.ColorRed)

type PickerItem struct {
	Label   string
	Value   string
	Group   string
	Badge   string
	Current bool
}

// PickerMatch is an item matching the filter, with the matched label runes.
type PickerMatch struct {
	Item PickerItem
	Pos  []int
}

// PickerState is a grouped, ranked fuzzy list. Unlike MenuState it reorders
// items by match score, so it is a separate component rather than a MenuState
// option. Cursor indexes Matches(), Offset is the first row shown and Height
// the rows shown (0 shows all).
type PickerState struct {
	Items  []PickerItem
	Cursor int
	Filter string
	Offset int
	Height int
}

// Matches returns the items matching Filter. With a filter, groups are ordered
// by their best score and items by score within a group, ties keeping the
// given order; with no filter, the given order.
func (s PickerState) Matches() []PickerMatch {
	type scored struct {
		PickerMatch
		score int
	}
	type group struct {
		name    string
		best    int
		matches []scored
	}
	var groups []*group
	for _, it := range s.Items {
		score, pos, ok := ui.FuzzyScore(s.Filter, it.Label)
		if !ok {
			continue
		}
		i := slices.IndexFunc(groups, func(g *group) bool { return g.name == it.Group })
		if i < 0 {
			groups = append(groups, &group{name: it.Group, best: score})
			i = len(groups) - 1
		}
		g := groups[i]
		g.best = max(g.best, score)
		g.matches = append(g.matches, scored{PickerMatch{Item: it, Pos: pos}, score})
	}
	slices.SortStableFunc(groups, func(a, b *group) int { return b.best - a.best })
	var out []PickerMatch
	for _, g := range groups {
		slices.SortStableFunc(g.matches, func(a, b scored) int { return b.score - a.score })
		for _, m := range g.matches {
			out = append(out, m.PickerMatch)
		}
	}
	return out
}

// Selected returns the item under the cursor; false when nothing matches.
func (s PickerState) Selected() (PickerItem, bool) {
	matches := s.Matches()
	if s.Cursor < 0 || s.Cursor >= len(matches) {
		return PickerItem{}, false
	}
	return matches[s.Cursor].Item, true
}

// pickerRow is a group header (match < 0) or a match.
type pickerRow struct {
	group string
	match int
}

// pickerRows gives each named group of matches a header row.
func pickerRows(matches []PickerMatch) []pickerRow {
	var out []pickerRow
	for i, m := range matches {
		if g := m.Item.Group; g != "" && (i == 0 || matches[i-1].Item.Group != g) {
			out = append(out, pickerRow{group: g, match: -1})
		}
		out = append(out, pickerRow{group: m.Item.Group, match: i})
	}
	return out
}

// scroll keeps the cursor, and its header when it is a group's first item,
// within the shown rows.
func (s PickerState) scroll(rows []pickerRow) PickerState {
	if s.Height <= 0 {
		s.Offset = 0
		return s
	}
	at := slices.IndexFunc(rows, func(r pickerRow) bool { return r.match == s.Cursor })
	if at < 0 {
		s.Offset = 0
		return s
	}
	top := at
	if at > 0 && rows[at-1].match < 0 {
		top--
	}
	s.Offset = max(min(s.Offset, top), at-s.Height+1)
	s.Offset = max(0, min(s.Offset, len(rows)-s.Height))
	return s
}

// shown is the slice of rows on screen.
func (s PickerState) shown(rows []pickerRow) []pickerRow {
	start := min(s.Offset, len(rows))
	end := len(rows)
	if s.Height > 0 {
		end = min(end, start+s.Height)
	}
	return rows[start:end]
}

// UpdatePicker returns (state, decided, accepted, handled), like UpdateMenu.
// Every printable key, digits included, is filter text. A filter change moves
// the cursor to the best match.
func UpdatePicker(msg tea.KeyPressMsg, state PickerState) (PickerState, bool, bool, bool) {
	n := len(state.Matches())
	state.Cursor = clampCursor(state.Cursor, n)

	switch msg.String() {
	case "ctrl+n", "down":
		if state.Cursor < n-1 {
			state.Cursor++
		}
	case "ctrl+p", "up":
		if state.Cursor > 0 {
			state.Cursor--
		}
	case "enter":
		return state, n > 0, n > 0, true
	case "esc":
		return state, true, false, true
	case "backspace":
		r := []rune(state.Filter)
		if len(r) == 0 {
			return state, false, false, true
		}
		state.Filter = string(r[:len(r)-1])
		state.Cursor, state.Offset = 0, 0
	default:
		if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			return state, false, false, false
		}
		state.Filter += msg.Text
		state.Cursor, state.Offset = 0, 0
	}
	return state.scroll(pickerRows(state.Matches())), false, false, true
}

// RenderPickerList renders the shown rows, each at most width cells.
func RenderPickerList(state PickerState, width int) string {
	matches := state.Matches()
	if len(matches) == 0 {
		return fitRow(ui.StyleDim.Render("  no matches"), width)
	}
	var lines []string
	for _, r := range state.shown(pickerRows(matches)) {
		if r.match < 0 {
			lines = append(lines, fitRow(ui.StyleMuted.Render(r.group), width))
			continue
		}
		m := matches[r.match]
		text := "  " + ui.HighlightMatches(m.Item.Label, m.Pos)
		if m.Item.Badge != "" {
			text += " " + stylePickerBadge.Render(m.Item.Badge)
		}
		if m.Item.Current {
			text += "  " + ui.StyleDim.Render("current")
		}
		row := fitRow(text, width)
		if r.match == state.Cursor {
			row = ui.RenderRowHighlight(row)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

// fitRow truncates or pads s to exactly width cells.
func fitRow(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	if pad := width - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// HitRow maps a line of the rendered list to a match index; false for a
// header, the no-matches row or a line past the list.
func (s PickerState) HitRow(line int) (int, bool) {
	shown := s.shown(pickerRows(s.Matches()))
	if line < 0 || line >= len(shown) || shown[line].match < 0 {
		return 0, false
	}
	return shown[line].match, true
}
