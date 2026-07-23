package tickets

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTicket_FullMetadata(t *testing.T) {
	raw := "Type: prototype\nBlocked by: 02, 05\nStatus: resolved\n\n## Question\n\nBody text.\n"

	ticket, err := ParseTicket(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket.Type != "prototype" {
		t.Errorf("Type = %q, want %q", ticket.Type, "prototype")
	}
	if !reflect.DeepEqual(ticket.BlockedBy, []int{2, 5}) {
		t.Errorf("BlockedBy = %v, want [2 5]", ticket.BlockedBy)
	}
	if ticket.Status != "resolved" {
		t.Errorf("Status = %q, want %q", ticket.Status, "resolved")
	}
	wantBody := "\n## Question\n\nBody text.\n"
	if ticket.Body != wantBody {
		t.Errorf("Body = %q, want %q", ticket.Body, wantBody)
	}
}

func TestParseTicket_MissingStatusDefaultsToOpen(t *testing.T) {
	raw := "Type: task\nBlocked by: -\n\nSome body.\n"

	ticket, err := ParseTicket(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket.Status != "" {
		t.Errorf("Status = %q, want empty (missing Status: is not an error)", ticket.Status)
	}
	if ticket.IsDone() {
		t.Error("ticket with missing Status should not be IsDone")
	}
}

func TestParseTicket_BoldNonContiguousMetadata(t *testing.T) {
	raw := "# 09 — Some ticket\n\n**What to build:** some prose paragraph that isn't metadata.\n\n" +
		"**Blocked by:** 02 (parse & load).\n\n**Status:** done\n\n- [x] a checklist item\n"

	ticket, err := ParseTicket(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket.Status != "done" {
		t.Errorf("Status = %q, want %q", ticket.Status, "done")
	}
	if !ticket.IsDone() {
		t.Error("expected ticket with bold '**Status:** done' to be IsDone")
	}
	if !reflect.DeepEqual(ticket.BlockedBy, []int{2}) {
		t.Errorf("BlockedBy = %v, want [2]", ticket.BlockedBy)
	}
	if strings.Contains(ticket.Body, "Blocked by") || strings.Contains(ticket.Body, "Status") {
		t.Errorf("expected metadata lines stripped from body, got: %q", ticket.Body)
	}
	if !strings.Contains(ticket.Body, "some prose paragraph") || !strings.Contains(ticket.Body, "a checklist item") {
		t.Errorf("expected surrounding prose/checklist preserved in body, got: %q", ticket.Body)
	}
}

func TestParseTicket_NoMetadataAtAll(t *testing.T) {
	raw := "# Just a heading\n\nNo metadata lines here.\n"

	ticket, err := ParseTicket(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket.Type != "" || ticket.Status != "" || ticket.BlockedBy != nil {
		t.Errorf("expected no metadata parsed, got %+v", ticket)
	}
	if ticket.Body != raw {
		t.Errorf("Body = %q, want entire raw text %q", ticket.Body, raw)
	}
}

func TestParseTicket_BlockedByNoneOrDash(t *testing.T) {
	for _, value := range []string{"-", "None", "none"} {
		raw := "Blocked by: " + value + "\nStatus: open\n"
		ticket, err := ParseTicket(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ticket.BlockedBy != nil {
			t.Errorf("BlockedBy(%q) = %v, want nil", value, ticket.BlockedBy)
		}
	}
}

func TestTicket_IsDone(t *testing.T) {
	doneValues := []string{"done", "resolved", "wontfix", "closed", "superseded", "Done", "RESOLVED"}
	for _, v := range doneValues {
		ticket := Ticket{Status: v}
		if !ticket.IsDone() {
			t.Errorf("Status %q should be IsDone", v)
		}
	}

	notDoneValues := []string{"", "open", "claimed", "needs-info", "ready-for-agent", "blocked", "bogus"}
	for _, v := range notDoneValues {
		ticket := Ticket{Status: v}
		if ticket.IsDone() {
			t.Errorf("Status %q should not be IsDone", v)
		}
	}
}
