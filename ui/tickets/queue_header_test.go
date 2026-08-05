package tickets

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

func TestEpicStatusLineColorsByEpicState(t *testing.T) {
	icons := ui.Icons(false)

	done := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done", ElapsedTime: 754},
	}}
	icon, text, style := epicStatusLine(icons, done)
	if !strings.Contains(text, "took 12m34s") {
		t.Fatalf("done epic: got text=%q, want it to contain %q", text, "took 12m34s")
	}
	if style.Render(icon) != epicStatusDoneStyle.Render(icon) {
		t.Fatalf("done epic: status line not rendered in epicStatusDoneStyle (green)")
	}

	problem := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "needs-info"},
	}}
	icon, _, style = epicStatusLine(icons, problem)
	if style.Render(icon) != epicStatusProblemStyle.Render(icon) {
		t.Fatalf("problem epic: status line not rendered in epicStatusProblemStyle (yellow)")
	}

	clean := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done"},
		{Identifier: "02", Status: "open"},
	}}
	icon, _, style = epicStatusLine(icons, clean)
	if style.Render(icon) != icon {
		t.Fatalf("in-progress-clean epic: expected the default/no-color treatment, got styled output %q", style.Render(icon))
	}
}

func TestEpicStatusLinePrefersCompletionTimestampsOverElapsedSum(t *testing.T) {
	icons := ui.Icons(false)
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	completed := started.Add(3*time.Hour + 30*time.Minute)

	withTimestamps := tickets.Epic{
		Tickets:     []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 754}},
		StartedAt:   started,
		CompletedAt: completed,
	}
	_, text, _ := epicStatusLine(icons, withTimestamps)
	if !strings.Contains(text, "took 3h 30m") {
		t.Fatalf("epic with completion timestamps: got text=%q, want it to contain %q", text, "took 3h 30m")
	}

	withoutTimestamps := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 754}}}
	_, text, _ = epicStatusLine(icons, withoutTimestamps)
	if !strings.Contains(text, "took 12m34s") {
		t.Fatalf("epic without completion timestamps: got text=%q, want it to contain %q", text, "took 12m34s")
	}
}

func TestEpicContextMetricsAveragesMaxAndSumsCompactions(t *testing.T) {
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
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 12000\nelapsed_time: 754\ncompactions: 2\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: task\nactual_context_window: 8000\nelapsed_time: 100\ncompactions: 1\n---\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	content := ansi.Strip(m.View().Content)

	if !strings.Contains(content, "took 14m14s") {
		t.Fatalf("expected the epic status line to report total elapsed time:\n%s", content)
	}
	if !strings.Contains(content, "Context window: avg 10.0k tok, max 12.0k tok (3 compacts)") {
		t.Fatalf("expected the epic context-window line:\n%s", content)
	}
}

func TestQueueModelListRowsIndentMatchesHeaderIndent(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	var headerLine, rowLine string
	for _, line := range m.queueLines() {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "alpha") && headerLine == "" {
			headerLine = plain
		}
		if strings.Contains(plain, "First") {
			rowLine = plain
		}
	}
	if headerLine == "" || rowLine == "" {
		t.Fatalf("expected both an epic header line and a ticket row line:\n%v", m.queueLines())
	}
	headerIndent := len(headerLine) - len(strings.TrimLeft(headerLine, " "))
	rowIndent := len(rowLine) - len(strings.TrimLeft(rowLine, " "))
	if headerIndent != 2 || rowIndent != headerIndent {
		t.Fatalf("got headerIndent=%d rowIndent=%d, want both 2", headerIndent, rowIndent)
	}
}
