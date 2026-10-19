package tickets

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tree"
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

// parkedWaitingMax caps how many waiting ids the parked status text names
// before collapsing the rest into "+N more".
const parkedWaitingMax = 3

// epicState is an epic header's status, from which its icon, text and style
// are all derived.
type epicState int

const (
	epicStateQueued epicState = iota
	epicStateImplementing
	epicStateProblem
	epicStateParkedAnswer
	epicStateParkedRepair
	epicStateDone
)

// style is the colour the state renders in (also the progress bar's fill).
func (s epicState) style() lipgloss.Style {
	switch s {
	case epicStateDone:
		return epicStatusDoneStyle
	case epicStateProblem:
		return epicStatusProblemStyle
	case epicStateParkedAnswer:
		return epicStatusParkedAnswerStyle
	case epicStateParkedRepair:
		return epicStatusParkedRepairStyle
	default:
		return lipgloss.NewStyle()
	}
}

// epicStateOf classifies an epic. Precedence: done > parked > needs attention
// > implementing > queued. running is whether the server reports the epic
// running. parkedText is only set for the parked states.
func epicStateOf(epic tickets.Epic, running bool) (state epicState, parkedText string) {
	if epic.AllDone() {
		return epicStateDone, ""
	}
	if text, parkedState, ok := parkedStatus(epic); ok {
		return parkedState, text
	}
	switch {
	case epicHasProblem(epic):
		return epicStateProblem, ""
	case running:
		return epicStateImplementing, ""
	default:
		return epicStateQueued, ""
	}
}

// epicStatusLine picks an epic header's status-line icon, text, and color from
// its state. spinnerGlyph is the icon of the implementing state.
func epicStatusLine(icons ui.IconSet, epic tickets.Epic, running bool, spinnerGlyph string) (icon, text string, style lipgloss.Style) {
	state, parkedText := epicStateOf(epic, running)
	style = state.style()
	switch state {
	case epicStateDone:
		return icons.TicketDone, "done", style
	case epicStateParkedAnswer, epicStateParkedRepair:
		return icons.Warning, parkedText, style
	case epicStateProblem:
		return icons.Warning, "needs attention", style
	case epicStateImplementing:
		return spinnerGlyph, "implementing", style
	default:
		return icons.EpicQueued, "queued", style
	}
}

// epicFill is the state colour of an epic on the Queue tab, for its bar fill.
func (m QueueModel) epicFill(epic tickets.Epic) lipgloss.Style {
	state, _ := epicStateOf(epic, m.runningEpics[epic.Name])
	return state.style()
}

// epicStatus is epicStatusLine for an epic on the Queue tab, with the model's
// running state and current spinner frame filled in.
func (m QueueModel) epicStatus(epic tickets.Epic) (icon, text string, style lipgloss.Style) {
	glyph := strings.TrimRight(m.implementSpinner.View(), " ")
	return epicStatusLine(m.icons(), epic, m.runningEpics[epic.Name], glyph)
}

// parkedStatus derives the parked state client-side from the same rule as the
// server's all-parked deadlock check (tickets.Epic.IsParked): nothing renders
// open or claimed, and some ticket is parked. Drafts keep the epic parked but
// are never listed as waiting; repair (red) when any listed ticket is
// needs-repair or error, answer (orange) otherwise.
func parkedStatus(epic tickets.Epic) (text string, state epicState, ok bool) {
	var waiting []string
	anyFault, anyDraft := false, false
	for _, t := range epic.Tickets {
		status := epic.RenderedStatus(t)
		switch {
		case status == tickets.StatusOpen || status == tickets.StatusClaimed:
			return "", 0, false
		case !epic.IsParked(t):
		case status == tickets.StatusDraft:
			anyDraft = true
		default:
			waiting = append(waiting, t.Identifier)
			anyFault = anyFault || status == tickets.StatusNeedsRepair || status == tickets.StatusError
		}
	}
	switch {
	case len(waiting) == 0 && !anyDraft:
		return "", 0, false
	case len(waiting) == 0:
		return "parked — only drafts left", epicStateParkedAnswer, true
	}
	text = "parked — waiting on " + strings.Join(waiting[:min(len(waiting), parkedWaitingMax)], ", ")
	if extra := len(waiting) - parkedWaitingMax; extra > 0 {
		text += fmt.Sprintf(" +%d more", extra)
	}
	if anyFault {
		return text, epicStateParkedRepair, true
	}
	return text, epicStateParkedAnswer, true
}

// epicElapsedSeconds sums the epic's tickets' landed ElapsedTime, for the
// header status line's elapsed part once the epic is fully done.
func epicElapsedSeconds(epic tickets.Epic) int {
	total := 0
	for _, t := range epic.Tickets {
		total += t.ElapsedTime
	}
	return total
}

// epicCost sums the epic's tickets' landed ActualCost, for the Status line's
// cost part.
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

// epicContextMetrics computes the epic's total compaction count.
func epicContextMetrics(epic tickets.Epic) (compacts int) {
	for _, t := range epic.Tickets {
		compacts += t.Compactions
	}
	return compacts
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

// queueBodyWidth is the width of the Queue panel's body: the sidebar less its
// two border columns.
func (m QueueModel) queueBodyWidth() int {
	sidebarW, _ := m.panelWidths(m.width)
	return sidebarW - 2
}

// queueLabelWidth is the width a Queue row's label can use inside the body.
// Zero or negative widths (before the first resize) mean "unconstrained".
func (m QueueModel) queueLabelWidth() int {
	if m.width <= 0 {
		return 0
	}
	return tree.LabelWidth(m.queueBodyWidth())
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

// formatSpan renders an elapsed span at minute resolution, "<1m" under a minute.
func formatSpan(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	return formatDuration(d)
}

// epicElapsedPart is the Status line's "<elapsed> elapsed" part, or "" when
// the epic has no started_at. Done epics show start-to-completion, falling
// back to the tickets' summed elapsed time when completed_at is missing.
func epicElapsedPart(epic tickets.Epic, now time.Time) string {
	if epic.StartedAt.IsZero() {
		return ""
	}
	var d time.Duration
	switch dur, ok := epic.CompletionDuration(); {
	case !epic.AllDone():
		d = now.Sub(epic.StartedAt)
	case ok:
		d = dur
	default:
		d = time.Duration(epicElapsedSeconds(epic)) * time.Second
	}
	return formatSpan(d) + " elapsed"
}

// epicAgentPart is the Status line's "<agent> agent" part: landed tickets'
// elapsed time plus the live elapsed of running ones (live is the epic's
// ticket identifier -> running state; nil when the epic isn't running). ""
// when zero.
func epicAgentPart(epic tickets.Epic, now time.Time, live map[string]liveTicketState) string {
	d := time.Duration(epicElapsedSeconds(epic)) * time.Second
	for _, t := range epic.Tickets {
		if l, ok := live[t.Identifier]; ok && l.running && !l.startedAt.IsZero() {
			d += now.Sub(l.startedAt)
		}
	}
	if d <= 0 {
		return ""
	}
	return formatSpan(d) + " agent"
}

// pluralize is "<n> <noun>", with noun pluralised by an "s" unless n is 1.
func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// epicStatusParts are the Status line's dim metrics after the state, in order:
// elapsed, agent, cost, compactions; zero parts are left out.
func epicStatusParts(epic tickets.Epic, now time.Time, live map[string]liveTicketState) []string {
	var parts []string
	for _, p := range []string{epicElapsedPart(epic, now), epicAgentPart(epic, now, live)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if cost := epicCost(epic); cost > 0 {
		parts = append(parts, tickets.FormatCost(cost))
	}
	if compacts := epicContextMetrics(epic); compacts > 0 {
		parts = append(parts, pluralize(compacts, "compact"))
	}
	return parts
}

// epicStatusRow renders "Status: <icon> <state> · elapsed · agent · cost ·
// n compact(s)", all dim except the state, zero parts hidden, cut at labelW.
func (m QueueModel) epicStatusRow(epic tickets.Epic, labelW int) string {
	icon, text, style := m.epicStatus(epic)
	line := " " + metricsLineStyle.Render("Status:") + " " + style.Render(icon+" "+text)
	var live map[string]liveTicketState
	if m.runningEpics[epic.Name] {
		live = m.live[epic.Name]
	}
	if parts := epicStatusParts(epic, m.now(), live); len(parts) > 0 {
		line += metricsLineStyle.Render(" · " + strings.Join(parts, " · "))
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
