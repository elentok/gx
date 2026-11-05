package terminal

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

// View renders the terminal panel and measures its screen. It is
// read-only: it owns the saved last screen, the sessions dir and the panel
// memo, and reads everything else from its Input, set with WithInput.
type View struct {
	// SessionsDir holds the sessions' state dirs; empty when unknown.
	SessionsDir string
	LastScreen  LastScreen
	MainMemo    *ui.Memo
	in          Input
}

// Input is what the view reads from the tab.
type Input struct {
	Settings                 ui.Settings
	Spinner                  string
	AFK                      bool
	Now                      func() time.Time
	Width, Height, MainWidth int
	Sessions                 []wire.SessionInfo
	Cursor                   int
	Attach                   *Attachment
	Copy                     *CopyMode
	// CopyRows is set only in copy mode.
	CopyRows func([]string) []string
	// Focused: the terminal has focus. Covered: a popup or the leader overlay
	// sits on the screen.
	Focused, Covered bool
	Selected         string
	// DisplayName is a session's name in the header.
	DisplayName func(wire.SessionInfo) string
}

// WithInput is v reading in.
func (v View) WithInput(in Input) View {
	v.in = in
	return v
}

// LoadLastScreen reads the selected Exited session's last screen off disk,
// unless it is already loaded. Only a TUI on the server's machine finds it.
func (v View) LoadLastScreen() tea.Cmd {
	if v.in.Cursor >= len(v.in.Sessions) || v.SessionsDir == "" {
		return nil
	}
	s := v.in.Sessions[v.in.Cursor]
	if s.Status.State != wire.SessionExited ||
		(v.LastScreen.ID == s.ID && v.LastScreen.ChangedAt.Equal(s.StatusChangedAt)) {
		return nil
	}
	path := filepath.Join(v.SessionsDir, s.ID, holder.LastScreenFile)
	return func() tea.Msg {
		msg := LastScreenMsg{ID: s.ID, ChangedAt: s.StatusChangedAt}
		if data, err := os.ReadFile(path); err == nil {
			msg.Rows = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
		return msg
	}
}

// TerminalTitle is the left of the terminal header: status icon, name,
// status label and Session ticket.
func (v View) TerminalTitle(s wire.SessionInfo, titleStyle lipgloss.Style) string {
	icons := ui.Icons(v.in.Settings.UseNerdFontIcons)
	title := agentstyle.StatusIcon(s, icons, v.in.Spinner) + " " + titleStyle.Render(v.in.DisplayName(s)) + " " + agentstyle.StatusLabel(s)
	rest := ""
	if s.Ticket != "" {
		rest += " " + agentstyle.Ticket.Render(s.Ticket)
	}
	title += v.harnessLabelText(s, ansi.StringWidth(title)+ansi.StringWidth(rest)) + rest
	if a := v.in.Attach; a != nil && a.ID == s.ID && a.offset > 0 {
		badge := fmt.Sprintf("scrolled ↑ %d", a.offset)
		if a.newLines > 0 {
			badge += fmt.Sprintf("  +%d new", a.newLines)
		}
		title += " " + agentstyle.Detail.Render(badge)
	}
	if v.in.Copy != nil && v.in.Copy.A.ID == s.ID {
		title += " " + agentstyle.Detail.Render("copy")
	}
	return title
}

// harnessLabelText is the session's dim ` harness · tier · model effort`,
// cut to the header room left beside the right title (used is the width of
// the title without it). Empty when a session has no harness or no room.
func (v View) harnessLabelText(s wire.SessionInfo, used int) string {
	if s.Harness == "" {
		return ""
	}
	inner := v.in.MainWidth - 2
	room := inner - ansi.StringWidth(v.TerminalRightTitle(s, inner)) - used - 2
	if room < 2 {
		return ""
	}
	label := ui.HarnessLabel(s.Harness, string(s.Tier), s.Model, s.Effort, s.ModelSwitched)
	return " " + agentstyle.Harness.Render(ansi.Truncate(label, room-1, "…"))
}

// TerminalRightTitle is the AFK mark, project, dir and the time since the
// last status change; it takes at most half the header so the title stays
// readable. Truncation cuts the project side, never the AFK mark.
func (v View) TerminalRightTitle(s wire.SessionInfo, width int) string {
	afk := ""
	if v.in.AFK {
		afk = " AFK ·"
	}
	// The leading space keeps a gap once the title is truncated against it.
	right := " " + s.Project + "  " + agentstyle.ShortDir(v.in.Settings.Home(), s.Dir)
	if !s.StatusChangedAt.IsZero() {
		right += "  for " + ui.DurationCompact(v.in.Now().Sub(s.StatusChangedAt))
	}
	if w, limit := ansi.StringWidth(right), width/2-len(afk); w > limit {
		right = ansi.TruncateLeft(right, w-limit+1, "…")
	}
	return afk + right
}

// TerminalLines is the terminal panel body: the info row (then a blank row)
// and the screen.
func (v View) TerminalLines(s wire.SessionInfo) []string {
	lines := infoLines(s, v.in.Now())
	live := v.in.Attach
	if live != nil && live.ID != s.ID {
		live = nil
	}
	if s.Status.State != wire.SessionExited && (live == nil || !live.exited) {
		switch {
		case live != nil && live.err != nil && !live.retrying:
			return append(lines, agentstyle.Reason.Render("attach failed: "+live.err.Error()))
		case live == nil || live.Rows == nil:
			return append(lines, ui.StyleMuted.Render("live screen not attached yet"))
		}
		if v.in.Copy != nil && v.in.Copy.A == live {
			return append(lines, v.in.CopyRows(live.Rows)...)
		}
		return append(lines, live.Rows...)
	}
	lines = append(lines, ui.StyleMuted.Render("<leader> r to resume"), "")
	if v.LastScreen.ID != s.ID {
		return lines
	}
	for _, row := range v.LastScreen.Rows {
		lines = append(lines, agentstyle.LastScreen.Render(ansi.Strip(row)))
	}
	return lines
}

// TerminalCursor is the attached session's cursor on the panel, shown only
// while the terminal has focus, the live screen is on view and nothing covers
// it.
func (v View) TerminalCursor() *tea.Cursor {
	a := v.in.Attach
	if !v.in.Focused || !a.Live() || a.offset > 0 || !a.cursor.Visible || v.in.Copy != nil ||
		v.in.Covered || v.in.Selected != a.ID {
		return nil
	}
	if cols, rows := v.ScreenSize(); a.cursor.X >= cols || a.cursor.Y >= rows {
		return nil
	}
	x, y := v.ScreenOrigin()
	return tea.NewCursor(x+a.cursor.X, y+a.cursor.Y)
}

// ScreenOrigin is where the session's screen starts on the page: past the
// sidebar and seam and the panel's padding, below its header and the info
// lines.
func (v View) ScreenOrigin() (x, y int) {
	y = 1
	if v.in.Cursor < len(v.in.Sessions) {
		y += len(infoLines(v.in.Sessions[v.in.Cursor], v.in.Now()))
	}
	return v.in.Width - v.in.MainWidth + 1, y
}

// ScreenCell is the session's screen cell under mouse; false off the screen.
func (v View) ScreenCell(mouse tea.Mouse) (x, y int, ok bool) {
	ox, oy := v.ScreenOrigin()
	cols, rows := v.ScreenSize()
	x, y = mouse.X-ox, mouse.Y-oy
	return x, y, x >= 0 && x < cols && y >= 0 && y < rows
}

// ClampedCell is the screen cell nearest mouse, so a drag ends where it left
// the panel.
func (v View) ClampedCell(mouse tea.Mouse) (x, y int) {
	ox, oy := v.ScreenOrigin()
	cols, rows := v.ScreenSize()
	return max(0, min(mouse.X-ox, cols-1)), max(0, min(mouse.Y-oy, rows-1))
}

// ScreenSize is the panel's room for the session's screen: inside the border,
// below the info lines.
func (v View) ScreenSize() (cols, rows int) {
	cols, rows = v.in.MainWidth-2, v.in.Height-1
	if v.in.Cursor < len(v.in.Sessions) {
		rows -= len(infoLines(v.in.Sessions[v.in.Cursor], v.in.Now()))
	}
	return max(cols, 1), max(rows, 1)
}

func (v View) OverPanel(mouse tea.Mouse) bool {
	left := v.in.Width - v.in.MainWidth
	return mouse.X >= left && mouse.X < v.in.Width && mouse.Y >= 0 && mouse.Y < v.in.Height
}
