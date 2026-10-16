package agents

import (
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
	return title
}

// terminalRightTitle is project, dir and the time since the last status
// change; it takes at most half the header so the title stays readable.
func (m Model) terminalRightTitle(s server.SessionInfo, width int) string {
	// The leading space keeps a gap once the title is truncated against it.
	right := " " + s.Project + "  " + shortDir(s.Dir)
	if !s.StatusChangedAt.IsZero() {
		right += "  for " + ui.DurationCompact(m.now().Sub(s.StatusChangedAt))
	}
	if w, limit := ansi.StringWidth(right), width/2; w > limit {
		right = ansi.TruncateLeft(right, w-limit+1, "…")
	}
	return right
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
	var lines []string
	if info := infoRow(s); info != "" {
		lines = append(lines, info, "")
	}
	if s.Status.State != server.SessionExited {
		return append(lines, ui.StyleMuted.Render("live screen not attached yet"))
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

// infoRow is the Actionable reason with the prompts `<leader> a` would run,
// else the status detail.
func infoRow(s server.SessionInfo) string {
	if s.Actionable.Reason != "" {
		row := styleReason.Render(s.Actionable.Reason)
		if len(s.Actionable.Action) > 0 {
			row += styleAction.Render("  <leader> a: " + strings.Join(s.Actionable.Action, " → "))
		}
		return row
	}
	var parts []string
	for _, p := range []string{s.Status.Kind, s.Status.Message} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return styleDetail.Render(strings.Join(parts, ": "))
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
