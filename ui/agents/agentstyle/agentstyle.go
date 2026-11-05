// Package agentstyle holds the styles and label helpers shared by the agents
// sidebar, terminal panel, jump list and create form.
package agentstyle

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui"
)

var (
	ProjectHeader = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	NameRead      = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	NameUnread    = lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	Ticket        = lipgloss.NewStyle().Foreground(ui.ColorOverlay)
	Background    = lipgloss.NewStyle().Foreground(ui.ColorTeal)
	UnreadMark    = lipgloss.NewStyle().Foreground(ui.ColorMauve)
	Actionable    = lipgloss.NewStyle().Foreground(ui.ColorRed)
	Design        = lipgloss.NewStyle().Foreground(ui.ColorMauve)

	Starting    = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	Working     = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	Idle        = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	Blocked     = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	RateLimited = lipgloss.NewStyle().Foreground(ui.ColorMauve)
	Crashed     = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)
	Exited      = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)

	Reason     = lipgloss.NewStyle().Foreground(ui.ColorRed)
	Action     = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	Detail     = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	LastScreen = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)
	Harness    = lipgloss.NewStyle().Foreground(ui.ColorOverlay)
)

// Fit truncates or pads s to exactly w cells.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// StatusIcon is the status glyph; spinnerView is the current spinner frame,
// shown while the session works.
func StatusIcon(s wire.SessionInfo, icons ui.IconSet, spinnerView string) string {
	switch s.Status.State {
	case wire.SessionStarting:
		return Starting.Render(icons.AgentStarting)
	case wire.SessionWorking:
		return Working.Render(spinnerView)
	case wire.SessionIdle:
		return Idle.Render(icons.TicketOpen)
	case wire.SessionBlocked:
		return Blocked.Render(icons.TicketPausedAnswer)
	case wire.SessionRateLimited:
		return RateLimited.Render(icons.TicketPaused)
	case wire.SessionExited:
		if s.Exit == wire.SessionExitCrashed {
			return Crashed.Render(icons.TicketError)
		}
		return Exited.Render(icons.AgentExited)
	}
	return " "
}

func StatusLabel(s wire.SessionInfo) string {
	switch s.Status.State {
	case wire.SessionStarting:
		return Starting.Render("starting")
	case wire.SessionWorking:
		if s.Status.Background {
			return Working.Render("working") + Background.Render(" +bg")
		}
		return Working.Render("working")
	case wire.SessionIdle:
		return Idle.Render("idle")
	case wire.SessionBlocked:
		return Blocked.Render("blocked")
	case wire.SessionRateLimited:
		return RateLimited.Render("rate-limited")
	case wire.SessionExited:
		if s.Exit == wire.SessionExitCrashed {
			return Crashed.Render("crashed")
		}
		return Exited.Render("exited")
	}
	return s.Status.State
}

func StatusDetail(s wire.SessionInfo) string {
	var parts []string
	for _, p := range []string{s.Status.Kind, s.Status.Message} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ": ")
}

func DisplayName(s wire.SessionInfo) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

// IsActionable: a Blocked session always waits on the person, with or
// without a recorded reason.
func IsActionable(s wire.SessionInfo) bool {
	return s.Actionable.Reason != "" || s.Status.State == wire.SessionBlocked
}

// Marks fills the Unread and Actionable slots; Queue and Design agents leave
// them blank, their status mark is all a row shows.
func Marks(s wire.SessionInfo, icons ui.IconSet) string {
	unread, actionable := " ", " "
	if s.Queue || s.Design {
		return unread + " " + actionable + " "
	}
	if s.Unread.Unread {
		unread = UnreadMark.Render(icons.Unread)
	}
	if IsActionable(s) {
		actionable = Actionable.Render(icons.SuggestedAction)
	}
	return unread + " " + actionable + " "
}

func ShortDir(home, dir string) string {
	if home == "" {
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
