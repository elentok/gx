package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/agents/agentstyle"
)

// terminalView renders the terminal panel and measures its screen. It is
// read-only: it owns the saved last screen, the sessions dir and the panel
// memo, and reads everything else from in, which Model.termView fills.
type terminalView struct {
	// sessionsDir holds the sessions' state dirs; empty when unknown.
	sessionsDir string
	lastScreen  lastScreen
	mainMemo    *ui.Memo
	in          termInput
}

// termInput is what the view reads from the tab.
type termInput struct {
	settings                 ui.Settings
	spinner                  string
	afk                      bool
	now                      func() time.Time
	width, height, mainWidth int
	sessions                 []wire.SessionInfo
	cursor                   int
	attach                   *attachment
	copy                     *copyMode
	// copyRows is set only in copy mode.
	copyRows func([]string) []string
	// focused: the terminal has focus. covered: a popup or the leader overlay
	// sits on the screen.
	focused, covered bool
	selected         string
}

// loadLastScreen reads the selected Exited session's last screen off disk,
// unless it is already loaded. Only a TUI on the server's machine finds it.
func (v terminalView) loadLastScreen() tea.Cmd {
	if v.in.cursor >= len(v.in.sessions) || v.sessionsDir == "" {
		return nil
	}
	s := v.in.sessions[v.in.cursor]
	if s.Status.State != wire.SessionExited ||
		(v.lastScreen.id == s.ID && v.lastScreen.changedAt.Equal(s.StatusChangedAt)) {
		return nil
	}
	path := filepath.Join(v.sessionsDir, s.ID, holder.LastScreenFile)
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
func (v terminalView) terminalTitle(s wire.SessionInfo, titleStyle lipgloss.Style) string {
	icons := ui.Icons(v.in.settings.UseNerdFontIcons)
	title := agentstyle.StatusIcon(s, icons, v.in.spinner) + " " + titleStyle.Render(displayName(s)) + " " + agentstyle.StatusLabel(s)
	rest := ""
	if s.Ticket != "" {
		rest += " " + agentstyle.Ticket.Render(s.Ticket)
	}
	title += v.harnessLabelText(s, ansi.StringWidth(title)+ansi.StringWidth(rest)) + rest
	if a := v.in.attach; a != nil && a.id == s.ID && a.offset > 0 {
		badge := fmt.Sprintf("scrolled ↑ %d", a.offset)
		if a.newLines > 0 {
			badge += fmt.Sprintf("  +%d new", a.newLines)
		}
		title += " " + agentstyle.Detail.Render(badge)
	}
	if v.in.copy != nil && v.in.copy.a.id == s.ID {
		title += " " + agentstyle.Detail.Render("copy")
	}
	return title
}

// harnessLabelText is the session's dim ` harness · tier · model effort`,
// cut to the header room left beside the right title (used is the width of
// the title without it). Empty when a session has no harness or no room.
func (v terminalView) harnessLabelText(s wire.SessionInfo, used int) string {
	if s.Harness == "" {
		return ""
	}
	inner := v.in.mainWidth - 2
	room := inner - ansi.StringWidth(v.terminalRightTitle(s, inner)) - used - 2
	if room < 2 {
		return ""
	}
	label := ui.HarnessLabel(s.Harness, string(s.Tier), s.Model, s.Effort, s.ModelSwitched)
	return " " + agentstyle.Harness.Render(ansi.Truncate(label, room-1, "…"))
}

// terminalRightTitle is the AFK mark, project, dir and the time since the
// last status change; it takes at most half the header so the title stays
// readable. Truncation cuts the project side, never the AFK mark.
func (v terminalView) terminalRightTitle(s wire.SessionInfo, width int) string {
	afk := ""
	if v.in.afk {
		afk = " AFK ·"
	}
	// The leading space keeps a gap once the title is truncated against it.
	right := " " + s.Project + "  " + agentstyle.ShortDir(v.in.settings.Home(), s.Dir)
	if !s.StatusChangedAt.IsZero() {
		right += "  for " + ui.DurationCompact(v.in.now().Sub(s.StatusChangedAt))
	}
	if w, limit := ansi.StringWidth(right), width/2-len(afk); w > limit {
		right = ansi.TruncateLeft(right, w-limit+1, "…")
	}
	return afk + right
}

// terminalLines is the terminal panel body: the info row (then a blank row)
// and the screen.
func (v terminalView) terminalLines(s wire.SessionInfo) []string {
	lines := infoLines(s, v.in.now())
	live := v.in.attach
	if live != nil && live.id != s.ID {
		live = nil
	}
	if s.Status.State != wire.SessionExited && (live == nil || !live.exited) {
		switch {
		case live != nil && live.err != nil && !live.retrying:
			return append(lines, agentstyle.Reason.Render("attach failed: "+live.err.Error()))
		case live == nil || live.rows == nil:
			return append(lines, ui.StyleMuted.Render("live screen not attached yet"))
		}
		if v.in.copy != nil && v.in.copy.a == live {
			return append(lines, v.in.copyRows(live.rows)...)
		}
		return append(lines, live.rows...)
	}
	lines = append(lines, ui.StyleMuted.Render("<leader> r to resume"), "")
	if v.lastScreen.id != s.ID {
		return lines
	}
	for _, row := range v.lastScreen.rows {
		lines = append(lines, agentstyle.LastScreen.Render(ansi.Strip(row)))
	}
	return lines
}

// terminalCursor is the attached session's cursor on the panel, shown only
// while the terminal has focus, the live screen is on view and nothing covers
// it.
func (v terminalView) terminalCursor() *tea.Cursor {
	a := v.in.attach
	if !v.in.focused || !a.live() || a.offset > 0 || !a.cursor.Visible || v.in.copy != nil ||
		v.in.covered || v.in.selected != a.id {
		return nil
	}
	if cols, rows := v.screenSize(); a.cursor.X >= cols || a.cursor.Y >= rows {
		return nil
	}
	x, y := v.screenOrigin()
	return tea.NewCursor(x+a.cursor.X, y+a.cursor.Y)
}

// screenOrigin is where the session's screen starts on the page: past the
// sidebar and seam and the panel's padding, below its header and the info
// lines.
func (v terminalView) screenOrigin() (x, y int) {
	y = 1
	if v.in.cursor < len(v.in.sessions) {
		y += len(infoLines(v.in.sessions[v.in.cursor], v.in.now()))
	}
	return v.in.width - v.in.mainWidth + 1, y
}

// screenCell is the session's screen cell under mouse; false off the screen.
func (v terminalView) screenCell(mouse tea.Mouse) (x, y int, ok bool) {
	ox, oy := v.screenOrigin()
	cols, rows := v.screenSize()
	x, y = mouse.X-ox, mouse.Y-oy
	return x, y, x >= 0 && x < cols && y >= 0 && y < rows
}

// clampedCell is the screen cell nearest mouse, so a drag ends where it left
// the panel.
func (v terminalView) clampedCell(mouse tea.Mouse) (x, y int) {
	ox, oy := v.screenOrigin()
	cols, rows := v.screenSize()
	return max(0, min(mouse.X-ox, cols-1)), max(0, min(mouse.Y-oy, rows-1))
}

// screenSize is the panel's room for the session's screen: inside the border,
// below the info lines.
func (v terminalView) screenSize() (cols, rows int) {
	cols, rows = v.in.mainWidth-2, v.in.height-1
	if v.in.cursor < len(v.in.sessions) {
		rows -= len(infoLines(v.in.sessions[v.in.cursor], v.in.now()))
	}
	return max(cols, 1), max(rows, 1)
}

func (v terminalView) overPanel(mouse tea.Mouse) bool {
	left := v.in.width - v.in.mainWidth
	return mouse.X >= left && mouse.X < v.in.width && mouse.Y >= 0 && mouse.Y < v.in.height
}
