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

func TestExecute_TicketsValidate_BlockedByCycles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string // file name -> frontmatter extra
		ids   map[string]string // file name -> id
		path  string
		want  string
	}{
		{
			name:  "two-ticket cycle",
			files: map[string]string{"01-a.md": "blocked_by:\n  - \"02\"\n", "02-b.md": "blocked_by:\n  - \"01\"\n"},
			ids:   map[string]string{"01-a.md": "01", "02-b.md": "02"},
			path:  "01-a.md",
			want:  `blocked_by "02" closes a cycle: 01 → 02 → 01`,
		},
		{
			name:  "blocked by own ancestor",
			files: map[string]string{"01-a.md": "", "01a-fork.md": "parent: \"01\"\nblocked_by:\n  - \"01\"\n"},
			ids:   map[string]string{"01-a.md": "01", "01a-fork.md": "01a"},
			path:  "01a-fork.md",
			want:  `blocked_by "01" closes a cycle: 01a → 01 → 01a`,
		},
		{
			name:  "blocked by own descendant",
			files: map[string]string{"01-a.md": "blocked_by:\n  - \"01a\"\n", "01a-fork.md": "parent: \"01\"\n"},
			ids:   map[string]string{"01-a.md": "01", "01a-fork.md": "01a"},
			path:  "01-a.md",
			want:  `blocked_by "01a" closes a cycle: 01 → 01a → 01`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			paths := map[string]string{}
			for name, extra := range tt.files {
				paths[name] = writeEpicTicket(t, dir, "e", name, ticketFrontmatter(tt.ids[name], extra))
			}
			d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil)}
			err := execute([]string{"tickets", "validate", paths[tt.path]}, d)
			if err == nil {
				t.Fatal("expected a cycle error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
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
