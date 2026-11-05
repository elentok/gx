// Package queue holds the Queue tab's header: the title, the run-state banner
// and the per-epic status rows.
package queue

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

// HeaderReservedLines is the constant height reserved for the Queue tab's
// run-state banner slot (Header.BodyLines), whether or not a banner is
// actually active — the maximum line count any RunStateKind can emit.
const HeaderReservedLines = 1

// ServerDownBanner is the banner shown while the server link is down.
const ServerDownBanner = "server not running — s to start"

var (
	// epicHeaderStyle highlights the Queue tab's per-epic header name in the
	// same blue used elsewhere in this UI's palette (statusClaimedStyle),
	// rather than sectionHeaderStyle's neutral divider color.
	epicHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)

	// epicStatusDoneStyle colors an epic header's status line green once
	// every ticket is done.
	epicStatusDoneStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen)

	// EpicStatusProblemStyle is the warning yellow, used for the herdr banner
	// and the budget total nearing its soft limit.
	EpicStatusProblemStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)

	// EpicStatusPausedAnswerStyle colors a paused epic's header status line
	// orange when every ticket a human still needs to act on is waiting on a
	// question (paused (answer)) rather than a fault.
	EpicStatusPausedAnswerStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)

	// EpicStatusPausedRepairStyle colors a paused epic's header status line
	// red once any ticket a human still needs to act on is broken
	// (paused (repair)) — red wins over orange when both kinds are present,
	// since a fault outranks a question.
	EpicStatusPausedRepairStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
)

// epicStateLabels must match LABELS in webui/src/view.ts.
var epicStateLabels = map[wire.EpicState]string{
	wire.EpicStatePaused:        "Paused",
	wire.EpicStateRunning:       "Running",
	wire.EpicStateWaiting:       "Waiting for a slot",
	wire.EpicStateAwaitingMerge: "Awaiting merge",
	wire.EpicStateMerging:       "Merging",
	wire.EpicStateFinished:      "Merged",
}

// epicStatusLine picks an epic header's status-line icon, text, and color from
// the server's summary. The text is the state's label, plus the reason for a
// deadlocked epic.
func epicStatusLine(icons ui.IconSet, sum wire.EpicSummary, spinnerGlyph string) (icon, text string, style lipgloss.Style) {
	switch sum.State {
	case wire.EpicStateAwaitingMerge, wire.EpicStateFinished:
		return icons.TicketDone, epicStateLabels[sum.State], epicStatusDoneStyle
	case wire.EpicStateRunning, wire.EpicStateMerging:
		return spinnerGlyph, epicStateLabels[sum.State], lipgloss.NewStyle()
	case wire.EpicStatePaused:
		text = epicStateLabels[sum.State]
		if sum.Reason != "" {
			text += ": " + sum.Reason
		}
		if sum.More > 0 {
			text += fmt.Sprintf(" +%d more", sum.More)
		}
		switch {
		case sum.MergeBlocked:
			return icons.Warning, text, EpicStatusProblemStyle
		// Red once a paused ticket is broken, since a fault outranks a question.
		case sum.HasRepair || sum.Reason == wire.EpicReasonDeadlock:
			return icons.Warning, text, EpicStatusPausedRepairStyle
		}
		return icons.Warning, text, EpicStatusPausedAnswerStyle
	default:
		return icons.EpicQueued, epicStateLabels[wire.EpicStateWaiting], lipgloss.NewStyle()
	}
}

// Header renders the Queue tab's header and epic status rows. It has no state
// of its own: the tab builds it from its current inputs for each render.
type Header struct {
	Icons        ui.IconSet
	Summaries    map[string]wire.EpicSummary
	RunningEpics map[string]bool
	Live         map[string]map[string]rowfmt.LiveTicketState
	Paused       bool
	// ServerDown, HasServer: the server link is down; a server client exists.
	ServerDown bool
	HasServer  bool
	HerdrDown  bool
	HerdrText  string
	Budget     wire.BudgetStatus
	Finished   []wire.FinishedEpic
	Spinner    spinner.Model
	Now        func() time.Time
	// CheckedCount and CheckedPlans are how many tickets are checked and how
	// many epics they span, for the idle line.
	CheckedCount int
	CheckedPlans int
}

// EpicStatus is epicStatusLine for an epic on the Queue tab, with the current
// spinner frame filled in.
func (h Header) EpicStatus(epic tickets.Epic) (icon, text string, style lipgloss.Style) {
	glyph := strings.TrimRight(h.Spinner.View(), " ")
	return epicStatusLine(h.Icons, h.Summaries[epic.Name], glyph)
}

// epicElapsedSeconds sums the epic's tickets' done ElapsedTime, for the
// header status line's elapsed part once the epic is fully done.
func epicElapsedSeconds(epic tickets.Epic) int {
	total := 0
	for _, t := range epic.Tickets {
		total += t.ElapsedTime
	}
	return total
}

// EpicCost sums the epic's tickets' done ActualCost, for the Status line's
// cost part.
func EpicCost(epic tickets.Epic) float64 {
	total := 0.0
	for _, t := range epic.Tickets {
		total += t.ActualCost
	}
	return total
}

// epicContextMetrics computes the epic's total compaction count.
func epicContextMetrics(epic tickets.Epic) (compacts int) {
	for _, t := range epic.Tickets {
		compacts += t.Compactions
	}
	return compacts
}

// EpicHeaderLayout is how every Queue epic header lays out its progress
// counter and bar. It is one value for all shown epics so the bars line up.
type EpicHeaderLayout int

const (
	EpicLayoutWide        EpicHeaderLayout = iota // name … (n/m) bar on one line
	EpicLayoutNarrowBar                           // name, then " bar (n/m)"
	EpicLayoutNarrowCount                         // name, then " (n/m)"
)

const epicBarCells = 12

func epicCounter(sum wire.EpicSummary) string {
	return fmt.Sprintf("(%d/%d)", sum.Done, sum.Total)
}

// EpicHeaderLayoutFor picks the layout for all of epics: wide only if every
// non-empty epic's name line fits, else narrow with the bar if its own line
// fits, else narrow with only the counter.
func (h Header) EpicHeaderLayoutFor(epics []tickets.Epic, labelW int) EpicHeaderLayout {
	if labelW <= 0 {
		return EpicLayoutWide
	}
	layout := EpicLayoutWide
	for _, epic := range epics {
		sum := h.Summaries[epic.Name]
		if sum.Total == 0 {
			continue
		}
		counterW := lipgloss.Width(epicCounter(sum))
		if 1+lipgloss.Width(epic.Name)+1+counterW+1+epicBarCells > labelW {
			layout = EpicLayoutNarrowBar
		}
		if 1+epicBarCells+1+counterW > labelW {
			return EpicLayoutNarrowCount
		}
	}
	return layout
}

// epicBar renders the epic's fixed-width progress bar, filled in the epic's
// state colour (fill).
func epicBar(sum wire.EpicSummary, fill lipgloss.Style) string {
	fraction := 0.0
	if sum.Total > 0 {
		fraction = float64(sum.Done) / float64(sum.Total)
	}
	return ui.ProgressBar(fraction, epicBarCells, fill, lipgloss.NewStyle().Foreground(ui.ColorSurface1))
}

// EpicNameLine renders the selectable name row. In the wide layout the
// counter and bar are right-aligned to labelW; otherwise the name is alone,
// cut with an ellipsis if it overflows.
func EpicNameLine(name string, sum wire.EpicSummary, fill lipgloss.Style, layout EpicHeaderLayout, labelW int) string {
	if labelW > 0 && layout != EpicLayoutWide {
		name = ansi.Truncate(name, labelW-1, "…")
	}
	line := " " + epicHeaderStyle.Render(name)
	if layout != EpicLayoutWide || sum.Total == 0 {
		return line
	}
	tail := epicCounter(sum) + " " + epicBar(sum, fill)
	pad := max(labelW-lipgloss.Width(line)-lipgloss.Width(tail), 1)
	return line + strings.Repeat(" ", pad) + tail
}

// EpicBarLine renders the narrow layouts' second line.
func EpicBarLine(sum wire.EpicSummary, fill lipgloss.Style, layout EpicHeaderLayout) string {
	if layout == EpicLayoutNarrowCount {
		return " " + epicCounter(sum)
	}
	return " " + epicBar(sum, fill) + " " + epicCounter(sum)
}

// formatSpan renders an elapsed span at minute resolution, "<1m" under a minute.
func formatSpan(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	return rowfmt.FormatDuration(d)
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

// epicAgentPart is the Status line's "<agent> agent" part: done tickets'
// elapsed time plus the live elapsed of running ones (live is the epic's
// ticket identifier -> running state; nil when the epic isn't running). ""
// when zero.
func epicAgentPart(epic tickets.Epic, now time.Time, live map[string]rowfmt.LiveTicketState) string {
	d := time.Duration(epicElapsedSeconds(epic)) * time.Second
	for _, t := range epic.Tickets {
		if l, ok := live[t.Identifier]; ok && l.Running && !l.StartedAt.IsZero() {
			d += now.Sub(l.StartedAt)
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
func epicStatusParts(epic tickets.Epic, now time.Time, live map[string]rowfmt.LiveTicketState) []string {
	var parts []string
	for _, p := range []string{epicElapsedPart(epic, now), epicAgentPart(epic, now, live)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if cost := EpicCost(epic); cost > 0 {
		parts = append(parts, tickets.FormatCost(cost))
	}
	if compacts := epicContextMetrics(epic); compacts > 0 {
		parts = append(parts, pluralize(compacts, "compact"))
	}
	return parts
}

// EpicStatusRow renders "Status: <icon> <state> · elapsed · agent · cost ·
// n compact(s)", all dim except the state, zero parts hidden, cut at labelW.
func (h Header) EpicStatusRow(epic tickets.Epic, labelW int) string {
	icon, text, style := h.EpicStatus(epic)
	line := " " + rowfmt.MetricsLineStyle.Render("Status:") + " " + style.Render(icon+" "+text)
	var live map[string]rowfmt.LiveTicketState
	if h.RunningEpics[epic.Name] {
		live = h.Live[epic.Name]
	}
	if parts := epicStatusParts(epic, h.Now(), live); len(parts) > 0 {
		line += rowfmt.MetricsLineStyle.Render(" · " + strings.Join(parts, " · "))
	}
	if labelW > 0 {
		line = ansi.Truncate(line, labelW, "…")
	}
	return line
}

// RunStateKind classifies the queue's run state for header rendering.
// Title and BodyLines both switch on the same RunState() call so they can't
// independently re-derive (and silently diverge on) what state the queue is in.
type RunStateKind int

const (
	RunIdle RunStateKind = iota
	RunRunning
)

// RunState classifies the queue's current run state from the epics the
// server reports running.
func (h Header) RunState() RunStateKind {
	if len(h.RunningEpics) > 0 {
		return RunRunning
	}
	return RunIdle
}

// Title and BodyLines together implement the Option B
// header redesign (see .scratch/tickets-queue-batch3/issues/assets/08-header-prototype.md):
// the title always encodes run state, and the body carries at most one
// state-specific line instead of the old always-present banner row.
func (h Header) Title() string {
	return h.runStateTitle() + " · " + h.CostSuffix()
}

// runStateTitle is Title's run-state text, before ticket 10's
// live-cost suffix is appended — split out so the suffix logic doesn't have
// to be repeated in every switch case.
func (h Header) runStateTitle() string {
	if h.RunState() != RunRunning {
		return "Queue"
	}
	glyph := strings.TrimRight(h.Spinner.View(), " ")
	done, total := h.runningProgress()
	if total == 0 {
		return fmt.Sprintf("Queue · %s implementing...", glyph)
	}
	return fmt.Sprintf("Queue · %d of %d done · %s implementing...", done, total, glyph)
}

// runningProgress sums the done/total ticket counts of the epics the server
// reports running.
func (h Header) runningProgress() (done, total int) {
	for _, sum := range h.Summaries {
		if sum.State == wire.EpicStateRunning {
			done += sum.Done
			total += sum.Total
		}
	}
	return done, total
}

// CostSuffix renders today's server spend appended to the title
// line: "$X of $Y" colored against the server's soft limit when one is set,
// bare unstyled "$X" when the budget is off.
func (h Header) CostSuffix() string {
	total, soft := h.Budget.Total, h.Budget.SoftLimit
	text := "today " + tickets.FormatCost(total)
	style := lipgloss.NewStyle()
	if soft > 0 {
		text = fmt.Sprintf("today %s of %s", tickets.FormatCost(total), tickets.FormatCost(soft))
		style = BudgetTotalStyle(total, soft)
	}
	return style.Render(text)
}

// BudgetTotalStyle picks the live-total's color against softLimit: the
// default/no-color treatment below 80% of it, EpicStatusProblemStyle's
// warning yellow from 80% up to it, and EpicStatusPausedRepairStyle's alarm
// red at or above it — reusing the same colors already used for epic-header
// warning states (ticket 10) rather than duplicating them. Unstyled entirely
// when softLimit is 0 (budget disabled) — callers should check that first,
// but this also degrades to unstyled if called with 0 directly.
func BudgetTotalStyle(total, softLimit float64) lipgloss.Style {
	if softLimit <= 0 {
		return lipgloss.NewStyle()
	}
	switch {
	case total >= softLimit:
		return EpicStatusPausedRepairStyle
	case total >= 0.8*softLimit:
		return EpicStatusProblemStyle
	default:
		return lipgloss.NewStyle()
	}
}

// BodyLines always returns exactly HeaderReservedLines
// lines — the reserved banner slot's constant height — so the queue tree's
// SetVisibleHeight never goes stale when the active run state changes
// without an intervening resize. Most non-idle states leave the slot blank
// (mirroring the pre-ticket-21 nil body); only RunIdle fills it with
// contextual copy, since that's the state a person is actually looking at
// the Queue tab wondering what to do next.
func (h Header) BodyLines() []string {
	if h.ServerDown {
		return []string{EpicStatusPausedAnswerStyle.Render(ServerDownBanner)}
	}
	if h.HasServer && h.HerdrDown {
		banner := herdrDownBanner
		if h.HerdrText != "" {
			banner = "herdr unavailable — " + h.HerdrText
		}
		return []string{EpicStatusProblemStyle.Render(banner)}
	}
	if h.Paused {
		return []string{EpicStatusProblemStyle.Render(pausedLine(h.runningTicketCount()))}
	}
	if h.RunState() == RunRunning {
		return []string{""}
	}
	return []string{h.idleBodyLine()}
}

// pausedLine is the header text of a queue the person paused.
func pausedLine(running int) string {
	switch running {
	case 0:
		return "Paused. No tickets running."
	case 1:
		return "Paused. 1 ticket still running."
	}
	return fmt.Sprintf("Paused. %d tickets still running.", running)
}

// runningTicketCount is how many tickets the server holds a run for.
func (h Header) runningTicketCount() int {
	n := 0
	for _, sum := range h.Summaries {
		n += len(sum.Active)
	}
	return n
}

// herdrDownBanner is shown in server mode while the server cannot reach
// herdr: queued tickets stay queued until it answers again.
const herdrDownBanner = "herdr unavailable — the server can't start agents until it answers"

// idleBodyLine picks the reserved slot's idle copy: a pointer back to
// the Tickets tab when nothing is checked, otherwise where runs start from.
func (h Header) idleBodyLine() string {
	if h.HasServer {
		if h.CheckedCount == 0 {
			return EpicStatusPausedAnswerStyle.Render("Queue is empty — enqueue tickets from the Tickets tab")
		}
		return EpicStatusPausedAnswerStyle.Render("Waiting for the server to claim queued tickets")
	}
	if h.CheckedPlans == 0 {
		return EpicStatusPausedAnswerStyle.Render("No selected tickets — go to the Tickets tab first")
	}
	return EpicStatusPausedAnswerStyle.Render("Idle — runs start from the server")
}
