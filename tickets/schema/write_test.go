package schema

import (
	"os"
	"strings"
	"testing"
)

func TestUpdateTicket_MutatesAndWrites(t *testing.T) {
	path := writeTemp(t, "04b-ticket.md", "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	err := UpdateTicket(path, func(t *Ticket) {
		t.Status = StatusClaimed
	})
	if err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if !strings.Contains(string(raw), "status: claimed") {
		t.Errorf("ticket file = %q, want status: claimed", string(raw))
	}
	if !strings.Contains(string(raw), "Body.") {
		t.Errorf("ticket file = %q, want body preserved", string(raw))
	}
}

func TestUpdateTicket_ValidationFailureWritesNothing(t *testing.T) {
	original := "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n"
	path := writeTemp(t, "04b-ticket.md", original)

	err := UpdateTicket(path, func(t *Ticket) {
		t.Status = Status("bogus")
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(raw) != original {
		t.Errorf("file changed on validation failure: got %q, want %q", string(raw), original)
	}
}
