package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeEpicTicket writes one ticket into scratchDir/<epic>/issues/, creating
// the tracker layout the epic-wide parent-graph checks need in order to find
// an epic at all.
func writeEpicTicket(t *testing.T, scratchDir, epic, filename, content string) string {
	t.Helper()
	issues := filepath.Join(scratchDir, epic, "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(issues, filename)
	writeTicketFile(t, path, content)
	return path
}

func ticketFrontmatter(id, extra string) string {
	return "---\nid: \"" + id + "\"\nstatus: open\ntype: task\n" + extra + "---\nBody.\n"
}

func TestExecute_TicketsSet_ParentAcceptedForExistingTicket(t *testing.T) {
	dir := t.TempDir()
	writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
	path := writeEpicTicket(t, dir, "e", "01a-fork.md", ticketFrontmatter("01a", ""))

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	if err := execute([]string{"tickets", "set", path, "--parent=01"}, d); err != nil {
		t.Fatalf("execute tickets set --parent: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "parent:") {
		t.Errorf("ticket file = %q, want parent written", string(raw))
	}
}

func TestExecute_TicketsSet_ParentRejectedWhenAbsentFromEpic(t *testing.T) {
	dir := t.TempDir()
	path := writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err = execute([]string{"tickets", "set", path, "--parent=09"}, d)
	if err == nil {
		t.Fatal("expected an error for a parent naming no ticket in the epic, got nil")
	}
	if !strings.Contains(err.Error(), "09") {
		t.Errorf("error = %q, want it to name the offending id", err)
	}
	assertFileUnchanged(t, path, before)
}

func TestExecute_TicketsSet_ParentRejectedWhenItWouldCloseACycle(t *testing.T) {
	dir := t.TempDir()
	path := writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
	writeEpicTicket(t, dir, "e", "01a-fork.md", ticketFrontmatter("01a", "parent: \"01\"\n"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err = execute([]string{"tickets", "set", path, "--parent=01a"}, d)
	if err == nil {
		t.Fatal("expected an error for a parent inside the ticket's own fork subtree, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to report a cycle", err)
	}
	assertFileUnchanged(t, path, before)
}

func TestExecute_TicketsValidate_ReportsInvalidParentEdge(t *testing.T) {
	dir := t.TempDir()
	path := writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", "parent: \"09\"\n"))

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	err := execute([]string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected validate to reject a parent naming no ticket in the epic")
	}
	if !strings.Contains(err.Error(), "09") {
		t.Errorf("error = %q, want it to name the offending id", err)
	}
}

func TestExecute_TicketsValidate_AcceptsDraftStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01-first.md")
	writeTicketFile(t, path, "---\nid: \"01\"\nstatus: draft\ntype: task\n---\nBody.\n")

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}
	if err := execute([]string{"tickets", "validate", path}, d); err != nil {
		t.Fatalf("execute tickets validate: %v", err)
	}
	if !strings.Contains(stdout.String(), "status=draft") {
		t.Errorf("stdout = %q, want it to report status=draft", stdout.String())
	}
}

func TestExecute_TicketsSet_AcceptsDraftStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01-first.md")
	writeTicketFile(t, path, "---\nid: \"01\"\nstatus: open\ntype: task\n---\nBody.\n")

	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
	if err := execute([]string{"tickets", "set", path, "--status=draft"}, d); err != nil {
		t.Fatalf("execute tickets set --status=draft: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: draft") {
		t.Errorf("ticket file = %q, want status: draft", string(raw))
	}
}

func assertFileUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("ticket file changed despite a rejected write:\nbefore: %q\nafter:  %q", before, after)
	}
}
