package tickets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
)

func TestExecute_TicketsValidate_OldFormatMissingFrontmatterFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "01-a-ticket.md")
	cmdtest.WriteTicketFile(t, path, "Status: open\n\nBody.\n")

	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), nil)

	err := execute(t, []string{"tickets", "validate", path}, d)
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
	cmdtest.WriteTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), nil)

	if err := execute(t, []string{"tickets", "validate", path}, d); err != nil {
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
	cmdtest.WriteTicketFile(t, path, "---\nid: \"04b\"\nstatus: bogus-status\ntype: implement\n---\nBody.\n")

	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), nil)

	err := execute(t, []string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected error for invalid ticket, got nil")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error = %q, want it to mention the invalid status field", err.Error())
	}
}

func TestExecute_TicketsValidate_EpicWithoutTicketMDFails(t *testing.T) {
	t.Parallel()
	epicDir := filepath.Join(t.TempDir(), "proj", "old-epic")
	path := filepath.Join(epicDir, "issues", "01-a.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Dir(path), "01-a.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n")
	testutil.WriteFile(t, epicDir, "epic.yaml", "started_at: 2026-01-02T03:04:05Z\n")

	d := testDeps(t, bytes.NewBuffer(nil), bytes.NewBuffer(nil), nil)
	err := execute(t, []string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected error for an epic directory with no ticket.md, got nil")
	}
	if !strings.Contains(err.Error(), epicDir) || !strings.Contains(err.Error(), "ticket.md") {
		t.Errorf("error = %q, want it to name %s and ticket.md", err.Error(), epicDir)
	}
}

func TestExecute_TicketsValidate_MissingFileExitsNonZero(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), nil)

	err := execute(t, []string{"tickets", "validate", filepath.Join(t.TempDir(), "does-not-exist.md")}, d)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
