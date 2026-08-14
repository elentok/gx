package tickets

import (
	"strings"
	"testing"

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
		Type:      "task",
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
	tk := tickets.Ticket{Type: "task"}

	out := renderFrontmatterBlock(tk, tickets.StatusOpen, 80)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 short frontmatter lines (status, type), got %d:\n%s", len(lines), out)
	}
}
