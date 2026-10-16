package agents

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
)

const sidebarWidth = 30

// markSlotsWidth is the Unread and Actionable slots, 2 cells each, so a
// wide-rendering glyph never squeezes its neighbour and names stay aligned.
const markSlotsWidth = 4

var (
	styleProjectHeader = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	styleNameRead      = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	styleNameUnread    = lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	styleTicket        = lipgloss.NewStyle().Foreground(ui.ColorOverlay)
	styleBackground    = lipgloss.NewStyle().Foreground(ui.ColorTeal)
	styleUnreadMark    = lipgloss.NewStyle().Foreground(ui.ColorMauve)
	styleActionable    = lipgloss.NewStyle().Foreground(ui.ColorRed)

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
		row := fit(" "+m.statusIcon(s, icons)+" "+sessionLabel(s), innerW-markSlotsWidth) + marks(s, icons)
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

func (m Model) renderSidebar() string {
	visible := m.visibleRows()
	lines := make([]string, len(visible))
	for i, r := range visible {
		lines[i] = r.text
	}
	active := !m.terminalFocused
	titleColor := ui.ColorMauve
	headerBg := ui.ColorSurface
	if active {
		titleColor, headerBg = ui.ColorOrange, ui.ColorSurface1
	}
	title := ui.RenderRowWithBackground(lipgloss.NewStyle().Foreground(titleColor).Bold(active).Render("Agents"), headerBg)
	live := strconv.Itoa(liveCount(m.sessions)) + " live"
	return ui.RenderPanel(ui.PanelOptionsFor(sidebarWidth, m.height, title, live, lines, active, titleColor, titleColor, true))
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
		return styleBlocked.Render(icons.TicketNeedsAnswer)
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

// sessionLabel is the name (bold when Unread), the teal +bg while a
// background task runs, and the Session ticket.
func sessionLabel(s server.SessionInfo) string {
	style := styleNameRead
	if s.Unread.Unread {
		style = styleNameUnread
	}
	label := style.Render(displayName(s))
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
