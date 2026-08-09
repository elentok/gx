package tickets

import "testing"

func TestRenderedStatus_Draft(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "draft"}}}

	got := epic.RenderedStatus(epic.Tickets[0])
	if got == StatusOpen {
		t.Fatal("a draft ticket renders as open")
	}
	if got != StatusDraft {
		t.Fatalf("RenderedStatus() = %v, want StatusDraft", got)
	}
	if got.Word() != "draft" {
		t.Errorf("Word() = %q, want %q", got.Word(), "draft")
	}
}

func TestRenderedStatus_DraftKeepsItsStateWhenBlocked(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "draft", BlockedBy: []string{"01"}},
	}}

	if got := epic.RenderedStatus(epic.Tickets[1]); got != StatusDraft {
		t.Errorf("RenderedStatus() = %v, want StatusDraft (draft is not overlaid with blocked)", got)
	}
}

func TestIsDone_DraftIsNotDone(t *testing.T) {
	if (Ticket{Status: "draft"}).IsDone() {
		t.Error("a draft ticket counts as done")
	}
}
