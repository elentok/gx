package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

func TestRunTicketsEnsureCodeReview_NoopWhenCodeReviewTicketExists(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "task")
	writeTicket(t, filepath.Join(issuesDir, "02-review.md"), "02", "ready-for-agent", "code-review")

	var stdout bytes.Buffer
	if err := runTicketsEnsureCodeReview(epicPath, &stdout); err != nil {
		t.Fatalf("runTicketsEnsureCodeReview: %v", err)
	}

	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("read issues dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("issues dir has %d entries, want 2 (no stub written)", len(entries))
	}
	if !strings.Contains(stdout.String(), "already has a code-review ticket") {
		t.Errorf("stdout = %q, want it to mention the existing code-review ticket", stdout.String())
	}
}

func TestRunTicketsEnsureCodeReview_CreatesValidStubWhenNoneExists(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "task")
	writeTicket(t, filepath.Join(issuesDir, "03-do-other-thing.md"), "03", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsEnsureCodeReview(epicPath, &stdout); err != nil {
		t.Fatalf("runTicketsEnsureCodeReview: %v", err)
	}

	stubPath := filepath.Join(issuesDir, "04-code-review.md")
	ticket, err := schema.ParseTicket(stubPath)
	if err != nil {
		t.Fatalf("stub ticket %s failed validate: %v", stubPath, err)
	}
	if ticket.Type != schema.TypeCodeReview {
		t.Errorf("stub ticket type = %q, want %q", ticket.Type, schema.TypeCodeReview)
	}
	if ticket.Status != schema.StatusReadyForAgent {
		t.Errorf("stub ticket status = %q, want %q", ticket.Status, schema.StatusReadyForAgent)
	}
	if ticket.ID != "04" {
		t.Errorf("stub ticket id = %q, want next sequential id 04", ticket.ID)
	}
}

// writeTicket writes a minimal valid ticket fixture with the given id,
// status, and type.
func writeTicket(t *testing.T, path, id, status, ticketType string) {
	t.Helper()
	content := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: " + ticketType + "\n---\n\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write ticket %s: %v", path, err)
	}
}
