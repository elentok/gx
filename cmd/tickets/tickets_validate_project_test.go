package tickets

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
)

func TestExecute_TicketsValidate_ReportsErrorInSiblingEpic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := cmdtest.WriteEpicTicket(t, dir, "a", "01-first.md", cmdtest.TicketFrontmatter("01", ""))
	cmdtest.WriteEpicTicket(t, dir, "b", "01-broken.md", cmdtest.TicketFrontmatter("01", "blocked_by:\n  - \"09\"\n"))

	d := testDeps(t, bytes.NewBuffer(nil), bytes.NewBuffer(nil), nil)
	err := execute(t, []string{"tickets", "validate", path}, d)
	if err == nil {
		t.Fatal("expected the sibling epic's dangling blocked_by to fail validate, got nil")
	}
	if want := `blocked_by "09" names no ticket in this epic`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestExecute_TicketsValidate_DoneTicketSkipsGraphChecks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := cmdtest.WriteEpicTicket(t, dir, "e", "01-old.md",
		"---\nid: \"01\"\nstatus: done\ntype: implement\nblocked_by:\n  - \"09\"\n---\nBody.\n")

	d := testDeps(t, bytes.NewBuffer(nil), bytes.NewBuffer(nil), nil)
	if err := execute(t, []string{"tickets", "validate", path}, d); err != nil {
		t.Fatalf("a done ticket with a dangling blocker should pass: %v", err)
	}
}

func TestExecute_TicketsValidate_DoneTicketStillFailsPerFileChecks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := cmdtest.WriteEpicTicket(t, dir, "e", "01-old.md", "---\nid: \"01\"\nstatus: done\ntype: nonsense\n---\nBody.\n")

	d := testDeps(t, bytes.NewBuffer(nil), bytes.NewBuffer(nil), nil)
	if err := execute(t, []string{"tickets", "validate", path}, d); err == nil {
		t.Fatal("expected a per-file error for a done ticket with a bad type, got nil")
	}
}

func TestExecute_TicketsValidate_AllReportsErrorsAcrossProjects(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	good := cmdtest.AddProject(t, store, "good", "/repo/good")
	bad := cmdtest.AddProject(t, store, "bad", "/repo/bad")
	cmdtest.WriteEpicTicket(t, good, "e", "01-first.md", cmdtest.TicketFrontmatter("01", ""))
	cmdtest.WriteEpicTicket(t, bad, "e", "01-broken.md", cmdtest.TicketFrontmatter("01", "blocked_by:\n  - \"09\"\n"))

	_, err := runIn(t, t.TempDir(), "tickets", "validate", "--all")
	if err == nil {
		t.Fatal("expected --all to report the bad project's error, got nil")
	}
	if want := `blocked_by "09" names no ticket in this epic`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestExecute_TicketsValidate_AllPassesCleanStore(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	good := cmdtest.AddProject(t, store, "good", "/repo/good")
	cmdtest.WriteEpicTicket(t, good, "e", "01-first.md", cmdtest.TicketFrontmatter("01", ""))

	if _, err := runIn(t, t.TempDir(), "tickets", "validate", "--all"); err != nil {
		t.Fatalf("validate --all: %v", err)
	}
}
