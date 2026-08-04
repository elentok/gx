package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecute_TicketsSet_MultiFieldSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=claimed", "--blocked-by=01,03"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}
	if !strings.Contains(stdout.String(), "updated (status=claimed, blocked_by=01,03)") {
		t.Errorf("stdout = %q, want it to list the changed fields", stdout.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "status: claimed") {
		t.Errorf("ticket file = %q, want status: claimed", string(raw))
	}
	if !strings.Contains(string(raw), `blocked_by:`) {
		t.Errorf("ticket file = %q, want blocked_by written", string(raw))
	}
}

func TestExecute_TicketsSet_RefusedFieldUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--actual-context-window=100"}, d)
	if err == nil {
		t.Fatal("expected an unknown-flag error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want it to mention unknown flag", err.Error())
	}
}

func TestExecute_TicketsSet_ValidationFailureWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	original := "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n"
	writeTicketFile(t, path, original)

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=bogus-status"}, d)
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error = %q, want it to mention the invalid status field", err.Error())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if string(raw) != original {
		t.Errorf("ticket file changed on validation failure: got %q, want unchanged %q", string(raw), original)
	}
}

func TestExecute_TicketsSet_Commitless(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--status=done", "--commitless=true"}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if !strings.Contains(string(raw), "commitless: true") {
		t.Errorf("ticket file = %q, want commitless: true written", string(raw))
	}
}

func TestExecute_TicketsSet_ClearingListField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\nblocked_by: [\"01\", \"03\"]\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "set", path, "--blocked-by="}, d)
	if err != nil {
		t.Fatalf("execute tickets set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading ticket back: %v", err)
	}
	if strings.Contains(string(raw), "blocked_by") {
		t.Errorf("ticket file = %q, want blocked_by cleared entirely (omitempty)", string(raw))
	}
}
