package tickets

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

var (
	// epicHeaderStyle highlights the Queue tab's per-epic header name in the
	// same blue used elsewhere in this UI's palette (statusClaimedStyle,
	// worktreeTagStyle), rather than sectionHeaderStyle's neutral divider
	// color.
	epicHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)

	// epicStatusDoneStyle colors an epic header's status line green once
	// every ticket is done.
	epicStatusDoneStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen)

	// epicStatusProblemStyle colors an epic header's status line yellow when
	// any of its tickets is needs-info/needs-attention/error-classed.
	// "In progress, clean" deliberately falls through to the default/no-color
	// treatment instead (ticket 02's same open=no-color choice), so it
	// doesn't read as an alarm state.
	epicStatusProblemStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)
)

// epicHeaderLines renders the Queue tab's per-epic header as two lines: a
// status line (an icon + status text, colored per epicStatusLine) and a
// context-window line (avg/max token usage plus a compact count across the
// epic's tickets). Both lines carry the same 2-char indent as the list rows
// beneath them (ticket 03) rather than the header widening to the list's old
// 4-char indent.
func (m QueueModel) epicHeaderLines(epic tickets.Epic) []string {
	icon, text, style := epicStatusLine(m.icons(), epic)
	statusLine := "  " + epicHeaderStyle.Render(epic.Name) + " " + style.Render(icon+" "+text)

	avg, maximum, compacts := epicContextMetrics(epic)
	contextLine := "  " + metricsLineStyle.Render(fmt.Sprintf(
		"Context window: avg %s, max %s (%d compacts)",
		formatTokenCount(avg), formatTokenCount(maximum), compacts,
	))
	return []string{statusLine, contextLine}
}

// epicStatusLine picks an epic header's status-line icon, text, and color:
// green "took <elapsed>" once every ticket is done, yellow flagging any
// needs-info/needs-attention/error-classed ticket, or the default/no-color
// treatment otherwise.
func epicStatusLine(icons ui.IconSet, epic tickets.Epic) (icon, text string, style lipgloss.Style) {
	switch {
	case epic.AllDone():
		return icons.TicketDone, "took " + formatElapsed(epicElapsedSeconds(epic)), epicStatusDoneStyle
	case epicHasProblem(epic):
		return icons.Warning, fmt.Sprintf("%d of %d done", epic.DoneCount(), epic.TotalCount()), epicStatusProblemStyle
	default:
		return icons.Dot, fmt.Sprintf("%d of %d done", epic.DoneCount(), epic.TotalCount()), lipgloss.NewStyle()
	}
}

// epicElapsedSeconds sums the epic's tickets' landed ElapsedTime, for the
// header status line's "took <elapsed>" once the epic is fully done.
func epicElapsedSeconds(epic tickets.Epic) int {
	total := 0
	for _, t := range epic.Tickets {
		total += t.ElapsedTime
	}
	return total
}

// epicHasProblem reports whether any of the epic's tickets renders as
// needs-info/needs-attention/error — the header status line's yellow trigger.
func epicHasProblem(epic tickets.Epic) bool {
	for _, t := range epic.Tickets {
		switch epic.RenderedStatus(t) {
		case tickets.StatusNeedsInfo, tickets.StatusNeedsAttention, tickets.StatusError:
			return true
		}
	}
	return false
}

// epicContextMetrics computes the header's context-window line figures:
// avg/max landed context window across tickets that have landed one, plus
// the epic's total compaction count.
func epicContextMetrics(epic tickets.Epic) (avg, maximum, compacts int) {
	total := 0
	count := 0
	for _, t := range epic.Tickets {
		compacts += t.Compactions
		if t.ActualContextWindow == 0 {
			continue
		}
		total += t.ActualContextWindow
		maximum = max(maximum, t.ActualContextWindow)
		count++
	}
	if count > 0 {
		avg = total / count
	}
	return avg, maximum, compacts
}
