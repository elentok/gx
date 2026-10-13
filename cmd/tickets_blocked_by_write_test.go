package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const widenNotice = "`other/01` needs commits; waiting for all of `other`"

func blockedByDeps(dir, stdin string) (deps, *bytes.Buffer) {
	stderr := bytes.NewBuffer(nil)
	return deps{
		stdout: bytes.NewBuffer(nil),
		stderr: stderr,
		stdin:  strings.NewReader(stdin),
		getwd:  func() (string, error) { return dir, nil },
	}, stderr
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
		{name: "widened cycle", ref: "other/01", want: `blocked_by "other/01" (widened to epic "other") closes a cycle`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
			writeEpicTicket(t, dir, "other", "02-dep.md", ticketFrontmatter("02", "blocked_by:\n  - \"e/02\"\n"))
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

func TestExecute_TicketsSet_BlockedByWideningPrintsNotice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
	path := writeEpicTicket(t, dir, "e", "02-second.md", ticketFrontmatter("02", ""))

	d, stderr := blockedByDeps(dir, "")
	if err := execute([]string{"tickets", "set", path, "--blocked-by", "other/01"}, d); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(stderr.String(), widenNotice) {
		t.Errorf("stderr = %q, want the widening notice", stderr)
	}
}

func TestExecute_TicketsAdd_BlockedByRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, ref, want string
	}{
		{name: "dangling", ref: "09", want: `blocked_by "09" names no ticket in this epic`},
		{name: "widened cycle", ref: "other/01", want: `blocked_by "other/01" (widened to epic "other") closes a cycle`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", "blocked_by:\n  - \"e\"\n"))
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

func TestExecute_TicketsAdd_BlockedByWideningPrintsNotice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
	writeEpicTicket(t, dir, "e", "01-first.md", ticketFrontmatter("01", ""))

	d, stderr := blockedByDeps(dir, "---\nblocked_by: [\"other/01\"]\n---\n# New\n")
	if err := execute([]string{"tickets", "add", filepath.Join(dir, "e"), "--slug", "new", "--body", "-"}, d); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(stderr.String(), widenNotice) {
		t.Errorf("stderr = %q, want the widening notice", stderr)
	}
}

func TestExecute_TicketsValidate_SilentOnWidenedToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeEpicTicket(t, dir, "other", "01-code.md", ticketFrontmatter("01", ""))
	path := writeEpicTicket(t, dir, "e", "02-second.md", ticketFrontmatter("02", "blocked_by:\n  - \"other/01\"\n"))

	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	if err := execute([]string{"tickets", "validate", path}, deps{stdout: stdout, stderr: stderr}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if out := stdout.String() + stderr.String(); strings.Contains(out, "needs commits") || strings.Contains(out, "widen") {
		t.Errorf("validate mentioned widening: %q", out)
	}
}
