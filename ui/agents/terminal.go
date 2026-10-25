package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
)

var (
	styleReason     = lipgloss.NewStyle().Foreground(ui.ColorRed)
	styleAction     = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	styleDetail     = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	styleLastScreen = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)
)

// lastScreen is an Exited session's saved last screen; changedAt tells a
// later exit of the same session (after a resume) from the one loaded.
type lastScreen struct {
	id        string
	changedAt time.Time
	rows      []string
}

type lastScreenMsg lastScreen

// loadLastScreen reads the selected Exited session's last screen off disk,
// unless it is already loaded. Only a TUI on the server's machine finds it.
func (m Model) loadLastScreen() tea.Cmd {
	if m.cursor >= len(m.sessions) || m.sessionsDir == "" {
		return nil
	}
	s := m.sessions[m.cursor]
	if s.Status.State != server.SessionExited ||
		(m.lastScreen.id == s.ID && m.lastScreen.changedAt.Equal(s.StatusChangedAt)) {
		return nil
	}
	path := filepath.Join(m.sessionsDir, s.ID, nativerunner.LastScreenFile)
	return func() tea.Msg {
		msg := lastScreenMsg{id: s.ID, changedAt: s.StatusChangedAt}
		if data, err := os.ReadFile(path); err == nil {
			msg.rows = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
		return msg
	}
}

// terminalTitle is the left of the terminal header: status icon, name,
// status label and Session ticket.
func (m Model) terminalTitle(s server.SessionInfo, titleStyle lipgloss.Style) string {
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	title := m.statusIcon(s, icons) + " " + titleStyle.Render(displayName(s)) + " " + statusLabel(s)
	if s.Ticket != "" {
		title += " " + styleTicket.Render(s.Ticket)
	}
	if a := m.attach; a != nil && a.id == s.ID && a.offset > 0 {
		badge := fmt.Sprintf("scrolled ↑ %d", a.offset)
		if a.newLines > 0 {
			badge += fmt.Sprintf("  +%d new", a.newLines)
		}
		title += " " + styleDetail.Render(badge)
	}
	if m.copy != nil && m.copy.a.id == s.ID {
		title += " " + styleDetail.Render("copy")
	}
	return title
}

// terminalRightTitle is the AFK mark, project, dir and the time since the
// last status change; it takes at most half the header so the title stays
// readable. Truncation cuts the project side, never the AFK mark.
func (m Model) terminalRightTitle(s server.SessionInfo, width int) string {
	afk := ""
	if m.afk {
		afk = " AFK ·"
	}
	// The leading space keeps a gap once the title is truncated against it.
	right := " " + s.Project + "  " + shortDir(s.Dir)
	if !s.StatusChangedAt.IsZero() {
		right += "  for " + ui.DurationCompact(m.now().Sub(s.StatusChangedAt))
	}
	if w, limit := ansi.StringWidth(right), width/2-len(afk); w > limit {
		right = ansi.TruncateLeft(right, w-limit+1, "…")
	}
	return afk + right
}

func statusLabel(s server.SessionInfo) string {
	switch s.Status.State {
	case server.SessionStarting:
		return styleStarting.Render("starting")
	case server.SessionWorking:
		if s.Status.Background {
			return styleWorking.Render("working") + styleBackground.Render(" +bg")
		}
		return styleWorking.Render("working")
	case server.SessionIdle:
		return styleIdle.Render("idle")
	case server.SessionBlocked:
		return styleBlocked.Render("blocked")
	case server.SessionRateLimited:
		return styleRateLimited.Render("rate-limited")
	case server.SessionExited:
		if s.Exit == server.SessionExitCrashed {
			return styleCrashed.Render("crashed")
		}
		return styleExited.Render("exited")
	}
	return s.Status.State
}

// terminalLines is the terminal panel body: the info row (then a blank row)
// and the screen.
func (m Model) terminalLines(s server.SessionInfo) []string {
	lines := infoLines(s, m.now())
	live := m.attach
	if live != nil && live.id != s.ID {
		live = nil
	}
	if s.Status.State != server.SessionExited && (live == nil || !live.exited) {
		switch {
		case live != nil && live.err != nil && !live.retrying:
			return append(lines, styleReason.Render("attach failed: "+live.err.Error()))
		case live == nil || live.rows == nil:
			return append(lines, ui.StyleMuted.Render("live screen not attached yet"))
		}
		if m.copy != nil && m.copy.a == live {
			return append(lines, m.copyRows(live.rows)...)
		}
		return append(lines, live.rows...)
	}
	lines = append(lines, ui.StyleMuted.Render("<leader> r to resume"), "")
	if m.lastScreen.id != s.ID {
		return lines
	}
	for _, row := range m.lastScreen.rows {
		lines = append(lines, styleLastScreen.Render(ansi.Strip(row)))
	}
	return lines
}

// terminalCursor is the attached session's cursor on the panel, shown only
// while the terminal has focus, the live screen is on view and nothing covers
// it.
func (m Model) terminalCursor() *tea.Cursor {
	a := m.attach
	if !m.terminalFocused || !a.live() || a.offset > 0 || !a.cursor.Visible || m.copy != nil ||
		m.ModalOpen() || m.leaderPending || m.selectedID() != a.id {
		return nil
	}
	if cols, rows := m.screenSize(); a.cursor.X >= cols || a.cursor.Y >= rows {
		return nil
	}
	x, y := m.screenOrigin()
	return tea.NewCursor(x+a.cursor.X, y+a.cursor.Y)
}

// screenOrigin is where the session's screen starts on the page: past the
// sidebar and seam and the panel's padding, below its header and the info
// lines.
func (m Model) screenOrigin() (x, y int) {
	y = 1
	if m.cursor < len(m.sessions) {
		y += len(infoLines(m.sessions[m.cursor], m.now()))
	}
	return m.width - m.mainWidth() + 1, y
}

// screenCell is the session's screen cell under mouse; false off the screen.
func (m Model) screenCell(mouse tea.Mouse) (x, y int, ok bool) {
	ox, oy := m.screenOrigin()
	cols, rows := m.screenSize()
	x, y = mouse.X-ox, mouse.Y-oy
	return x, y, x >= 0 && x < cols && y >= 0 && y < rows
}

// infoLines is the info row then a blank row, or none without an info row.
func infoLines(s server.SessionInfo, now time.Time) []string {
	if info := infoRow(s, now); info != "" {
		return []string{info, ""}
	}
	return nil
}

// screenSize is the panel's room for the session's screen: inside the border,
// below the info lines.
func (m Model) screenSize() (cols, rows int) {
	cols, rows = m.mainWidth()-2, m.height-1
	if m.cursor < len(m.sessions) {
		rows -= len(infoLines(m.sessions[m.cursor], m.now()))
	}
	return max(cols, 1), max(rows, 1)
}

// infoRow is the Actionable reason with the prompts `<leader> a` would run,
// else the status detail. A running auto-advance countdown ends it, with the
// way to cancel.
func infoRow(s server.SessionInfo, now time.Time) string {
	if s.Actionable.Reason != "" {
		row := styleReason.Render(s.Actionable.Reason)
		if len(s.Actionable.Action) > 0 {
			row += styleAction.Render("  <leader> a: " + strings.Join(s.Actionable.Action, " → "))
		}
		if s.AutoAdvanceAt != nil {
			left := max(s.AutoAdvanceAt.Sub(now), 0)
			row += styleDetail.Render(fmt.Sprintf("  Continuing in %ds", int((left+time.Second-1)/time.Second)))
			row += styleAction.Render("  Cancel: session menu (m)")
		}
		return row
	}
	if d := statusDetail(s); d != "" {
		return styleDetail.Render(d)
	}
	return ""
}

func statusDetail(s server.SessionInfo) string {
	var parts []string
	for _, p := range []string{s.Status.Kind, s.Status.Message} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ": ")
}

func shortDir(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return dir
}
