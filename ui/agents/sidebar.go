package agents

import (
	"image/color"
	"strconv"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/agents/agentstyle"
)

const sidebarWidth = 30

// markSlotsWidth is a gap cell after the name, then the Unread and
// Actionable slots, 2 cells each, so a wide-rendering glyph never squeezes
// its neighbour and names stay aligned.
const markSlotsWidth = 5

// sidebarRow is one rendered sidebar line; session is -1 for project headers
// and group gaps.
type sidebarRow struct {
	text    string
	session int
}

// orderedSessions lists regular sessions, then Queue agents, then Design
// agents; each part is grouped by project, projects in order of their first
// session, keeping creation order inside each group so rows never jump.
func orderedSessions(sessions []wire.SessionInfo) []wire.SessionInfo {
	out := make([]wire.SessionInfo, 0, len(sessions))
	for _, g := range []agentGroup{groupRegular, groupQueue, groupDesign} {
		var projects []string
		byProject := map[string][]wire.SessionInfo{}
		for _, s := range sessions {
			if groupOf(s) != g {
				continue
			}
			if _, ok := byProject[s.Project]; !ok {
				projects = append(projects, s.Project)
			}
			byProject[s.Project] = append(byProject[s.Project], s)
		}
		for _, p := range projects {
			out = append(out, byProject[p]...)
		}
	}
	return out
}

func liveCount(sessions []wire.SessionInfo) int {
	n := 0
	for _, s := range sessions {
		if s.Status.State != wire.SessionExited {
			n++
		}
	}
	return n
}

func (m Model) sidebarRows(innerW int) []sidebarRow {
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	var rows []sidebarRow
	header := func(text string) {
		if len(rows) > 0 {
			rows = append(rows, sidebarRow{session: -1})
		}
		rows = append(rows, sidebarRow{text: agentstyle.ProjectHeader.Render(icons.FolderClosed + " " + text), session: -1})
	}
	// prev is the last session listed, to know when a header starts.
	var prev *wire.SessionInfo
	for i, s := range m.sessions {
		if !m.onSidebar(i) {
			continue
		}
		switch g := groupOf(s); {
		case prev == nil || groupOf(*prev) != g:
			if g == groupRegular {
				header(s.Project)
			} else {
				header(g.title() + "s")
			}
		case g == groupRegular && s.Project != prev.Project:
			header(s.Project)
		}
		prev = &m.sessions[i]
		row := agentstyle.Fit(" "+agentstyle.StatusIcon(s, icons, m.spinner.View())+" "+sessionLabel(s, icons), innerW-markSlotsWidth) + " " + marks(s, icons)
		if i == m.cursor {
			row = ui.RenderRowHighlight(row)
		}
		rows = append(rows, sidebarRow{text: row, session: i})
	}
	for _, g := range []agentGroup{groupQueue, groupDesign} {
		if n := m.hiddenCount(g); n > 0 {
			if len(rows) > 0 {
				rows = append(rows, sidebarRow{session: -1})
			}
			rows = append(rows, hiddenRow(g, n))
		}
	}
	return rows
}

// visibleRows is the window of rows the sidebar body shows, scrolled just
// enough to keep the cursor on screen.
func (m Model) visibleRows() []sidebarRow {
	rows := m.sidebarRows(sidebarWidth - 2)
	bodyH := m.height - 1
	if m.height <= 0 || len(rows) <= bodyH {
		return rows
	}
	cursorRow := 0
	for i, r := range rows {
		if r.session == m.cursor {
			cursorRow = i
		}
	}
	offset := max(0, cursorRow-bodyH+1)
	return rows[offset : offset+bodyH]
}

// updateClick selects the session clicked in the sidebar and focuses its
// terminal, as selecting it from the jump popup does; a click in the terminal
// focuses it. Popups and copy mode own the screen, so a click under them is
// dropped. The agent never sees a click, even one with mouse tracking on.
// A right click on a session opens its Context menu, as m does; while the
// menu is on top, it takes the click.
func (m Model) updateClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if m.menu.IsOpen && !m.confirm.IsOpen && !m.help.IsOpen {
		var cmd tea.Cmd
		m.menu, cmd = m.menu.UpdateMouse(msg, m.menuAnchor(), m.width, m.height)
		return m, cmd
	}
	if m.copy != nil || m.ModalOpen() || m.leaderPending {
		return m, nil
	}
	if m.attach.ForwardPress(mouse, m.termView().ScreenCell) {
		if mouse.Button == tea.MouseLeft {
			m.focusTerminal()
		}
		return m, nil
	}
	if mouse.Button == tea.MouseRight {
		return m.rightClick(mouse)
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if !m.sidebarOnScreen() || mouse.X > sidebarWidth {
		m.focusTerminal()
		return m, nil
	}
	i, ok := m.sidebarSessionAt(mouse.X, mouse.Y)
	if !ok {
		return m, nil
	}
	m.terminalFocused = false
	return m.selectAndFocus(i)
}

func (m Model) rightClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	i, ok := m.sidebarSessionAt(mouse.X, mouse.Y)
	if !ok {
		return m, nil
	}
	m.cursor = i
	next, cmd := m.refresh()
	m = next.(Model)
	m.terminalFocused = false
	next, menuCmd := m.openMenu()
	return next, tea.Batch(cmd, menuCmd)
}

// sidebarSessionAt is the session on the sidebar row under (x, y); false off
// the sidebar and on project headers and gaps.
func (m Model) sidebarSessionAt(x, y int) (int, bool) {
	if !m.sidebarOnScreen() || x < 0 || x >= sidebarWidth {
		return 0, false
	}
	// Row 0 is the panel's header.
	rows := m.visibleRows()
	if i := y - 1; i >= 0 && i < len(rows) && rows[i].session >= 0 {
		return rows[i].session, true
	}
	return 0, false
}

func (m Model) renderSidebar() string {
	visible := m.visibleRows()
	hovered, hovering := m.hoveredSession()
	lines := make([]string, len(visible))
	for i, r := range visible {
		lines[i] = r.text
		if hovering && r.session == hovered && hovered != m.cursor {
			lines[i] = ui.RenderRowHover(r.text)
		}
	}
	active := !m.terminalFocused
	titleColor, headerBg := headerColors(active)
	title := ui.RenderRowWithBackground(lipgloss.NewStyle().Foreground(titleColor).Bold(active).Render("Agents"), headerBg)
	live := strconv.Itoa(liveCount(m.sessions)) + " live"
	if m.afk {
		live = "AFK · " + live
	}
	opts := ui.PanelOptionsFor(sidebarWidth, m.height, title, live, lines, active, titleColor, titleColor, true)
	return m.sidebarMemo.Get(opts, func() string { return ui.RenderPanel(opts) })
}

// headerColors are the title colour and header fill of a focused or
// unfocused panel.
func headerColors(active bool) (title, bg color.Color) {
	if active {
		return ui.ColorOrange, ui.ColorSurface1
	}
	return ui.ColorMauve, ui.ColorSurface
}

// sessionLabel is the Design mark, the name (bold when Unread), the teal +bg
// while a background task runs, and the Session ticket.
func sessionLabel(s wire.SessionInfo, icons ui.IconSet) string {
	style := agentstyle.NameRead
	if s.Unread.Unread {
		style = agentstyle.NameUnread
	}
	label := style.Render(displayName(s))
	if s.Design {
		label = agentstyle.Design.Render(icons.DesignSession) + " " + label
	}
	if s.Status.State == wire.SessionWorking && s.Status.Background {
		label += agentstyle.Background.Render(" +bg")
	}
	if s.Ticket != "" {
		label += " " + agentstyle.Ticket.Render(s.Ticket)
	}
	return label
}

// marks fills the Unread and Actionable slots; Queue and Design agents leave
// them blank, their status mark is all a row shows.
func marks(s wire.SessionInfo, icons ui.IconSet) string {
	unread, actionable := " ", " "
	if s.Queue || s.Design {
		return unread + " " + actionable + " "
	}
	if s.Unread.Unread {
		unread = agentstyle.UnreadMark.Render(icons.Unread)
	}
	if isActionable(s) {
		actionable = agentstyle.Actionable.Render(icons.SuggestedAction)
	}
	return unread + " " + actionable + " "
}

// isActionable: a Blocked session always waits on the person, with or
// without a recorded reason.
func isActionable(s wire.SessionInfo) bool {
	return s.Actionable.Reason != "" || s.Status.State == wire.SessionBlocked
}
