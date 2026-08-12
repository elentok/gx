package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTicketFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_TicketsValidate_OldFormatMissingFrontmatterFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "01-a-ticket.md")
	writeTicketFile(t, path, "Status: open\n\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected error for a file with no frontmatter block, got nil")
	}
	if !strings.Contains(err.Error(), "frontmatter") {
		t.Errorf("error = %q, want it to mention the missing frontmatter block", err.Error())
	}
}

func TestExecute_TicketsValidate_ValidNewFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	if err := execute([]string{"tickets", "validate", path}, d); err != nil {
		t.Fatalf("execute tickets validate: %v", err)
	}
	if !strings.Contains(stdout.String(), "04b") {
		t.Errorf("stdout = %q, want it to mention ticket id 04b", stdout.String())
	}
}

func TestExecute_TicketsValidate_InvalidTicketExitsNonZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: bogus-status\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected error for invalid ticket, got nil")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error = %q, want it to mention the invalid status field", err.Error())
	}
}

func TestExecute_TicketsValidate_MissingFileExitsNonZero(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	err := execute([]string{"tickets", "validate", filepath.Join(t.TempDir(), "does-not-exist.md")}, d)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
