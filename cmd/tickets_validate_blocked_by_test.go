package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecute_TicketsValidate_DanglingBlockedByFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
	path := writeEpicTicket(t, dir, "e", "02-second.md", ticketFrontmatter("02", "blocked_by:\n  - \"09\"\n"))

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err := execute([]string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected an error for a blocked_by naming no ticket, got nil")
	}
	if want := `blocked_by "09" names no ticket in this epic`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestExecute_TicketsValidate_QualifiedBlockedByFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeEpicTicket(t, dir, "e", "02-second.md", ticketFrontmatter("02", "blocked_by:\n  - \"other/06\"\n"))

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err := execute([]string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected an error for a qualified blocked_by, got nil")
	}
	if want := `blocked_by "other/06" is malformed`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestExecute_TicketsValidate_ResolvableBlockedByPasses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
	path := writeEpicTicket(t, dir, "e", "02-second.md", ticketFrontmatter("02", "blocked_by:\n  - \"01\"\n"))

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	if err := execute([]string{"tickets", "validate", path}, d); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
