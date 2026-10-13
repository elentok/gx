package tickets

import (
	"os"
	"path/filepath"
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

func crossEpics() []Epic {
	return []Epic{
		{Name: "a", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", Commitless: true}}},
		{Name: "b", BlockedBy: []string{"a/01"}, Tickets: []Ticket{
			{Number: 1, Identifier: "01", Status: "open"},
			{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
		}},
		{Name: "c", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a/01"}}}},
	}
}

func TestResolveCrossEpic_BlocksUntilBlockerDone(t *testing.T) {
	epics := ResolveCrossEpic("p", EpicRelease{}, crossEpics())
	for _, e := range epics[1:] {
		if got := e.RenderedStatus(e.Tickets[0]); got != StatusBlocked {
			t.Errorf("epic %s ticket 01 = %v, want blocked", e.Name, got)
		}
	}

	done := crossEpics()
	done[0].Tickets[0].Status = "done"
	for _, e := range ResolveCrossEpic("p", EpicRelease{}, done)[1:] {
		if got := e.RenderedStatus(e.Tickets[0]); got != StatusOpen {
			t.Errorf("epic %s ticket 01 = %v, want open once a/01 is done", e.Name, got)
		}
	}
}

func TestResolveCrossEpic_BadRefIsError(t *testing.T) {
	epics := crossEpics()
	epics[2].Tickets[0].BlockedBy = []string{"a/09"}
	c := ResolveCrossEpic("p", EpicRelease{}, epics)[2]
	if got := c.RenderedStatus(c.Tickets[0]); got != StatusError {
		t.Errorf("got %v, want error", got)
	}
}

func TestFlagDanglingBlockers_QualifiedRefFailsClosed(t *testing.T) {
	for name, epic := range map[string]Epic{
		"ticket ref": {Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a/01"}}}},
		"epic ref":   {BlockedBy: []string{"a/01"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}},
	} {
		epic.flagDanglingBlockers()
		if got := epic.RenderedStatus(epic.Tickets[0]); got != StatusError {
			t.Errorf("%s: got %v, want error", name, got)
		}
	}
}

func TestLoadResolved_ResolvesCrossEpicBlockerButFailsClosedOnUnresolved(t *testing.T) {
	dir := t.TempDir()
	write := func(epic, name, body string) {
		issues := filepath.Join(dir, epic, "issues")
		if err := os.MkdirAll(issues, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(issues, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a", "01-x.md", "---\nid: \"01\"\nstatus: done\ntype: research\n---\n")
	write("b", "01-y.md", "---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by: [a/01]\n---\n")
	write("b", "02-z.md", "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [a/09]\n---\n")

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
	b := epics[1]
	if got := b.RenderedStatus(b.Tickets[0]); got != StatusOpen {
		t.Errorf("b/01 = %v, want open once a/01 is done", got)
	}
	if got := b.RenderedStatus(b.Tickets[1]); got != StatusError {
		t.Errorf("b/02 = %v, want error for a/09", got)
	}
}
