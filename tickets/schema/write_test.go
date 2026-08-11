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

func TestUpdateTicket_AppendsToSessionIDsWithoutOverwriting(t *testing.T) {
	path := writeTemp(t, "04b-ticket.md", "---\nid: \"04b\"\nstatus: claimed\ntype: task\nsession_ids: [\"sess-1\"]\n---\nBody.\n")

	err := UpdateTicket(path, func(t *Ticket) {
		t.SessionIDs = append(t.SessionIDs, "sess-2")
	})
	if err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}

	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	want := []string{"sess-1", "sess-2"}
	if len(got.SessionIDs) != len(want) || got.SessionIDs[0] != want[0] || got.SessionIDs[1] != want[1] {
		t.Errorf("SessionIDs = %v, want %v", got.SessionIDs, want)
	}
}

func TestClearIterationStatus_ClearsFieldLeavesStatusUntouched(t *testing.T) {
	path := writeTemp(t, "04b-ticket.md", "---\nid: \"04b\"\nstatus: claimed\ntype: task\niteration_status: finished\n---\nBody.\n")

	if err := ClearIterationStatus(path); err != nil {
		t.Fatalf("ClearIterationStatus: %v", err)
	}

	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	if got.IterationStatus != "" {
		t.Errorf("IterationStatus = %q, want empty", got.IterationStatus)
	}
	if got.Status != StatusClaimed {
		t.Errorf("Status = %q, want unchanged %q", got.Status, StatusClaimed)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if strings.Contains(string(raw), "iteration_status") {
		t.Errorf("ticket file = %q, want no iteration_status key", string(raw))
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
