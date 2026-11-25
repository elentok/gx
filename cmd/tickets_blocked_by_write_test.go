package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
)

const (
	crossEpicTicketRule = "a ticket may only be blocked by tickets in its own epic; to wait on other, run `gx tickets epic set e --blocked-by other`"
	epicTicketRefRule   = "an epic may only be blocked by whole epics; to wait on other, run `gx tickets epic set e --blocked-by other`"
)

func blockedByDeps(dir, stdin string) (Deps, *bytes.Buffer) {
	stderr := bytes.NewBuffer(nil)
	return Deps{Env: cmdbase.Env{
		Stdout: bytes.NewBuffer(nil),
		Stderr: stderr,
		Stdin:  strings.NewReader(stdin),
		Getwd:  func() (string, error) { return dir, nil },
	}}, stderr
}

func assertUnchanged(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s was rewritten:\n%s", path, got)
	}
}

func TestExecute_TicketsSet_BlockedByRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, ref, want string
	}{
		{name: "dangling", ref: "09", want: `blocked_by "09" names no ticket in this epic`},
		{name: "cross-epic ticket", ref: "other/01", want: `blocked_by "other/01": ` + crossEpicTicketRule},
		{name: "whole other epic", ref: "other", want: `blocked_by "other": ` + crossEpicTicketRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
			content := ticketFrontmatter("02", "")
			path := writeEpicTicket(t, dir, "e", "02-second.md", content)

			d, _ := blockedByDeps(dir, "")
			err := execute([]string{"tickets", "set", path, "--blocked-by", tt.ref}, d)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
			assertUnchanged(t, path, content)
		})
	}
}

func TestExecute_TicketsSet_EpicBlockedByRefusesTicketRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
	writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
	epicFile := "---\nstatus: open\n---\n\n# e\n"
	path := filepath.Join(dir, "e", "ticket.md")
	if err := os.WriteFile(path, []byte(epicFile), 0o644); err != nil {
		t.Fatal(err)
	}

	d, _ := blockedByDeps(dir, "")
	err := execute([]string{"tickets", "set", path, "--blocked-by", "other/01"}, d)
	if want := `blocked_by "other/01": ` + epicTicketRefRule; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want it to contain %q", err, want)
	}
	assertUnchanged(t, path, epicFile)
}

func TestExecute_TicketsAdd_BlockedByRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, ref, want string
	}{
		{name: "dangling", ref: "09", want: `blocked_by "09" names no ticket in this epic`},
		{name: "cross-epic ticket", ref: "other/01", want: `blocked_by "other/01": ` + crossEpicTicketRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
			writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))
			epic := filepath.Join(dir, "e")

			d, _ := blockedByDeps(dir, "---\nblocked_by: [\""+tt.ref+"\"]\n---\n# New\n")
			err := execute([]string{"tickets", "add", epic, "--slug", "new", "--body", "-"}, d)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(epic, "issues", "02-new.md")); !os.IsNotExist(err) {
				t.Errorf("refused add still wrote the ticket (stat err %v)", err)
			}
		})
	}
}
