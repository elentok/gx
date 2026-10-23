package cmd

import (
	"strings"
	"testing"
)

func TestExecute_TicketsValidate_BodyMentionMissingFromBlockedByWarns(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"ticket N": "Same rule as ticket 01 for the others.",
		"#N":       "See #01 for the rule.",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", ""))
			path := writeEpicTicket(t, dir, "e", "02-b.md", "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n"+body+"\n")

			stderr, err := validateStderr(t, path)
			if err != nil {
				t.Fatal(err)
			}
			want := "warning: " + path + ": ticket 02: body mentions ticket 01, which is not in blocked_by or parent\n"
			if stderr != want {
				t.Errorf("stderr = %q, want %q", stderr, want)
			}
		})
	}
}

func TestExecute_TicketsValidate_BodyMentionStaysQuiet(t *testing.T) {
	t.Parallel()
	frontmatter := func(id, extra, body string) string {
		return "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\n" + extra + "---\n" + body + "\n"
	}

	t.Run("covered by blocked_by", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", ""))
		path := writeEpicTicket(t, dir, "e", "02-b.md", frontmatter("02", "blocked_by:\n  - \"01\"\n", "After ticket 01."))
		if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
			t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
		}
	})
	t.Run("covered transitively", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", ""))
		writeEpicTicket(t, dir, "e", "02-b.md", ticketFrontmatter("02", "blocked_by:\n  - \"01\"\n"))
		path := writeEpicTicket(t, dir, "e", "03-c.md", frontmatter("03", "blocked_by:\n  - \"02\"\n", "Builds on ticket 01."))
		if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
			t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
		}
	})
	t.Run("covered by parent", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", ""))
		path := writeEpicTicket(t, dir, "e", "01a-b.md", frontmatter("01a", "parent: \"01\"\n", "Forked from ticket 01."))
		if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
			t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
		}
	})
	t.Run("done ticket skipped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeEpicTicket(t, dir, "e", "01-a.md", "---\nid: \"01\"\nstatus: done\ntype: implement\n---\nBody.\n")
		path := writeEpicTicket(t, dir, "e", "02-b.md", frontmatter("02", "", "Landed in ticket 01."))
		if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
			t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
		}
	})
	t.Run("bare number and own number ignored", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", ""))
		path := writeEpicTicket(t, dir, "e", "02-b.md", frontmatter("02", "", "Handle 01 items. This is ticket 02."))
		stderr, err := validateStderr(t, path)
		if err != nil || strings.Contains(stderr, "mentions") {
			t.Errorf("validate = %v, stderr %q, want no mention warning", err, stderr)
		}
	})
}
