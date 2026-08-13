package tickets

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
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
	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 12000\nelapsed_time: 754\ncompactions: 2\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: task\nactual_context_window: 8000\nelapsed_time: 100\ncompactions: 1\n---\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	content := ansi.Strip(m.View().Content)

	if !strings.Contains(content, "took 14m14s") {
		t.Fatalf("expected the epic status line to report total elapsed time:\n%s", content)
	}
	if !strings.Contains(content, "Context window: avg 10.0k tok, max 12.0k tok (3 compacts)") {
		t.Fatalf("expected the epic context-window line:\n%s", content)
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
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

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
	// unselected, "▌" when this row holds the cursor — the ticket row is
	// selected by default now that filler rows are excluded from selection,
	// see ticket 17), which strings.TrimLeft's space-only trim can't skip past
	// on its own.
	rowRunes := []rune(rowLine)
	rowRest := string(rowRunes[1:])
	rowIndent := 1 + len(rowRest) - len(strings.TrimLeft(rowRest, " "))
	triangleColumn := triangleColumnWidth(m.icons()) + 1
	if headerIndent != 2 || rowIndent != headerIndent+triangleColumn {
		t.Fatalf("got headerIndent=%d rowIndent=%d, want headerIndent=2 rowIndent=%d", headerIndent, rowIndent, 2+triangleColumn)
	}
}
