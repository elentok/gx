package tickets

import (
	"testing"

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
