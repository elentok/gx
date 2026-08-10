package tickets

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

var (
	// epicHeaderStyle highlights the Queue tab's per-epic header name in the
	// same blue used elsewhere in this UI's palette (statusClaimedStyle),
	// rather than sectionHeaderStyle's neutral divider color.
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

	// epicStatusParkedStyle colors a parked epic's header status line
	// distinctly from both the yellow "problem" and default "running" looks,
	// so parked reads as its own state rather than an alarm or normal
	// progress (see reduceLiveEvent's RunStateParked).
	epicStatusParkedStyle = lipgloss.NewStyle().Foreground(ui.ColorMauve)
)

// epicHeaderLines renders the Queue tab's per-epic header as two lines: a
// status line (an icon + status text, colored per epicStatusLine) and a
// context-window line (avg/max token usage plus a compact count across the
// epic's tickets). Both lines carry the same 2-char indent as the list rows
// beneath them (ticket 03) rather than the header widening to the list's old
// 4-char indent.
func (m QueueModel) epicHeaderLines(epic tickets.Epic, parked []ralphloop.StalledTicket) []string {
	icon, text, style := epicStatusLine(m.icons(), epic, parked)
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
func epicStatusLine(icons ui.IconSet, epic tickets.Epic, parked []ralphloop.StalledTicket) (icon, text string, style lipgloss.Style) {
	switch {
	case len(parked) > 0:
		return icons.Warning, "parked — " + parkedStallText(parked), epicStatusParkedStyle
	case epic.AllDone():
		text := "took " + formatElapsed(epicElapsedSeconds(epic))
		if dur, ok := epic.CompletionDuration(); ok {
			text = "took " + formatDuration(dur)
		}
		return icons.TicketDone, text, epicStatusDoneStyle
	case epicHasProblem(epic):
		return icons.Warning, fmt.Sprintf("%d of %d done", epic.DoneCount(), epic.TotalCount()), epicStatusProblemStyle
	default:
		return icons.Dot, fmt.Sprintf("%d of %d done", epic.DoneCount(), epic.TotalCount()), lipgloss.NewStyle()
	}
}

// parkedStallText renders a parked epic's stall reason for the header status
// line: "waiting on <id>[, <id>...]", with "(reattachable)" appended per
// ticket for the ones whose prior iteration still owns a live herdr
// tab/agent (StalledTicket.Reattachable — see resumeReattachable) — the "jump
// to pane" acceptance criterion resolved as informational rendering, not a
// new navigation action, since no jump-to-pane affordance exists anywhere in
// this UI today (see ticket 11a4's "Open question").
func parkedStallText(stalled []ralphloop.StalledTicket) string {
	names := make([]string, len(stalled))
	for i, s := range stalled {
		name := s.Identifier
		if s.Reattachable {
			name += " (reattachable)"
		}
		names[i] = name
	}
	return "waiting on " + strings.Join(names, ", ")
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

// queueRunStateKind classifies the queue's run state for header rendering.
// queueHeaderTitle and queueHeaderBodyLines both switch on the same
// queueRunState() call so they can't independently re-derive (and silently
// diverge on) what state the queue is in.
type queueRunStateKind int

const (
	queueRunIdle queueRunStateKind = iota
	queueRunRunning
	queueRunPaused
	queueRunParked
	queueRunCompleted
)

// queueRunState classifies the queue's current run state. Paused only wins
// over idle once a run has actually captured a ticket scope
// (m.checkedProgress total > 0) — m.paused alone can be set by the bare `p`
// key with no run-state guard, so a queue that was never started must still
// classify as idle even while globally paused.
//
// Parked wins over running: park is tracked per run (loopRegistry.parkedEpics)
// independently of m.runningEpics, so a queue can have some epics actively
// running and others parked at the same time. A parked epic needs a human to
// look at it, so the header leads with that rather than the generic
// "implementing..." text.
func (m QueueModel) queueRunState() queueRunStateKind {
	if !m.executionCompletedAt.IsZero() {
		done, total := m.completedExecutionProgress()
		if total > 0 && done == total {
			return queueRunCompleted
		}
	}
	if m.paused {
		if _, total := m.checkedProgress(); total > 0 {
			return queueRunPaused
		}
	}
	if len(ralphLoopRegistry.parkedEpics()) > 0 {
		return queueRunParked
	}
	if len(m.runningEpics) > 0 {
		return queueRunRunning
	}
	return queueRunIdle
}

// lowestParkedEpicAndTicket picks a deterministic (epic, ticket) pair to name
// in the header title when one or more epics are parked: the lowest parked
// epic name, and within it the lowest ticket identifier among its stalled
// tickets — so the title doesn't flicker between different tickets across
// renders due to map iteration order.
func (m QueueModel) lowestParkedEpicAndTicket() (epicName, ticketID string, ok bool) {
	parked := ralphLoopRegistry.parkedEpics()
	if len(parked) == 0 {
		return "", "", false
	}
	names := make([]string, 0, len(parked))
	for name := range parked {
		names = append(names, name)
	}
	sort.Strings(names)
	epicName = names[0]
	for _, s := range parked[epicName] {
		if ticketID == "" || s.Identifier < ticketID {
			ticketID = s.Identifier
		}
	}
	return epicName, ticketID, true
}

// queueHeaderTitle and queueHeaderBodyLines together implement the Option B
// header redesign (see .scratch/tickets-queue-batch3/issues/assets/08-header-prototype.md):
// the title always encodes run state, and the body carries at most one
// state-specific line instead of the old always-present banner row.
func (m QueueModel) queueHeaderTitle() string {
	if m.foreignAttachPID != 0 {
		return fmt.Sprintf("Queue · attached to gx pid %d", m.foreignAttachPID)
	}
	switch m.queueRunState() {
	case queueRunCompleted:
		elapsed := int(m.executionCompletedAt.Sub(m.executionStartedAt).Seconds())
		return fmt.Sprintf("Queue · done, took %s", formatElapsed(elapsed))
	case queueRunPaused:
		done, total := m.checkedProgress()
		return fmt.Sprintf("Queue · paused (%d of %d done)", done, total)
	case queueRunParked:
		if _, ticketID, ok := m.lowestParkedEpicAndTicket(); ok && ticketID != "" {
			return fmt.Sprintf("Queue · parked, waiting on %s", ticketID)
		}
		return "Queue · parked"
	case queueRunRunning:
		done, total := m.checkedProgress()
		glyph := strings.TrimRight(m.implementSpinner.View(), " ")
		return fmt.Sprintf("Queue · %d of %d done · %s implementing...", done, total, glyph)
	default:
		return "Queue"
	}
}

func (m QueueModel) queueHeaderBodyLines() []string {
	switch m.queueRunState() {
	case queueRunCompleted:
		total, average, maximum := m.completedContextMetrics()
		return []string{fmt.Sprintf(
			"context windows: total %s, avg %s, max %s",
			formatTokenCount(total), formatTokenCount(average), formatTokenCount(maximum),
		)}
	case queueRunPaused:
		if len(m.runningEpics) > 0 {
			return []string{"Queue paused — in-flight iterations will finish"}
		}
		return nil
	case queueRunParked:
		// The title (queueHeaderTitle) already names the parked epic/ticket;
		// no separate body line needed.
		return nil
	default:
		return nil
	}
}

func (m QueueModel) completedContextMetrics() (total, average, maximum int) {
	count := 0
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			count++
			total += ticket.ActualContextWindow
			maximum = max(maximum, ticket.ActualContextWindow)
		}
	}
	if count > 0 {
		average = total / count
	}
	return total, average, maximum
}
