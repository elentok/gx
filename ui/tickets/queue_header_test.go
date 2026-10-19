package tickets

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	icon, text, style := epicStatusLine(icons, done, false, "")
	if !strings.Contains(text, "took 12m34s") {
		t.Fatalf("done epic: got text=%q, want it to contain %q", text, "took 12m34s")
	}
	if style.Render(icon) != epicStatusDoneStyle.Render(icon) {
		t.Fatalf("done epic: status line not rendered in epicStatusDoneStyle (green)")
	}

	problem := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "needs-answer"},
		{Identifier: "03", Status: "open"},
	}}
	icon, _, style = epicStatusLine(icons, problem, false, "")
	if style.Render(icon) != epicStatusProblemStyle.Render(icon) {
		t.Fatalf("problem epic: status line not rendered in epicStatusProblemStyle (yellow)")
	}

	clean := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "open"},
	}}
	icon, _, style = epicStatusLine(icons, clean, false, "")
	if style.Render(icon) != icon {
		t.Fatalf("in-progress-clean epic: expected the default/no-color treatment, got styled output %q", style.Render(icon))
	}
}

func TestEpicStatusLineImplementingAndQueuedStates(t *testing.T) {
	t.Parallel()
	for _, nerd := range []bool{false, true} {
		icons := ui.Icons(nerd)
		epic := tickets.Epic{Tickets: []tickets.Ticket{
			{Identifier: "01", Status: "done"},
			{Identifier: "02", Status: "open"},
		}}

		icon, text, style := epicStatusLine(icons, epic, true, "S")
		if icon != "S" || text != "implementing" || style.Render(text) != text {
			t.Fatalf("nerd=%v running: got icon=%q text=%q styled=%q", nerd, icon, text, style.Render(text))
		}

		icon, text, style = epicStatusLine(icons, epic, false, "S")
		if icon != "○" || text != "queued" || style.Render(text) != text {
			t.Fatalf("nerd=%v queued: got icon=%q text=%q styled=%q", nerd, icon, text, style.Render(text))
		}

		problem := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "needs-repair"}, {Identifier: "02", Status: "open"}}}
		icon, _, style = epicStatusLine(icons, problem, true, "S")
		if icon != icons.Warning || style.Render(icon) != epicStatusProblemStyle.Render(icon) {
			t.Fatalf("nerd=%v running+problem: needs attention must outrank implementing, got icon=%q", nerd, icon)
		}

		allDone := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "done"}}}
		icon, _, _ = epicStatusLine(icons, allDone, true, "S")
		if icon != icons.TicketDone {
			t.Fatalf("nerd=%v running+done: done must outrank implementing, got icon=%q", nerd, icon)
		}
	}
}

func TestEpicBarFillFollowsStateColour(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "open"},
	}}
	if got, want := epicBar(epic, epicStatusProblemStyle), epicBar(epic, lipgloss.NewStyle()); got == want {
		t.Fatalf("bar fill ignored the state colour: %q", got)
	}
	if !strings.Contains(epicBar(epic, epicStatusProblemStyle), epicStatusProblemStyle.Render(strings.Repeat("▰", epicBarCells/2))) {
		t.Fatalf("bar fill not rendered in the state colour: %q", epicBar(epic, epicStatusProblemStyle))
	}
}

func TestQueueSpinnerFollowsNerdFontSetting(t *testing.T) {
	t.Parallel()
	plain := NewQueueModel("", ui.Settings{UseNerdFontIcons: false}, keys.Manager{})
	nerd := NewQueueModel("", ui.Settings{UseNerdFontIcons: true}, keys.Manager{})
	if plain.implementSpinner.Spinner.Frames[0] != ui.ASCIIProgressSpinner.Frames[0] {
		t.Fatalf("plain spinner frame = %q, want ASCII frames", plain.implementSpinner.Spinner.Frames[0])
	}
	if nerd.implementSpinner.Spinner.Frames[0] != ui.ProgressSpinner.Frames[0] {
		t.Fatalf("nerd spinner frame = %q, want Nerd frames", nerd.implementSpinner.Spinner.Frames[0])
	}
}

func parkedEpic(statuses ...string) tickets.Epic {
	var ts []tickets.Ticket
	for i, st := range statuses {
		ts = append(ts, tickets.Ticket{Identifier: fmt.Sprintf("%02d", i+1), Status: st})
	}
	return tickets.Epic{Tickets: ts}
}

// Parked is derived from the epic's own tickets (mirroring server/deadlock.go):
// nothing open/claimed and something needs a person.
func TestEpicStatusLineParkedDerivedFromTickets(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)

	icon, text, style := epicStatusLine(icons, parkedEpic("done", "needs-answer", "draft"), false, "")
	if icon != icons.Warning {
		t.Fatalf("icon = %q, want icons.Warning", icon)
	}
	if text != "parked — waiting on 02" {
		t.Fatalf("text = %q, drafts must not be listed", text)
	}
	if style.Render(icon) != epicStatusParkedAnswerStyle.Render(icon) {
		t.Fatalf("needs-answer park: want orange")
	}

	if _, text, _ = epicStatusLine(icons, parkedEpic("needs-answer", "open"), false, ""); strings.Contains(text, "parked") {
		t.Fatalf("an open ticket means not parked: text = %q", text)
	}
	if _, text, _ = epicStatusLine(icons, parkedEpic("needs-answer", "claimed"), false, ""); strings.Contains(text, "parked") {
		t.Fatalf("a claimed ticket means not parked: text = %q", text)
	}
	if _, text, _ = epicStatusLine(icons, parkedEpic("done", "cancelled"), false, ""); strings.Contains(text, "parked") {
		t.Fatalf("done/cancelled alone is not parked: text = %q", text)
	}
}

func TestEpicStatusLineParkedColorByKind(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	for _, st := range []string{"needs-repair", "error"} {
		icon, text, style := epicStatusLine(icons, parkedEpic("needs-answer", st), false, "")
		if style.Render(icon) != epicStatusParkedRepairStyle.Render(icon) {
			t.Fatalf("%s park: want red", st)
		}
		if text != "parked — waiting on 01, 02" {
			t.Fatalf("%s park: text = %q", st, text)
		}
	}
}

func TestEpicStatusLineParkedOnlyDrafts(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	icon, text, style := epicStatusLine(icons, parkedEpic("done", "draft"), false, "")
	if text != "parked — only drafts left" {
		t.Fatalf("text = %q", text)
	}
	if style.Render(icon) != epicStatusParkedAnswerStyle.Render(icon) {
		t.Fatalf("only-drafts park: want orange")
	}
}

func TestEpicStatusLineParkedCapsWaitingList(t *testing.T) {
	t.Parallel()
	_, text, _ := epicStatusLine(ui.Icons(false), parkedEpic(
		"needs-answer", "needs-answer", "needs-answer", "needs-answer", "needs-answer",
	), false, "")
	if text != "parked — waiting on 01, 02, 03 +2 more" {
		t.Fatalf("text = %q", text)
	}
	if strings.Contains(text, "reattachable") {
		t.Fatalf("no reattachable suffix expected: %q", text)
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
	_, text, _ := epicStatusLine(icons, withTimestamps, false, "")
	if !strings.Contains(text, "took 3h 30m") {
		t.Fatalf("epic with completion timestamps: got text=%q, want it to contain %q", text, "took 3h 30m")
	}

	withoutTimestamps := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 754}}}
	_, text, _ = epicStatusLine(icons, withoutTimestamps, false, "")
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

func statusRowAt(now time.Time, m QueueModel, epic tickets.Epic) string {
	m.now = func() time.Time { return now }
	return ansi.Strip(m.epicStatusRow(epic, 0))
}

func TestEpicStatusRowElapsedAndAgentParts(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.Manager{})

	cases := []struct {
		name string
		now  time.Duration
		epic tickets.Epic
		want []string
		not  []string
	}{
		{"never started shows only the state", time.Hour,
			tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
			[]string{"queued"}, []string{"elapsed", "agent", "·"}},
		{"started but idle uses the model clock, under a minute", 30 * time.Second,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
			[]string{"queued · <1m elapsed"}, []string{"agent"}},
		{"minute resolution", 134 * time.Minute,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
			[]string{"2h 14m elapsed"}, nil},
		{"agent time sums landed tickets", 45 * time.Minute,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{
				{Identifier: "01", Status: "done", ElapsedTime: 600}, {Identifier: "02", Status: "open"}}},
			[]string{"45m elapsed · 10m agent"}, []string{"· $"}},
		{"done uses start to completion", 10 * time.Hour,
			tickets.Epic{StartedAt: started, CompletedAt: started.Add(90 * time.Minute),
				Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 600}}},
			[]string{"1h 30m elapsed · 10m agent"}, nil},
		{"done without completed_at falls back to the ticket sum", 10 * time.Hour,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 600}}},
			[]string{"10m elapsed · 10m agent"}, nil},
		{"done without started_at hides elapsed", 10 * time.Hour,
			tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 600}}},
			[]string{"10m agent"}, []string{" elapsed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusRowAt(started.Add(tc.now), m, tc.epic)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("%q missing %q", got, w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(got, n) {
					t.Errorf("%q should not contain %q", got, n)
				}
			}
			if strings.HasSuffix(strings.TrimSpace(got), "·") {
				t.Errorf("trailing separator: %q", got)
			}
		})
	}
}

func TestEpicStatusRowAgentTimeIncludesLiveRunningTicket(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.Manager{})
	m.runningEpics = map[string]bool{"alpha": true}
	m.live = map[string]map[string]liveTicketState{"alpha": {"02": {running: true, startedAt: started.Add(30 * time.Minute)}}}
	epic := tickets.Epic{Name: "alpha", StartedAt: started, Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done", ElapsedTime: 600}, {Identifier: "02", Status: "claimed"}}}

	got := statusRowAt(started.Add(45*time.Minute), m, epic)
	if !strings.Contains(got, "45m elapsed · 25m agent") {
		t.Errorf("want 10m landed + 15m live = 25m agent: %q", got)
	}
}

func TestEpicStatusRowCutAtPaneEdge(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.Manager{})
	m.now = func() time.Time { return started.Add(time.Hour) }
	epic := tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}}

	if got := ansi.Strip(m.epicStatusRow(epic, 20)); lipgloss.Width(got) > 20 {
		t.Errorf("width %d > 20: %q", lipgloss.Width(got), got)
	}
}
