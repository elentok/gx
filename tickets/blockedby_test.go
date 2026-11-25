package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlagDanglingBlockers_RendersErrorWithValidateMessage(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
		{Number: 3, Identifier: "03", Status: "open", BlockedBy: []string{"09"}},
	}}

	epic.flagDanglingBlockers()

	dangling := epic.Tickets[2]
	want := epic.CheckBlockedBy(dangling).Error()
	if dangling.BlockedByErr != want {
		t.Errorf("BlockedByErr = %q, want the validate message %q", dangling.BlockedByErr, want)
	}
	if got := epic.RenderedStatus(dangling); got != StatusError {
		t.Errorf("RenderedStatus(03) = %v, want StatusError", got)
	}
	if epic.Tickets[1].BlockedByErr != "" {
		t.Error("ticket 02's resolvable blocker was flagged")
	}
}

func TestCrossEpicRefusal(t *testing.T) {
	const ticketRule = "a ticket may only be blocked by tickets in its own epic"
	pointer := "to wait on other, run `gx tickets epic set d --blocked-by other`"
	other := Epic{Name: "other", Tickets: []Ticket{{Number: 6, Identifier: "06", Status: "open"}}}

	for _, ref := range []string{"other/06", "p:other/06", "other", "p:other"} {
		want := fmt.Sprintf("ticket 01: blocked_by %q: %s; %s", ref, ticketRule, pointer)
		epics := []Epic{other, {Name: "d", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{ref}}}}}

		t.Run("ValidateRefs "+ref, func(t *testing.T) {
			errs := ValidateRefs("p", epics, nil)
			if len(errs) != 1 || errs[0].Error() != "epic d: "+want {
				t.Errorf("errs = %v, want exactly %q", errs, "epic d: "+want)
			}
		})

		t.Run("Load "+ref, func(t *testing.T) {
			dir := writeCrossEpicStore(t, ref, "open")
			for name, load := range map[string]func() ([]Epic, error){
				"Load":         func() ([]Epic, error) { return Load(dir) },
				"LoadResolved": func() ([]Epic, error) { return LoadResolved("p", dir) },
			} {
				loaded, err := load()
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				d := loaded[0]
				if got := d.RenderedStatus(d.Tickets[0]); got != StatusError {
					t.Errorf("%s: status = %v, want error", name, got)
				}
				wantErr := want
				if name == "Load" && strings.HasPrefix(ref, "p:") {
					// Load does not know the project name, so it reads "p:" as another project.
					wantErr = fmt.Sprintf("ticket 01: blocked_by %q names another project (cross-project blocking not supported)", ref)
				}
				if got := d.Tickets[0].BlockedByErr; got != wantErr {
					t.Errorf("%s: BlockedByErr = %q, want %q", name, got, wantErr)
				}
			}
		})

		for status, wantStatus := range map[string]RenderedStatus{"done": StatusDone, "cancelled": StatusCancelled} {
			t.Run(status+" twin "+ref, func(t *testing.T) {
				dir := writeCrossEpicStore(t, ref, status)
				for name, load := range map[string]func() ([]Epic, error){
					"Load":         func() ([]Epic, error) { return Load(dir) },
					"LoadResolved": func() ([]Epic, error) { return LoadResolved("p", dir) },
				} {
					loaded, err := load()
					if err != nil {
						t.Fatalf("%s: %v", name, err)
					}
					d := loaded[0]
					if got := d.Tickets[0].BlockedByErr; got != "" {
						t.Errorf("%s: BlockedByErr = %q, want none", name, got)
					}
					if got, want := d.RenderedStatus(d.Tickets[0]), wantStatus; got != want {
						t.Errorf("%s: status = %v, want %v", name, got, want)
					}
				}
				if errs := ValidateRefs("p", []Epic{other, {Name: "d", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: status, BlockedBy: []string{ref}}}}}, nil); len(errs) != 0 {
					t.Errorf("ValidateRefs: errs = %v, want none for a %s ticket", errs, status)
				}
			})
		}
	}

	t.Run("epic blocked_by a ticket ref", func(t *testing.T) {
		for _, ref := range []string{"other/06", "p:other/06"} {
			epics := []Epic{other, {Name: "d", BlockedBy: []string{ref}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}}}
			errs := ValidateRefs("p", epics, nil)
			want := fmt.Sprintf("an epic may only be blocked by whole epics; %s", pointer)
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), fmt.Sprintf("%q: %s", ref, want)) {
				t.Errorf("%s: errs = %v, want one containing %q", ref, errs, want)
			}
		}
	})
}

// writeCrossEpicStore writes epic "d" with one ticket of the given status
// blocked on ref, and the epic "other" it names.
func writeCrossEpicStore(t *testing.T, ref, status string) string {
	t.Helper()
	dir := t.TempDir()
	for path, body := range map[string]string{
		"d/issues/01-x.md":     fmt.Sprintf("---\nid: \"01\"\nstatus: %s\ntype: implement\nblocked_by: [%q]\n---\n", status, ref),
		"other/issues/06-y.md": "---\nid: \"06\"\nstatus: open\ntype: implement\n---\n",
	} {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFlagDanglingBlockers_QualifiedEpicRefFailsClosed(t *testing.T) {
	epic := Epic{BlockedBy: []string{"a/01"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}}
	epic.flagDanglingBlockers()
	if got := epic.RenderedStatus(epic.Tickets[0]); got != StatusError {
		t.Errorf("got %v, want error", got)
	}
}

func TestLoadResolved_ResolvesEpicBlockerButFailsClosedOnUnresolved(t *testing.T) {
	dir := t.TempDir()
	write := func(path, body string) {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	issue := "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n"
	write("a/ticket.md", "---\nstatus: open\nmerged_at: 2026-01-02T03:04:05Z\n---\n")
	write("a/issues/01-x.md", "---\nid: \"01\"\nstatus: done\ntype: research\n---\n")
	write("b/ticket.md", "---\nstatus: open\nblocked_by: [a]\n---\n")
	write("b/issues/01-y.md", issue)
	write("c/ticket.md", "---\nstatus: open\nblocked_by: [zz]\n---\n")
	write("c/issues/01-z.md", issue)

	raw, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := raw[1].RenderedStatus(raw[1].Tickets[0]); got != StatusError {
		t.Errorf("plain Load b/01 = %v, want error (fails closed)", got)
	}

	epics, err := LoadResolved("p", dir)
	if err != nil {
		t.Fatal(err)
	}
	b, c := epics[1], epics[2]
	if got := b.RenderedStatus(b.Tickets[0]); got != StatusOpen {
		t.Errorf("b/01 = %v, want open once epic a is merged", got)
	}
	if got := c.RenderedStatus(c.Tickets[0]); got != StatusError {
		t.Errorf("c/01 = %v, want error for the unknown epic zz", got)
	}
}
