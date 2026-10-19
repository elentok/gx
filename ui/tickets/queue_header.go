package tickets

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// queueHeaderReservedLines is the constant height reserved for the Queue
// tab's run-state banner slot (queueHeaderBodyLines), whether or not a
// banner is actually active — the maximum line count any queueRunStateKind
// can emit. Keeping this a fixed constant, rather than deriving it from
// len(queueHeaderBodyLines()), is the point of ticket 21: m.queueTree's
// SetVisibleHeight only runs from the window-resize handler, so a banner
// that changed the body's line count mid-session (no resize in between)
// used to desync the tree's scroll/paging math.
const queueHeaderReservedLines = 1

var (
	// epicHeaderStyle highlights the Queue tab's per-epic header name in the
	// same blue used elsewhere in this UI's palette (statusClaimedStyle),
	// rather than sectionHeaderStyle's neutral divider color.
	epicHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)

	// epicStatusDoneStyle colors an epic header's status line green once
	// every ticket is done.
	epicStatusDoneStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen)

	// epicStatusProblemStyle colors an epic header's status line yellow when
	// any of its tickets is needs-answer/needs-repair/error-classed.
	// "In progress, clean" deliberately falls through to the default/no-color
	// treatment instead (ticket 02's same open=no-color choice), so it
	// doesn't read as an alarm state.
	epicStatusProblemStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)

	// epicStatusParkedAnswerStyle colors a parked epic's header status line
	// orange when every ticket a human still needs to act on is waiting on a
	// question (needs-answer) rather than a fault. This recolours the single
	// existing parked style into two variants instead of adding a third
	// visual style alongside it (see ticket 21's "recolour, not a third
	// style").
	epicStatusParkedAnswerStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)

	// epicStatusParkedRepairStyle colors a parked epic's header status line
	// red once any ticket a human still needs to act on is broken
	// (needs-repair) — red wins over orange when both kinds are present,
	// since a fault outranks a question.
	epicStatusParkedRepairStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
)

// epicStatusLine picks an epic header's status-line icon, text, and color.
// Precedence: done (green) > parked > needs attention (yellow) > implementing
// (spinnerGlyph, no color) > queued (no color). running is whether the server
// reports the epic running.
func epicStatusLine(icons ui.IconSet, epic tickets.Epic, parked []ralphloop.StalledTicket, running bool, spinnerGlyph string) (icon, text string, style lipgloss.Style) {
	if epic.AllDone() {
		text := "took " + formatElapsed(epicElapsedSeconds(epic))
		if dur, ok := epic.CompletionDuration(); ok {
			text = "took " + formatDuration(dur)
		}
		return icons.TicketDone, text, epicStatusDoneStyle
	}
	if waiting, parkedStyle := parkedWaitingAndStyle(epic, parked); len(waiting) > 0 {
		return icons.Warning, fmt.Sprintf("%d parked — %s", len(waiting), parkedStallText(waiting)), parkedStyle
	}
	switch {
	case epicHasProblem(epic):
		return icons.Warning, fmt.Sprintf("%d of %d done", epic.DoneCount(), epic.TotalCount()), epicStatusProblemStyle
	case running:
		return spinnerGlyph, "implementing", lipgloss.NewStyle()
	default:
		return icons.EpicQueued, "queued", lipgloss.NewStyle()
	}
}

// epicStatus is epicStatusLine for an epic on the Queue tab, with the model's
// running state and current spinner frame filled in.
func (m QueueModel) epicStatus(epic tickets.Epic) (icon, text string, style lipgloss.Style) {
	glyph := strings.TrimRight(m.implementSpinner.View(), " ")
	return epicStatusLine(m.icons(), epic, nil, m.runningEpics[epic.Name], glyph)
}

// parkedWaitingAndStyle filters parked down to the tickets a human still
// needs to act on and picks the colour for them: draft tickets are excluded
// (they're parked for scheduling — see StatusDraft — not waiting on a
// person, which is what this count is for), and the style is red if any
// remaining ticket is needs-repair, orange otherwise.
func parkedWaitingAndStyle(epic tickets.Epic, parked []ralphloop.StalledTicket) (waiting []ralphloop.StalledTicket, style lipgloss.Style) {
	anyRepair := false
	for _, s := range parked {
		status, ok := epicTicketStatus(epic, s.Identifier)
		if ok && status == tickets.StatusDraft {
			continue
		}
		waiting = append(waiting, s)
		if status == tickets.StatusNeedsRepair {
			anyRepair = true
		}
	}
	if anyRepair {
		return waiting, epicStatusParkedRepairStyle
	}
	return waiting, epicStatusParkedAnswerStyle
}

// epicTicketStatus looks up identifier's rendered status within epic. ok is
// false when identifier isn't one of the epic's own tickets, in which case
// the caller treats it as non-draft so it still counts.
func epicTicketStatus(epic tickets.Epic, identifier string) (status tickets.RenderedStatus, ok bool) {
	for _, t := range epic.Tickets {
		if t.Identifier == identifier {
			return epic.RenderedStatus(t), true
		}
	}
	return 0, false
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

// epicCost sums the epic's tickets' landed ActualCost, for the "took Xm
// $X.XX" cost summary appended alongside "took Xm" once the epic is fully
// done.
func epicCost(epic tickets.Epic) float64 {
	total := 0.0
	for _, t := range epic.Tickets {
		total += t.ActualCost
	}
	return total
}

// epicHasProblem reports whether any of the epic's tickets renders as
// needs-answer/needs-repair/error — the header status line's yellow trigger.
func epicHasProblem(epic tickets.Epic) bool {
	for _, t := range epic.Tickets {
		switch epic.RenderedStatus(t) {
		case tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair, tickets.StatusError:
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

// epicHeaderLayout is how every Queue epic header lays out its progress
// counter and bar. It is one value for all shown epics so the bars line up.
type epicHeaderLayout int

const (
	epicLayoutWide        epicHeaderLayout = iota // name … (n/m) bar on one line
	epicLayoutNarrowBar                           // name, then " bar (n/m)"
	epicLayoutNarrowCount                         // name, then " (n/m)"
)

const epicBarCells = 12

// queueLabelWidth is the width a Queue row's label can use: the body width,
// less the tree's selection mark and its two-column right margin. Zero or
// negative widths (before the first resize) mean "unconstrained".
func (m QueueModel) queueLabelWidth() int {
	sidebarW, _ := m.panelWidths(m.width)
	if m.width <= 0 {
		return 0
	}
	return sidebarW - 2 - 3
}

func epicCounter(epic tickets.Epic) string {
	return fmt.Sprintf("(%d/%d)", epic.DoneCount(), epic.TotalCount())
}

// epicHeaderLayoutFor picks the layout for all of epics: wide only if every
// non-empty epic's name line fits, else narrow with the bar if its own line
// fits, else narrow with only the counter.
func epicHeaderLayoutFor(epics []tickets.Epic, labelW int) epicHeaderLayout {
	if labelW <= 0 {
		return epicLayoutWide
	}
	layout := epicLayoutWide
	for _, epic := range epics {
		if epic.TotalCount() == 0 {
			continue
		}
		counterW := lipgloss.Width(epicCounter(epic))
		if 1+lipgloss.Width(epic.Name)+1+counterW+1+epicBarCells > labelW {
			layout = epicLayoutNarrowBar
		}
		if 1+epicBarCells+1+counterW > labelW {
			return epicLayoutNarrowCount
		}
	}
	return layout
}

// epicBar renders the epic's fixed-width progress bar, filled in the epic's
// state colour (fill).
func epicBar(epic tickets.Epic, fill lipgloss.Style) string {
	fraction := 0.0
	if total := epic.TotalCount(); total > 0 {
		fraction = float64(epic.DoneCount()) / float64(total)
	}
	return ui.ProgressBar(fraction, epicBarCells, fill, lipgloss.NewStyle().Foreground(ui.ColorSurface1))
}

// epicNameLine renders the selectable name row. In the wide layout the
// counter and bar are right-aligned to labelW; otherwise the name is alone,
// cut with an ellipsis if it overflows.
func epicNameLine(epic tickets.Epic, fill lipgloss.Style, layout epicHeaderLayout, labelW int) string {
	name := epic.Name
	if labelW > 0 && layout != epicLayoutWide {
		name = ansi.Truncate(name, labelW-1, "…")
	}
	line := " " + epicHeaderStyle.Render(name)
	if layout != epicLayoutWide || epic.TotalCount() == 0 {
		return line
	}
	tail := epicCounter(epic) + " " + epicBar(epic, fill)
	pad := max(labelW-lipgloss.Width(line)-lipgloss.Width(tail), 1)
	return line + strings.Repeat(" ", pad) + tail
}

// epicBarLine renders the narrow layouts' second line.
func epicBarLine(epic tickets.Epic, fill lipgloss.Style, layout epicHeaderLayout) string {
	if layout == epicLayoutNarrowCount {
		return " " + epicCounter(epic)
	}
	return " " + epicBar(epic, fill) + " " + epicCounter(epic)
}

// epicStatusRow renders "Status: <icon> <state> · n compact(s)", all dim
// except the state, cut at labelW. extra is appended as another dim part.
func (m QueueModel) epicStatusRow(epic tickets.Epic, extra string, labelW int) string {
	icon, text, style := m.epicStatus(epic)
	line := " " + metricsLineStyle.Render("Status:") + " " + style.Render(icon+" "+text)
	parts := []string{}
	if _, _, compacts := epicContextMetrics(epic); compacts > 0 {
		noun := "compacts"
		if compacts == 1 {
			noun = "compact"
		}
		parts = append(parts, fmt.Sprintf("%d %s", compacts, noun))
	}
	if extra != "" {
		parts = append(parts, extra)
	}
	for _, p := range parts {
		line += metricsLineStyle.Render(" · " + p)
	}
	if labelW > 0 {
		line = ansi.Truncate(line, labelW, "…")
	}
	return line
}

// queueRunStateKind classifies the queue's run state for header rendering.
// queueHeaderTitle and queueHeaderBodyLines both switch on the same
// queueRunState() call so they can't independently re-derive (and silently
// diverge on) what state the queue is in.
type queueRunStateKind int

const (
	queueRunIdle queueRunStateKind = iota
	queueRunRunning
)

// queueRunState classifies the queue's current run state from the epics the
// server reports running.
func (m QueueModel) queueRunState() queueRunStateKind {
	if len(m.runningEpics) > 0 {
		return queueRunRunning
	}
	return queueRunIdle
}

// queueHeaderTitle and queueHeaderBodyLines together implement the Option B
// header redesign (see .scratch/tickets-queue-batch3/issues/assets/08-header-prototype.md):
// the title always encodes run state, and the body carries at most one
// state-specific line instead of the old always-present banner row.
func (m QueueModel) queueHeaderTitle() string {
	return m.queueRunStateTitle() + " · " + m.queueHeaderCostSuffix()
}

// queueRunStateTitle is queueHeaderTitle's run-state text, before ticket 10's
// live-cost suffix is appended — split out so the suffix logic doesn't have
// to be repeated in every switch case.
func (m QueueModel) queueRunStateTitle() string {
	if m.queueRunState() != queueRunRunning {
		return "Queue"
	}
	glyph := strings.TrimRight(m.implementSpinner.View(), " ")
	done, total := m.runningProgress()
	if total == 0 {
		return fmt.Sprintf("Queue · %s implementing...", glyph)
	}
	return fmt.Sprintf("Queue · %d of %d done · %s implementing...", done, total, glyph)
}

// runningProgress sums the done/total ticket counts of the epics the server
// reports running.
func (m QueueModel) runningProgress() (done, total int) {
	for _, epic := range m.epics {
		if m.runningEpics[epic.Name] {
			done += epic.DoneCount()
			total += epic.TotalCount()
		}
	}
	return done, total
}

// queueHeaderCostSuffix renders today's server spend appended to the title
// line: "$X of $Y" colored against the server's soft limit when one is set,
// bare unstyled "$X" when the budget is off.
func (m QueueModel) queueHeaderCostSuffix() string {
	total, soft := m.serverBudget.Total, m.serverBudget.SoftLimit
	text := "today " + tickets.FormatCost(total)
	style := lipgloss.NewStyle()
	if soft > 0 {
		text = fmt.Sprintf("today %s of %s", tickets.FormatCost(total), tickets.FormatCost(soft))
		style = budgetTotalStyle(total, soft)
	}
	return style.Render(text)
}

// budgetTotalStyle picks the live-total's color against softLimit: the
// default/no-color treatment below 80% of it, epicStatusProblemStyle's
// warning yellow from 80% up to it, and epicStatusParkedRepairStyle's alarm
// red at or above it — reusing the same colors already used for epic-header
// warning states (ticket 10) rather than duplicating them. Unstyled entirely
// when softLimit is 0 (budget disabled) — callers should check that first,
// but this also degrades to unstyled if called with 0 directly.
func budgetTotalStyle(total, softLimit float64) lipgloss.Style {
	if softLimit <= 0 {
		return lipgloss.NewStyle()
	}
	switch {
	case total >= softLimit:
		return epicStatusParkedRepairStyle
	case total >= 0.8*softLimit:
		return epicStatusProblemStyle
	default:
		return lipgloss.NewStyle()
	}
}

// queueHeaderBodyLines always returns exactly queueHeaderReservedLines
// lines — the reserved banner slot's constant height — so m.queueTree's
// SetVisibleHeight never goes stale when the active run state changes
// without an intervening resize. Most non-idle states leave the slot blank
// (mirroring the pre-ticket-21 nil body); only queueRunIdle fills it with
// contextual copy, since that's the state a person is actually looking at
// the Queue tab wondering what to do next.
func (m QueueModel) queueHeaderBodyLines() []string {
	if m.serverDown() {
		return []string{epicStatusParkedAnswerStyle.Render(queueServerDownBanner)}
	}
	if m.serverAPI != nil && m.herdrDown {
		return []string{epicStatusProblemStyle.Render(queueHerdrDownBanner)}
	}
	if m.queueRunState() == queueRunRunning {
		return []string{""}
	}
	return []string{m.queueIdleBodyLine()}
}

// queueHerdrDownBanner is shown in server mode while the server cannot reach
// herdr: queued tickets stay queued until it answers again.
const queueHerdrDownBanner = "herdr unavailable — the server can't start agents until it answers"

// queueIdleBodyLine picks the reserved slot's idle copy: a pointer back to
// the Tickets tab when nothing is checked, otherwise where runs start from.
func (m QueueModel) queueIdleBodyLine() string {
	if m.serverAPI != nil {
		if len(m.checked) == 0 {
			return epicStatusParkedAnswerStyle.Render("Queue is empty — enqueue tickets from the Tickets tab")
		}
		return epicStatusParkedAnswerStyle.Render("Waiting for the server to claim queued tickets")
	}
	if len(m.checkedEpicPlans()) == 0 {
		return epicStatusParkedAnswerStyle.Render("No selected tickets — go to the Tickets tab first")
	}
	return epicStatusParkedAnswerStyle.Render("Idle — runs start from the server")
}
