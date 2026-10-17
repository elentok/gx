package tickets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// wholeEpic is a project where epic "d" waits on the whole epic "a".
func wholeEpic(aTickets []Ticket, completed, merged bool) []Epic {
	a := Epic{Name: "a", Tickets: aTickets}
	if completed {
		a.CompletedAt = time.Now()
	}
	if merged {
		a.MergedAt = time.Now()
	}
	return []Epic{a, {Name: "d", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a"}}}}}
}

func doneTicket() []Ticket { return []Ticket{{Number: 1, Identifier: "01", Status: "done"}} }

func dependentStatus(release EpicRelease, epics []Epic) RenderedStatus {
	d := ResolveCrossEpic("p", release, epics)[1]
	return d.RenderedStatus(d.Tickets[0])
}

func TestResolveCrossEpic_WholeEpicReleasesOnPolicyStamp(t *testing.T) {
	onLanded := EpicRelease{}
	onDone := EpicRelease{OnDone: true}
	onDoneFF := EpicRelease{OnDone: true, AutoFFMerge: true}
	for _, tc := range []struct {
		name              string
		release           EpicRelease
		completed, merged bool
		want              RenderedStatus
	}{
		{"on-landed unstamped", onLanded, false, false, StatusBlocked},
		{"on-landed completed only", onLanded, true, false, StatusBlocked},
		{"on-landed merged", onLanded, true, true, StatusOpen},
		{"on-done unstamped", onDone, false, false, StatusBlocked},
		{"on-done completed", onDone, true, false, StatusOpen},
		{"on-done+ff completed only (root park)", onDoneFF, true, false, StatusBlocked},
		{"on-done+ff merged without completed", onDoneFF, false, true, StatusBlocked},
		{"on-done+ff completed and merged", onDoneFF, true, true, StatusOpen},
	} {
		if got := dependentStatus(tc.release, wholeEpic(doneTicket(), tc.completed, tc.merged)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestResolveCrossEpic_WholeEpicBlocksWhileATicketIsOpen(t *testing.T) {
	reopened := append(doneTicket(), Ticket{Number: 2, Identifier: "02", Status: "open"})
	for _, release := range []EpicRelease{{}, {OnDone: true}, {OnDone: true, AutoFFMerge: true}} {
		if got := dependentStatus(release, wholeEpic(reopened, true, true)); got != StatusBlocked {
			t.Errorf("%+v: ticket added to a stamped epic: got %v, want blocked", release, got)
		}
	}
}

func TestResolveCrossEpic_EmptyEpicBlocksUntilStamped(t *testing.T) {
	if got := dependentStatus(EpicRelease{}, wholeEpic(nil, false, false)); got != StatusBlocked {
		t.Errorf("unstamped empty epic: got %v, want blocked", got)
	}
	if got := dependentStatus(EpicRelease{}, wholeEpic(nil, false, true)); got != StatusOpen {
		t.Errorf("merged empty epic: got %v, want open", got)
	}
}

func TestResolveBlocker_TokenSyntax(t *testing.T) {
	g := newProjectGraph("p", []Epic{
		{Name: "a", Tickets: doneTicket()},
		{Name: "12", Tickets: doneTicket()},
		{Name: "d", Tickets: doneTicket()},
	})
	for ref, want := range map[string]string{
		"a":      "a",
		"p:a":    "a",
		"p:12":   "12",
		"01":     "d/1",
		"a/01":   "a", // commitful, so widened
		"p:a/01": "a",
	} {
		if got, err := g.resolveBlocker("d", ref); err != nil || got != want {
			t.Errorf("%q: got (%q, %v), want %q", ref, got, err, want)
		}
	}
	// An all-numeric token is always a ticket id, never the epic "12".
	if _, err := g.resolveBlocker("d", "12"); err == nil || strings.Contains(err.Error(), CodeUnknownEpic) {
		t.Errorf(`"12": err = %v, want a missing-ticket error`, err)
	}
}

func TestResolveBlocker_RejectsWithCodes(t *testing.T) {
	g := newProjectGraph("p", []Epic{
		{Name: "a", Tickets: doneTicket()},
		{Name: "m", IsMap: true, Tickets: doneTicket()},
		{Name: "d", Tickets: doneTicket()},
	})
	for ref, code := range map[string]string{
		"q:a": CodeBlockedByOtherProject,
		"d":   CodeBlockedByOwnEpic,
		"p:d": CodeBlockedByOwnEpic,
		"m":   CodeBlockedByMapEpic,
		"zz":  CodeUnknownEpic,
	} {
		_, err := g.resolveBlocker("d", ref)
		var ae *AddressError
		if !errors.As(err, &ae) || ae.Code != code {
			t.Errorf("%q: err = %v, want code %s", ref, err, code)
		}
	}
}

func TestResolveCrossEpic_UnknownEpicIsError(t *testing.T) {
	epics := wholeEpic(doneTicket(), true, true)
	epics[1].Tickets[0].BlockedBy = []string{"zz"}
	if got := dependentStatus(EpicRelease{}, epics); got != StatusError {
		t.Errorf("got %v, want error", got)
	}
}

func TestResolveCrossEpic_EpicLevelErrorSkipsTerminalTickets(t *testing.T) {
	epics := []Epic{{Name: "d", BlockedBy: []string{"zz"}, Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}}
	d := ResolveCrossEpic("p", EpicRelease{}, epics)[0]
	if got := d.Tickets[0].BlockedByErr; got != "" {
		t.Errorf("done ticket: BlockedByErr = %q, want none", got)
	}
	if got := d.RenderedStatus(d.Tickets[1]); got != StatusError {
		t.Errorf("open ticket: got %v, want error", got)
	}
}

func TestValidateRefs_WholeEpicCycleNamesTheEpic(t *testing.T) {
	epics := []Epic{
		{Name: "a", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"d/01"}}}},
		{Name: "d", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a"}}}},
	}
	errs := ValidateRefs("p", epics)
	if len(errs) == 0 || !strings.Contains(errors.Join(errs...).Error(), "01 → a → a/01 → 01") {
		t.Errorf("errs = %v, want a cycle through the whole epic a", errs)
	}
}

// widenedProject is epic "d" whose ticket waits on "a/01"; epic "a" also
// holds an open "02".
func widenedProject(commitless bool, merged bool) []Epic {
	a := Epic{Name: "a", Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Commitless: commitless},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	if merged {
		a.CompletedAt, a.MergedAt = time.Now(), time.Now()
	}
	return []Epic{a, {Name: "d", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a/01"}}}}}
}

func TestResolveCrossEpic_CommitfulTicketWidensToItsEpic(t *testing.T) {
	d := ResolveCrossEpic("p", EpicRelease{}, widenedProject(false, false))[1]
	if got := d.RenderedStatus(d.Tickets[0]); got != StatusBlocked {
		t.Errorf("commitful a/01 done, a/02 open: got %v, want blocked", got)
	}
	if got := d.Tickets[0].WidenedBlockers; got["a/01"] != "a" {
		t.Errorf("WidenedBlockers = %v, want a/01 → a", got)
	}

	epics := widenedProject(false, false)
	epics[0].Tickets[1].Status = "done"
	d = ResolveCrossEpic("p", EpicRelease{}, epics)[1]
	if got := d.RenderedStatus(d.Tickets[0]); got != StatusBlocked {
		t.Errorf("whole epic done but unstamped: got %v, want blocked", got)
	}

	epics = widenedProject(false, true)
	epics[0].Tickets[1].Status = "done"
	d = ResolveCrossEpic("p", EpicRelease{}, epics)[1]
	if got := d.RenderedStatus(d.Tickets[0]); got != StatusOpen {
		t.Errorf("whole epic done and merged: got %v, want open", got)
	}
}

func TestResolveCrossEpic_CommitlessTicketNarrowsBack(t *testing.T) {
	d := ResolveCrossEpic("p", EpicRelease{}, widenedProject(true, false))[1]
	if got := d.RenderedStatus(d.Tickets[0]); got != StatusOpen {
		t.Errorf("commitless a/01 done: got %v, want open", got)
	}
	if got := d.Tickets[0].WidenedBlockers; got != nil {
		t.Errorf("WidenedBlockers = %v, want none", got)
	}
}

func TestValidateRefs_WidenedOnlyCycleNamesTheToken(t *testing.T) {
	// Ticket-level there is no cycle: a/01 → d/02, d/01 → a/01. Widened,
	// a/01 waits on all of d, d/01 included.
	epics := []Epic{
		{Name: "a", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"d/02"}}}},
		{Name: "d", Tickets: []Ticket{
			{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a/01"}},
			{Number: 2, Identifier: "02", Status: "open"},
		}},
	}
	msg := errors.Join(ValidateRefs("p", epics)...).Error()
	if !strings.Contains(msg, `blocked_by "d/02" (widened to epic "d") closes a cycle`) {
		t.Errorf("errs = %s, want the widened d/02 token named", msg)
	}

	a := ResolveCrossEpic("p", EpicRelease{}, epics)[0]
	if got := a.RenderedStatus(a.Tickets[0]); got != StatusError || !strings.Contains(a.Tickets[0].BlockedByErr, "widened") {
		t.Errorf("loader: status %v, BlockedByErr %q, want error with the cycle", got, a.Tickets[0].BlockedByErr)
	}

	epics[1].Tickets[1].Commitless = true
	epics[0].Tickets[0].Commitless = true
	if errs := ValidateRefs("p", epics); len(errs) != 0 {
		t.Errorf("both targets commitless: errs = %v, want none", errs)
	}
}

func TestLoadResolved_ReadsReleasePolicyFromProjectFile(t *testing.T) {
	dir := t.TempDir()
	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "a", "ticket.md"), "---\nstatus: open\ncompleted_at: 2026-01-02T03:04:05Z\n---\n")
	write(filepath.Join(dir, "a", "issues", "01-x.md"), "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n")
	write(filepath.Join(dir, "d", "issues", "01-y.md"), "---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by: [a]\n---\n")

	status := func() RenderedStatus {
		epics, err := LoadResolved("p", dir)
		if err != nil {
			t.Fatal(err)
		}
		d := epics[1]
		return d.RenderedStatus(d.Tickets[0])
	}
	if got := status(); got != StatusBlocked {
		t.Errorf("default on-landed, completed_at only: got %v, want blocked", got)
	}
	write(filepath.Join(dir, "project.json"), `{"landing":"on-done"}`)
	if got := status(); got != StatusOpen {
		t.Errorf("on-done, completed_at: got %v, want open", got)
	}
}
