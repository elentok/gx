package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

// setupEpicBlockedByProject is project "mine" with loop epics e and x and map
// epic m, each holding ticket 01; x/01 waits on xBlockedBy when set.
func setupEpicBlockedByProject(t *testing.T, xBlockedBy string) (d deps, ticketMD string) {
	t.Helper()
	d, ticketMD = setupEpicSetProject(t)
	project := filepath.Dir(filepath.Dir(ticketMD))
	extra := ""
	if xBlockedBy != "" {
		extra = "blocked_by:\n  - \"" + xBlockedBy + "\"\n"
	}
	writeEpicTicket(t, project, "e", "01-a.md", ticketFrontmatter("01", ""))
	writeEpicTicket(t, project, "x", "01-a.md", ticketFrontmatter("01", extra))
	writeEpicTicket(t, project, "m", "01-a.md", ticketFrontmatter("01", ""))
	for epic, kind := range map[string]string{"x": "loop", "m": "map"} {
		if err := os.WriteFile(filepath.Join(project, epic, "ticket.md"), []byte("---\nkind: "+kind+"\nstatus: open\n---\n\n# "+epic+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d, ticketMD
}

func TestTicketsEpicSet_BlockedByWritesWholeEpicToken(t *testing.T) {
	d, ticketMD := setupEpicBlockedByProject(t, "")

	if err := execute([]string{"tickets", "epic", "set", "mine:e", "--blocked-by", "x"}, d); err != nil {
		t.Fatalf("epic set: %v", err)
	}
	raw, _ := os.ReadFile(ticketMD)
	for _, want := range []string{"blocked_by:\n    - x\n", "custom: keep-me", "body words"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ticket.md missing %q:\n%s", want, raw)
		}
	}
}

func TestTicketsEpicSet_BlockedByRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, xBlockedBy, ref, code, want string
	}{
		{name: "dangling", ref: "nope", code: tickets.CodeUnknownEpic},
		{name: "map epic", ref: "m", code: tickets.CodeBlockedByMapEpic},
		{name: "own ticket", ref: "01", want: `epic e: blocked_by "01" names a ticket of this epic`},
		{name: "cycle", xBlockedBy: "e", ref: "x", want: `epic e: blocked_by "x" closes a cycle: e → x → x/01 → e`},
		{name: "widened cycle", xBlockedBy: "e/01", ref: "x/01", want: `epic e: blocked_by "x/01" (widened to epic "x") closes a cycle`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, ticketMD := setupEpicBlockedByProject(t, tc.xBlockedBy)
			err := execute([]string{"tickets", "epic", "set", "mine:e", "--blocked-by", tc.ref}, d)
			var addrErr *tickets.AddressError
			switch {
			case err == nil:
				t.Fatal("want a refusal")
			case tc.code != "" && (!errors.As(err, &addrErr) || addrErr.Code != tc.code):
				t.Errorf("err = %v, want code %s", err, tc.code)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
			if raw, _ := os.ReadFile(ticketMD); string(raw) != epicTicketMD {
				t.Errorf("ticket.md changed:\n%s", raw)
			}
		})
	}
}

func TestTicketsValidate_ReportsBadEpicLevelBlockedBy(t *testing.T) {
	d, ticketMD := setupEpicBlockedByProject(t, "")
	if err := os.WriteFile(ticketMD, []byte("---\nkind: loop\nstatus: open\nblocked_by:\n  - nope\n---\n\n# e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.stdout, d.stderr = &bytes.Buffer{}, &bytes.Buffer{}
	err := execute([]string{"tickets", "validate", filepath.Join(filepath.Dir(ticketMD), "issues", "01-a.md")}, d)
	if want := `ticket.md: epic e: blocked_by unknown-epic: "nope" names no epic in this project`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want it to contain %q", err, want)
	}
}
