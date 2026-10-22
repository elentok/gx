package agents

import (
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
)

const sidebarWidth = 30

// markSlotsWidth is a gap cell after the name, then the Unread and
// Actionable slots, 2 cells each, so a wide-rendering glyph never squeezes
// its neighbour and names stay aligned.
const markSlotsWidth = 5

var (
	styleProjectHeader = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	styleNameRead      = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	styleNameUnread    = lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	styleTicket        = lipgloss.NewStyle().Foreground(ui.ColorOverlay)
	styleBackground    = lipgloss.NewStyle().Foreground(ui.ColorTeal)
	styleUnreadMark    = lipgloss.NewStyle().Foreground(ui.ColorMauve)
	styleActionable    = lipgloss.NewStyle().Foreground(ui.ColorRed)
	styleDesign        = lipgloss.NewStyle().Foreground(ui.ColorMauve)

	styleStarting    = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	styleWorking     = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	styleIdle        = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	styleBlocked     = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	styleRateLimited = lipgloss.NewStyle().Foreground(ui.ColorMauve)
	styleCrashed     = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)
	styleExited      = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)
)

// sidebarRow is one rendered sidebar line; session is -1 for project headers
// and group gaps.
type sidebarRow struct {
	text    string
	session int
}

// orderedSessions groups sessions by project, projects in order of their
// first session, keeping creation order inside each group so rows never jump.
func orderedSessions(sessions []server.SessionInfo) []server.SessionInfo {
	var projects []string
	byProject := map[string][]server.SessionInfo{}
	for _, s := range sessions {
		if _, ok := byProject[s.Project]; !ok {
			projects = append(projects, s.Project)
		}
		byProject[s.Project] = append(byProject[s.Project], s)
	}
	out := make([]server.SessionInfo, 0, len(sessions))
	for _, p := range projects {
		out = append(out, byProject[p]...)
	}
	return out
}

func liveCount(sessions []server.SessionInfo) int {
	n := 0
	for _, s := range sessions {
		if s.Status.State != server.SessionExited {
			n++
		}
	}
	return n
}

func (m Model) sidebarRows(innerW int) []sidebarRow {
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	var rows []sidebarRow
	for i, s := range m.sessions {
		if i == 0 || s.Project != m.sessions[i-1].Project {
			if i > 0 {
				rows = append(rows, sidebarRow{session: -1})
			}
			header := styleProjectHeader.Render(icons.FolderClosed + " " + s.Project)
			rows = append(rows, sidebarRow{text: header, session: -1})
		}
		row := fit(" "+m.statusIcon(s, icons)+" "+sessionLabel(s, icons), innerW-markSlotsWidth) + " " + marks(s, icons)
		if i == m.cursor {
			row = ui.RenderRowHighlight(row)
		}
		rows = append(rows, sidebarRow{text: row, session: i})
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
	if m.forwardPress(mouse) {
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
	return ui.RenderPanel(ui.PanelOptionsFor(sidebarWidth, m.height, title, live, lines, active, titleColor, titleColor, true))
}

// headerColors are the title colour and header fill of a focused or
// unfocused panel.
func headerColors(active bool) (title, bg color.Color) {
	if active {
		return ui.ColorOrange, ui.ColorSurface1
	}
	return ui.ColorMauve, ui.ColorSurface
}

func (m Model) statusIcon(s server.SessionInfo, icons ui.IconSet) string {
	switch s.Status.State {
	case server.SessionStarting:
		return styleStarting.Render(icons.AgentStarting)
	case server.SessionWorking:
		return styleWorking.Render(m.spinner.View())
	case server.SessionIdle:
		return styleIdle.Render(icons.TicketOpen)
	case server.SessionBlocked:
		return styleBlocked.Render(icons.TicketPausedAnswer)
	case server.SessionRateLimited:
		return styleRateLimited.Render(icons.TicketPaused)
	case server.SessionExited:
		if s.Exit == server.SessionExitCrashed {
			return styleCrashed.Render(icons.TicketError)
		}
		return styleExited.Render(icons.AgentExited)
	}
	return " "
}

// sessionLabel is the Design mark, the name (bold when Unread), the teal +bg
// while a background task runs, and the Session ticket.
func sessionLabel(s server.SessionInfo, icons ui.IconSet) string {
	style := styleNameRead
	if s.Unread.Unread {
		style = styleNameUnread
	}
	label := style.Render(displayName(s))
	if s.Design {
		label = styleDesign.Render(icons.DesignSession) + " " + label
	}
	if s.Status.State == server.SessionWorking && s.Status.Background {
		label += styleBackground.Render(" +bg")
	}
	if s.Ticket != "" {
		label += " " + styleTicket.Render(s.Ticket)
	}
	return label
}

func marks(s server.SessionInfo, icons ui.IconSet) string {
	unread, actionable := " ", " "
	if s.Unread.Unread {
		unread = styleUnreadMark.Render(icons.Unread)
	}
	if isActionable(s) {
		actionable = styleActionable.Render(icons.SuggestedAction)
	}
	return unread + " " + actionable + " "
}

// isActionable: a Blocked session always waits on the person, with or
// without a recorded reason.
func isActionable(s server.SessionInfo) bool {
	return s.Actionable.Reason != "" || s.Status.State == server.SessionBlocked
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
