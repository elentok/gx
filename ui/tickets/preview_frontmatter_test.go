package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/tickets"
)

func TestTicketFrontmatterFields_ShowsIDAndExpectedContextWindowUntilLanded(t *testing.T) {
	t.Parallel()
	tk := tickets.Ticket{
		Identifier:            "01-preview-missing-fields",
		Status:                "open",
		ExpectedContextWindow: 15000,
	}

	fields := ticketFrontmatterFields(tk, tickets.StatusOpen)

	var id, contextWindow string
	for _, f := range fields {
		switch f.key {
		case "id":
			id = f.value
		case "actual_context_window":
			contextWindow = f.value
		}
	}
	if id != "01-preview-missing-fields" {
		t.Fatalf("expected id field in preview fields, got %q", id)
	}
	if contextWindow != "Expected: 15.0k tok" {
		t.Fatalf("expected 'Expected: 15.0k tok' context-window field before landing, got %q", contextWindow)
	}
}

func TestTicketFrontmatterFields_ActualContextWindowReplacesExpectedOnceLanded(t *testing.T) {
	t.Parallel()
	tk := tickets.Ticket{
		Identifier:            "01-preview-missing-fields",
		Status:                "done",
		ExpectedContextWindow: 15000,
		ActualContextWindow:   19842,
	}

	fields := ticketFrontmatterFields(tk, tickets.StatusDone)

	var contextWindow string
	found := 0
	for _, f := range fields {
		if f.key == "actual_context_window" {
			contextWindow = f.value
			found++
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly one context-window field once landed, got %d", found)
	}
	if contextWindow != "19.8k tok" {
		t.Fatalf("expected actual context window to replace the expected one, got %q", contextWindow)
	}
}

// TestRenderFrontmatterBlock_WrapsLongValueToWidth covers ticket 06 (bugs-07):
// a frontmatter value long enough to overflow the pane (e.g. a blocked_by
// list with many entries) must wrap within width instead of producing one
// long line.
func TestRenderFrontmatterBlock_WrapsLongValueToWidth(t *testing.T) {
	t.Parallel()
	tk := tickets.Ticket{
		Type:      "implement",
		BlockedBy: []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12"},
	}

	const width = 30
	out := renderFrontmatterBlock(tk, tickets.StatusOpen, width)

	for _, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(ansi.Strip(line)); w > width {
			t.Fatalf("expected every line within width %d, got line of width %d:\n%q\nfull output:\n%s", width, w, line, out)
		}
	}
	if !strings.Contains(ansi.Strip(out), "12") {
		t.Fatalf("expected wrapped output to still contain the full blocked_by list, got:\n%s", ansi.Strip(out))
	}
}

// TestRenderFrontmatterBlock_NoRegressionForShortValues covers this ticket's
// second acceptance criterion: short field values render as a single line,
// unchanged by the added wrapping.
func TestRenderFrontmatterBlock_NoRegressionForShortValues(t *testing.T) {
	t.Parallel()
	tk := tickets.Ticket{Type: "implement"}

	out := renderFrontmatterBlock(tk, tickets.StatusOpen, 80)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 short frontmatter lines (status, type), got %d:\n%s", len(lines), out)
	}
}

func TestTicketFrontmatterFields_ShowsCategoryAndExportedAt(t *testing.T) {
	t.Parallel()
	tk := tickets.Ticket{
		Category:   "idea",
		ExportedAt: time.Date(2026, 10, 11, 9, 30, 0, 0, time.Local),
	}

	got := map[string]string{}
	for _, f := range ticketFrontmatterFields(tk, tickets.StatusOpen) {
		got[f.key] = f.value
	}
	if got["category"] != "idea" {
		t.Fatalf("expected category idea, got %q", got["category"])
	}
	if got["exported_at"] != "2026-10-11 09:30" {
		t.Fatalf("expected exported_at 2026-10-11 09:30, got %q", got["exported_at"])
	}
}

func TestPreviewEpicContent_ShowsEvolvedAtAndCompletedAt(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{
		Name:        "widgets",
		CompletedAt: time.Date(2026, 10, 10, 8, 0, 0, 0, time.Local),
		EvolvedAt:   time.Date(2026, 10, 11, 9, 30, 0, 0, time.Local),
	}

	out := ansi.Strip(previewEpicContent(epic, 80))
	for _, want := range []string{"Completed at: 2026-10-10 08:00", "Evolved at: 2026-10-11 09:30"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in epic preview, got:\n%s", want, out)
		}
	}
}

func TestPreviewEpicContent_OmitsUnsetTimes(t *testing.T) {
	t.Parallel()
	out := ansi.Strip(previewEpicContent(tickets.Epic{Name: "widgets"}, 80))
	if strings.Contains(out, "Completed at") || strings.Contains(out, "Evolved at") {
		t.Fatalf("expected no time rows for an epic without them, got:\n%s", out)
	}
}

func TestPreviewEpicContent_ShowsPlainEpicBodyAfterRule(t *testing.T) {
	t.Parallel()
	out := ansi.Strip(previewEpicContent(tickets.Epic{Name: "widgets", Body: "Implementation of the spec.\n"}, 80))
	rule := strings.Index(out, "─")
	body := strings.Index(out, "Implementation of the spec.")
	if rule == -1 || body == -1 || body < rule {
		t.Fatalf("expected a rule then the ticket.md body, got:\n%s", out)
	}
}

func TestPreviewEpicContent_NoRuleForEmptyBody(t *testing.T) {
	t.Parallel()
	out := ansi.Strip(previewEpicContent(tickets.Epic{Name: "widgets", Body: "\n"}, 80))
	if strings.Contains(out, "─") {
		t.Fatalf("expected no rule for an empty body, got:\n%s", out)
	}
}

func TestWithEpicTicketMD_ReadsServerModeEpicBodyFromDisk(t *testing.T) {
	t.Parallel()
	epicDir := t.TempDir()
	md := "---\nstatus: open\n---\nImplementation of the spec.\n"
	if err := os.WriteFile(filepath.Join(epicDir, "ticket.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	epic := tickets.Epic{Name: "p:widgets", Tickets: []tickets.Ticket{{File: filepath.Join(epicDir, "issues", "01-a.md")}}}

	if got := withEpicTicketMD(epic); got.Body != "Implementation of the spec.\n" {
		t.Fatalf("expected ticket.md body, got %q", got.Body)
	}
}

func TestWithEpicTicketMD_ReadsServerModeEpicTimesFromDisk(t *testing.T) {
	t.Parallel()
	epicDir := t.TempDir()
	md := "---\nstatus: done\ncompleted_at: 2026-10-10T08:00:00Z\nevolved_at: 2026-10-11T09:30:00Z\n---\n"
	if err := os.WriteFile(filepath.Join(epicDir, "ticket.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	epic := tickets.Epic{Name: "p:widgets", Tickets: []tickets.Ticket{{File: filepath.Join(epicDir, "issues", "01-a.md")}}}

	got := withEpicTicketMD(epic)
	if !got.CompletedAt.Equal(time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)) || !got.EvolvedAt.Equal(time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("expected ticket.md times, got completed=%v evolved=%v", got.CompletedAt, got.EvolvedAt)
	}
}
