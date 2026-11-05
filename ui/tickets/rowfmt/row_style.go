package rowfmt

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

var (
	// StatusOpenStyle deliberately has no Foreground override (ticket 02):
	// open is the default/no-signal state, so it renders in the terminal's
	// own default foreground rather than drawing the eye with a color.
	StatusOpenStyle = lipgloss.NewStyle()
	// StatusDraftStyle reads as paused, not idle: a draft is outstanding work
	// nobody can pick up, so it sits between open's default foreground and
	// done's near-invisible dim.
	StatusDraftStyle        = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	StatusClaimedStyle      = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	StatusBlockedStyle      = lipgloss.NewStyle().Foreground(ui.ColorRed)
	StatusPausedAnswerStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	// StatusWaitingForChildrenStyle uses the same color family as claimed —
	// this ticket's own work is finished, but the epic still has live work
	// underneath it, so it reads as active rather than settled/dim like done.
	StatusWaitingForChildrenStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	// StatusDoneStyle is deliberately dimmer than ui.StyleDim/StyleMuted
	// (used elsewhere for transient states like search-fade or loading
	// text): "done" is a permanent, low-priority state that should read as
	// clearly less prominent than everything else in the row, not merely
	// muted.
	StatusDoneStyle = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)

	// SuggestedActionBadgeStyle renders the "m"-menu badge (ticketHasSuggestedActions)
	// distinct from every status color so it reads as an affordance, not a
	// status signal.
	SuggestedActionBadgeStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
)

// TriangleColumnWidth is the fixed column width every row reserves for its
// triangle glyph (ticket 10), so a childless row's blank placeholder lines
// up with a sibling's actual triangle regardless of which collapse-state
// glyph (up or right) is wider.
func TriangleColumnWidth(icons ui.IconSet) int {
	w := lipgloss.Width(icons.TriangleExpanded)
	if cw := lipgloss.Width(icons.TriangleCollapsed); cw > w {
		w = cw
	}
	return w
}

// StatusIconAndStyle maps a ticket's rendered status to its dedicated glyph
// and color, distinct from the PRs tab's facet icon set.
func StatusIconAndStyle(icons ui.IconSet, status tickets.RenderedStatus) (string, lipgloss.Style) {
	switch status {
	case tickets.StatusDraft:
		return icons.TicketDraft, StatusDraftStyle
	case tickets.StatusOpen:
		return icons.TicketOpen, StatusOpenStyle
	case tickets.StatusClaimed:
		return icons.TicketClaimed, StatusClaimedStyle
	case tickets.StatusBlocked:
		return icons.TicketBlocked, StatusBlockedStyle
	case tickets.StatusPausedAnswer:
		return icons.TicketPausedAnswer, StatusPausedAnswerStyle
	case tickets.StatusPausedRepair:
		return icons.TicketPausedRepair, StatusPausedRepairStyle
	case tickets.StatusWaitingForChildren:
		return icons.TicketWaitingForChildren, StatusWaitingForChildrenStyle
	case tickets.StatusDone, tickets.StatusCancelled:
		return icons.TicketDone, StatusDoneStyle
	default: // tickets.StatusError
		return icons.TicketError, ui.StyleError
	}
}

// BlockedBySuffix renders the "(blocked by NN[, NN...])" suffix for a
// blocked/paused (answer) ticket, filtered to still-unresolved blockers. Empty
// for any other status or once every blocker has resolved.
func BlockedBySuffix(epic tickets.Epic, t tickets.Ticket, status tickets.RenderedStatus) string {
	if status != tickets.StatusBlocked && status != tickets.StatusPausedAnswer {
		return ""
	}
	unresolved := epic.UnresolvedBlockers(t)
	if len(unresolved) == 0 {
		return ""
	}
	if len(unresolved) > 3 {
		return fmt.Sprintf("(blocked by %d tickets)", len(unresolved))
	}
	for i, ref := range unresolved {
		unresolved[i] = t.BlockerLabel(ref)
	}
	return fmt.Sprintf("(blocked by %s)", strings.Join(unresolved, ", "))
}
