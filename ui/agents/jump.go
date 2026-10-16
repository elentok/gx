package agents

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
)

const jumpMaxWidth = 100

var jumpHint = ui.RenderInlineBindings(
	key.NewBinding(key.WithHelp("ctrl+n/ctrl+p or ↑/↓", "navigate")),
	key.NewBinding(key.WithHelp("type", "filter")),
	key.NewBinding(key.WithHelp("tab", "order")),
	key.NewBinding(key.WithHelp("enter", "jump")),
	key.NewBinding(key.WithHelp("esc", "close")),
)

// jumpPopup is the jump-to-agent popup; cursor indexes its matches.
type jumpPopup struct {
	query        string
	projectOrder bool
	cursor       int
}

func (j *jumpPopup) attention() bool { return !j.projectOrder }

// jumpMatches is the indexes into m.sessions the popup lists: filtered, and
// in attention order (Actionable, Unread, the rest) unless toggled to the
// sidebar's project order.
func (m Model) jumpMatches() []int {
	var out []int
	for i, s := range m.sessions {
		if ui.FuzzyMatch(m.jump.query, displayName(s), s.Project, s.Dir) {
			out = append(out, i)
		}
	}
	if m.jump.attention() {
		slices.SortStableFunc(out, func(a, b int) int {
			return attentionBucket(m.sessions[a]) - attentionBucket(m.sessions[b])
		})
	}
	return out
}

func attentionBucket(s server.SessionInfo) int {
	switch {
	case isActionable(s):
		return 0
	case s.Unread.Unread:
		return 1
	}
	return 2
}

func (m Model) updateJumpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	j := *m.jump
	m.jump = &j
	matches := m.jumpMatches()
	switch msg.String() {
	case "ctrl+n", "down":
		j.cursor = min(j.cursor+1, max(0, len(matches)-1))
	case "ctrl+p", "up":
		j.cursor = max(j.cursor-1, 0)
	case "tab":
		j.projectOrder = !j.projectOrder
		j.cursor = 0
	case "esc":
		m.jump = nil
	case "enter":
		if j.cursor >= len(matches) {
			return m, nil
		}
		m.jump = nil
		return m.selectAndFocus(matches[j.cursor])
	case "backspace":
		if r := []rune(j.query); len(r) > 0 {
			j.query = string(r[:len(r)-1])
			j.cursor = 0
		}
	default:
		if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			j.query += msg.Text
			j.cursor = 0
		}
	}
	return m, nil
}

func (m Model) renderJump() string {
	width := min(jumpMaxWidth, m.width-4)
	innerW := width - 4
	matches := m.jumpMatches()
	order := "attention"
	if !m.jump.attention() {
		order = "project"
	}
	lines := []string{ui.StyleMuted.Render("> ") + m.jump.query}
	if len(matches) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no matches"))
	}
	// Room for the border, query line, the gaps and the hint.
	bodyH := max(1, m.height-7)
	offset := max(0, m.jump.cursor-bodyH+1)
	for n, i := range matches[offset:min(len(matches), offset+bodyH)] {
		row := m.jumpRow(m.sessions[i], innerW)
		if offset+n == m.jump.cursor {
			row = ui.RenderRowHighlight(row)
		}
		lines = append(lines, row)
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Jump to agent",
		RightTitle:    order + " · " + strconv.Itoa(len(matches)) + "/" + strconv.Itoa(len(m.sessions)),
		Body:          strings.Join(lines, "\n"),
		Hint:          jumpHint,
		Width:         width,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
		TitleInBorder: true,
	})
}

// jumpRow is status icon, marks, name, project, dir, the time since the last
// status change, status label and the Actionable reason or status detail.
func (m Model) jumpRow(s server.SessionInfo, w int) string {
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	style := styleNameRead
	if s.Unread.Unread {
		style = styleNameUnread
	}
	since := ""
	if !s.StatusChangedAt.IsZero() {
		since = ui.DurationCompact(m.now().Sub(s.StatusChangedAt))
	}
	// Some status glyphs render 2 cells wide; a fixed slot keeps columns aligned.
	row := fit(m.statusIcon(s, icons), 2) + " " + marks(s, icons) +
		fit(style.Render(displayName(s)), 18) + " " +
		fit(styleProjectHeader.Render(s.Project), 12) + " " +
		fit(ui.StyleMuted.Render(shortDir(s.Dir)), 20) + " " +
		fit(ui.StyleMuted.Render(since), 5) + " " +
		statusLabel(s)
	if s.Actionable.Reason != "" {
		row += " " + styleReason.Render(s.Actionable.Reason)
	} else if d := statusDetail(s); d != "" {
		row += " " + styleDetail.Render(d)
	}
	return fit(row, w)
}
