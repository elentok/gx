package tickets

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestEpicStatusLineColorsByEpicState(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)

	done := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done", ElapsedTime: 754},
	}}
	icon, text, style := epicStatusLine(icons, done, nil)
	if !strings.Contains(text, "took 12m34s") {
		t.Fatalf("done epic: got text=%q, want it to contain %q", text, "took 12m34s")
	}
	if style.Render(icon) != epicStatusDoneStyle.Render(icon) {
		t.Fatalf("done epic: status line not rendered in epicStatusDoneStyle (green)")
	}

	problem := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "needs-answer"},
	}}
	icon, _, style = epicStatusLine(icons, problem, nil)
	if style.Render(icon) != epicStatusProblemStyle.Render(icon) {
		t.Fatalf("problem epic: status line not rendered in epicStatusProblemStyle (yellow)")
	}

	clean := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "open"},
	}}
	icon, _, style = epicStatusLine(icons, clean, nil)
	if style.Render(icon) != icon {
		t.Fatalf("in-progress-clean epic: expected the default/no-color treatment, got styled output %q", style.Render(icon))
	}
}

// TestEpicStatusLineParkedRendersStallReasonAndReattachability covers 11a5's
// direct-coverage AC for parked header rendering: the status line renders
// distinctly from running/queued (icons.Warning, epicStatusParkedStyle) and
// names each stalled ticket, appending "(reattachable)" only for the ones
// whose StalledTicket.Reattachable is true — driven off directly constructed
// StalledTicket values, not ticket disk status, since both stalled tickets
// here share the same underlying ticket status.
func TestEpicStatusLineParkedRendersStallReasonAndReattachability(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "claimed"}}}

	stalled := []ralphloop.StalledTicket{
		{Identifier: "01", Reattachable: true},
		{Identifier: "02", Reattachable: false},
	}
	icon, text, style := epicStatusLine(icons, epic, stalled)
	if icon != icons.Warning {
		t.Fatalf("parked epic: icon = %q, want icons.Warning", icon)
	}
	if style.Render(icon) != epicStatusParkedAnswerStyle.Render(icon) {
		t.Fatalf("parked epic: status line not rendered in epicStatusParkedAnswerStyle")
	}
	if !strings.Contains(text, "2 parked") {
		t.Fatalf("parked epic: text = %q, want it to lead with the parked count", text)
	}
	if !strings.Contains(text, "01 (reattachable)") {
		t.Fatalf("parked epic: text = %q, want ticket 01 marked reattachable", text)
	}
	if strings.Contains(text, "02 (reattachable)") {
		t.Fatalf("parked epic: text = %q, ticket 02 must not be marked reattachable", text)
	}
	if !strings.Contains(text, "02") {
		t.Fatalf("parked epic: text = %q, want it to still list ticket 02", text)
	}
}

// TestEpicStatusLineParkedColorByKind covers ticket 21's colour rule: an
// epic with only needs-answer parks renders orange, one containing a
// needs-repair park renders red (red wins even alongside a needs-answer
// ticket).
func TestEpicStatusLineParkedColorByKind(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)

	answerOnly := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "needs-answer"},
	}}
	stalled := []ralphloop.StalledTicket{{Identifier: "01"}}
	icon, _, style := epicStatusLine(icons, answerOnly, stalled)
	if style.Render(icon) != epicStatusParkedAnswerStyle.Render(icon) {
		t.Fatalf("needs-answer-only park: expected epicStatusParkedAnswerStyle (orange)")
	}

	mixed := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "needs-answer"},
		{Identifier: "02", Status: "needs-repair"},
	}}
	stalled = []ralphloop.StalledTicket{{Identifier: "01"}, {Identifier: "02"}}
	icon, _, style = epicStatusLine(icons, mixed, stalled)
	if style.Render(icon) != epicStatusParkedRepairStyle.Render(icon) {
		t.Fatalf("park containing needs-repair: expected epicStatusParkedRepairStyle (red)")
	}
}

// TestEpicStatusLineParkedExcludesDraftFromCount covers ticket 21's draft
// exclusion: a draft ticket is parked for scheduling purposes only, so it
// doesn't move the count or the colour.
func TestEpicStatusLineParkedExcludesDraftFromCount(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)

	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "needs-answer"},
		{Identifier: "02", Status: "draft"},
	}}
	stalled := []ralphloop.StalledTicket{{Identifier: "01"}, {Identifier: "02"}}
	_, text, _ := epicStatusLine(icons, epic, stalled)
	if !strings.Contains(text, "1 parked") {
		t.Fatalf("expected draft ticket excluded from count: text = %q, want it to say 1 parked", text)
	}
	if strings.Contains(text, "02") {
		t.Fatalf("expected draft ticket excluded from the waiting list: text = %q", text)
	}

	allDraft := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "draft"},
	}}
	stalled = []ralphloop.StalledTicket{{Identifier: "01"}}
	icon, _, style := epicStatusLine(icons, allDraft, stalled)
	if style.Render(icon) == epicStatusParkedAnswerStyle.Render(icon) || style.Render(icon) == epicStatusParkedRepairStyle.Render(icon) {
		t.Fatalf("expected an all-draft park not to render as parked at all")
	}
}

func TestEpicStatusLinePrefersCompletionTimestampsOverElapsedSum(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	completed := started.Add(3*time.Hour + 30*time.Minute)

	withTimestamps := tickets.Epic{
		Tickets:     []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 754}},
		StartedAt:   started,
		CompletedAt: completed,
	}
	_, text, _ := epicStatusLine(icons, withTimestamps, nil)
	if !strings.Contains(text, "took 3h 30m") {
		t.Fatalf("epic with completion timestamps: got text=%q, want it to contain %q", text, "took 3h 30m")
	}

	withoutTimestamps := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 754}}}
	_, text, _ = epicStatusLine(icons, withoutTimestamps, nil)
	if !strings.Contains(text, "took 12m34s") {
		t.Fatalf("epic without completion timestamps: got text=%q, want it to contain %q", text, "took 12m34s")
	}
}

func TestEpicContextMetricsAveragesMaxAndSumsCompactions(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done", ActualContextWindow: 12000, Compactions: 2},
		{Identifier: "02", Status: "done", ActualContextWindow: 8000, Compactions: 1},
		{Identifier: "03", Status: "open"}, // never run: excluded from avg/max, contributes 0 compactions
	}}

	avg, maximum, compacts := epicContextMetrics(epic)
	if avg != 10000 || maximum != 12000 || compacts != 3 {
		t.Fatalf("got avg=%d max=%d compacts=%d, want avg=10000 max=12000 compacts=3", avg, maximum, compacts)
	}
}

func TestQueueModelEpicHeaderRendersStatusAndContextLines(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\nactual_context_window: 12000\nelapsed_time: 754\ncompactions: 2\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: implement\nactual_context_window: 8000\nelapsed_time: 100\ncompactions: 1\n---\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := ansi.Strip(m.View().Content)

	if !strings.Contains(content, "took 14m14s") {
		t.Fatalf("expected the epic status line to report total elapsed time:\n%s", content)
	}
	if !strings.Contains(content, "Status: ") || !strings.Contains(content, " · 3 compacts") {
		t.Fatalf("expected the epic Status line with its compacts:\n%s", content)
	}
	if strings.Contains(content, "Context window") {
		t.Fatalf("expected the context-window row to be gone:\n%s", content)
	}
}

// TestQueueModelListRowsIndentMatchesHeaderIndent covers ticket 03's header
// indent and ticket 10's fixed-width triangle-column reservation: a
// childless row's pre-icon indent is the 2-char header indent plus the
// reserved triangle-glyph slot (blank, since this leaf ticket shows no
// triangle), not the header's bare 2-char indent.
func TestQueueModelListRowsIndentMatchesHeaderIndent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := selectFirstQueueTicketRow(t, loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked)))

	var headerLine, rowLine string
	for _, line := range m.queueBody(80) {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "alpha") && headerLine == "" {
			headerLine = plain
		}
		if strings.Contains(plain, "First") {
			rowLine = plain
		}
	}
	if headerLine == "" || rowLine == "" {
		t.Fatalf("expected both an epic header line and a ticket row line:\n%v", m.queueBody(80))
	}
	headerIndent := len(headerLine) - len(strings.TrimLeft(headerLine, " "))
	// rowLine's first rune is the selection mark column (a space when
	// unselected, "▌" when this row holds the cursor, as it does here), which strings.TrimLeft's space-only trim can't skip past
	// on its own.
	rowRunes := []rune(rowLine)
	rowRest := string(rowRunes[1:])
	rowIndent := 1 + len(rowRest) - len(strings.TrimLeft(rowRest, " "))
	triangleColumn := triangleColumnWidth(m.icons()) + 1
	if headerIndent != 2 || rowIndent != headerIndent+triangleColumn {
		t.Fatalf("got headerIndent=%d rowIndent=%d, want headerIndent=2 rowIndent=%d", headerIndent, rowIndent, 2+triangleColumn)
	}
}

// TestQueueHeaderCostSuffixFormats covers the bare-"$X"-vs-"$X of $Y" split on
// whether a soft limit is configured; colour bands live in
// queue_server_budget_test.go.
func TestQueueHeaderCostSuffixFormats(t *testing.T) {
	root := t.TempDir()
	budgetOff := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), map[string]bool{}))
	budgetOff.serverBudget.Total = 42.3
	if got, want := budgetOff.queueHeaderCostSuffix(), "today $42.30"; got != want {
		t.Fatalf("budget disabled: queueHeaderCostSuffix() = %q, want bare %q", got, want)
	}

	withBudget := budgetOff
	withBudget.serverBudget = server.BudgetStatus{Total: 50, SoftLimit: 100}
	if got, want := withBudget.queueHeaderCostSuffix(), "today $50.00 of $100.00"; got != want {
		t.Fatalf("soft limit set: queueHeaderCostSuffix() = %q, want %q", got, want)
	}
}

// TestQueueEpicHeaderOmitsCostForNonRunningEpic covers the "no cost data"
// case: an incomplete local-mode epic gets nothing appended.
func TestQueueEpicHeaderOmitsCostForNonRunningEpic(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	headerLine := epicHeaderLine(t, m, "alpha")
	if strings.Contains(headerLine, "$") {
		t.Fatalf("expected no per-epic cost text for an incomplete local-mode epic: %q", headerLine)
	}
}

// TestQueueEpicHeaderAppendsSummedCostForCompletedEpic covers the completed-
// epic cost summary: an AllDone epic with no live cost data gets its final
// ActualCost total appended alongside "took Xm", summed across its own
// tickets.
func TestQueueEpicHeaderAppendsSummedCostForCompletedEpic(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\nelapsed_time: 60\nactual_cost: 30\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: implement\nelapsed_time: 60\nactual_cost: 13\n---\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	statusLine := epicStatusLineText(t, m)
	if !strings.Contains(statusLine, "$43.00") {
		t.Fatalf("epic Status line missing summed cost \"$43.00\" for a completed epic: %q", statusLine)
	}
}

// epicStatusLineText returns m.queueBody's plain-text "Status:" line.
func epicStatusLineText(t *testing.T, m QueueModel) string {
	t.Helper()
	for _, line := range m.queueBody(80) {
		if plain := ansi.Strip(line); strings.Contains(plain, "Status:") {
			return plain
		}
	}
	t.Fatalf("no epic Status line found")
	return ""
}

// epicHeaderLine returns m.queueBody's plain-text epic header line naming
// epicName (the line rendered by queue_view.go's nodeEpicName case), for
// tests that need to inspect just that line rather than the whole panel
// (which also carries queueHeaderTitle's own live-total suffix).
func epicHeaderLine(t *testing.T, m QueueModel, epicName string) string {
	t.Helper()
	for _, line := range m.queueBody(80) {
		plain := ansi.Strip(line)
		if strings.Contains(plain, epicName) {
			return plain
		}
	}
	t.Fatalf("no epic header line found naming %q", epicName)
	return ""
}

// layoutBody loads one done and one open ticket per epic name at the given
// window width and returns the plain Queue body lines.
func layoutBody(t *testing.T, width int, epicNames ...string) (QueueModel, []string) {
	t.Helper()
	root := t.TempDir()
	checked := map[string]bool{}
	for _, name := range epicNames {
		writeTicket(t, root, name, "01-first.md", "Status: open\n\nBody.\n")
		writeTicket(t, root, name, "02-second.md", "Status: open\n\nBody.\n")
		writeRawQueueTicket(t, root, name, "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\ncompactions: 1\n---\n\nBody.\n")
		checked[ticketPath(root, name, "01-first.md")] = true
		checked[ticketPath(root, name, "02-second.md")] = true
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = updated.(QueueModel)
	sidebarW, _ := m.panelWidths(m.width)
	var lines []string
	for _, line := range m.queueBody(sidebarW - 2) {
		lines = append(lines, strings.TrimRight(ansi.Strip(line), " "))
	}
	return m, lines
}

func TestQueueEpicHeaderWideLayoutEndsWithCounterAndBar(t *testing.T) {
	t.Parallel()
	m, lines := layoutBody(t, 140, "alpha")
	sidebarW, _ := m.panelWidths(m.width)
	var name, status string
	for i, l := range lines {
		if strings.Contains(l, "alpha") && name == "" {
			name = l
			status, _ = lines[i+1], 0
		}
	}
	if !strings.HasSuffix(name, "(1/2) ▰▰▰▰▰▰▱▱▱▱▱▱") {
		t.Fatalf("wide name line should end with counter and half-full bar: %q", name)
	}
	if got, want := lipgloss.Width(name), sidebarW-2-3+1; got != want {
		t.Fatalf("name line width (with selection mark) = %d, want %d", got, want)
	}
	if !strings.HasPrefix(strings.TrimSpace(status), "Status: ") || !strings.Contains(status, "1 compact") || strings.Contains(status, "1 compacts") {
		t.Fatalf("expected Status line with singular compact: %q", status)
	}
}

func TestQueueEpicHeaderNarrowSwitchesEveryEpic(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 60, "alpha", "a-very-long-epic-name-that-cannot-fit")
	var bars []string
	for _, l := range lines {
		if strings.Contains(l, "▰") || strings.Contains(l, "▱") {
			bars = append(bars, l)
		}
	}
	if len(bars) != 2 {
		t.Fatalf("expected a bar line per epic, got %q", bars)
	}
	for _, b := range bars {
		if !strings.HasPrefix(strings.TrimLeft(b, "▌ "), "▰▰▰▰▰▰▱▱▱▱▱▱ (1/2)") || strings.Index(b, "▰") != strings.Index(bars[0], "▰") {
			t.Fatalf("bar lines should share one column: %q", bars)
		}
	}
}

func TestQueueEpicHeaderVeryNarrowKeepsOnlyCounter(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 20, "alpha")
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "▰") || strings.Contains(joined, "▱") {
		t.Fatalf("very narrow layout should drop the bar:\n%s", joined)
	}
	if !strings.Contains(joined, " (1/2)") {
		t.Fatalf("very narrow layout should keep the counter:\n%s", joined)
	}
}

func TestQueueEpicHeaderNarrowCutsLongNameWithEllipsis(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 40, "an-extremely-long-epic-name-that-overflows-the-pane")
	found := false
	for _, l := range lines {
		if strings.Contains(l, "an-extremely") && strings.HasSuffix(l, "…") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the long name cut with an ellipsis:\n%s", strings.Join(lines, "\n"))
	}
}

func TestQueueEpicHeaderHasSpacerAfterStatusLine(t *testing.T) {
	t.Parallel()
	_, lines := layoutBody(t, 140, "alpha")
	for i, l := range lines {
		if strings.Contains(l, "Status:") {
			if strings.TrimSpace(lines[i+1]) != "" || !strings.Contains(lines[i+2], "First") {
				t.Fatalf("expected a blank spacer then the first ticket after the Status line: %q", lines[i:i+3])
			}
			return
		}
	}
	t.Fatalf("no Status line found")
}
