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
	return []Epic{a, {Name: "d", BlockedBy: []string{"a"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}}}
}

func doneTicket() []Ticket { return []Ticket{{Number: 1, Identifier: "01", Status: "done"}} }

func dependentStatus(release EpicRelease, epics []Epic) RenderedStatus {
	d := resolveEpicHold("p", release, epics, nil)[1]
	return d.RenderedStatus(d.Tickets[0])
}

func TestResolveEpicHold_WholeEpicReleasesOnPolicyStamp(t *testing.T) {
	onMerged := EpicRelease{}
	onDone := EpicRelease{OnDone: true}
	onDoneFF := EpicRelease{OnDone: true, AutoFFMerge: true}
	for _, tc := range []struct {
		name              string
		release           EpicRelease
		completed, merged bool
		want              RenderedStatus
	}{
		{"on-merge unstamped", onMerged, false, false, StatusBlocked},
		{"on-merge completed only", onMerged, true, false, StatusBlocked},
		{"on-merge merged", onMerged, true, true, StatusOpen},
		{"on-done unstamped", onDone, false, false, StatusBlocked},
		{"on-done completed", onDone, true, false, StatusOpen},
		{"on-done+ff completed only (root pause)", onDoneFF, true, false, StatusBlocked},
		{"on-done+ff merged without completed", onDoneFF, false, true, StatusBlocked},
		{"on-done+ff completed and merged", onDoneFF, true, true, StatusOpen},
	} {
		if got := dependentStatus(tc.release, wholeEpic(doneTicket(), tc.completed, tc.merged)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestResolveEpicHold_PREpicReleasesOnlyOnMergedAt(t *testing.T) {
	for _, release := range []EpicRelease{{}, {OnDone: true}, {OnDone: true, AutoFFMerge: true}} {
		open := wholeEpic(doneTicket(), true, false)
		open[0].PRNumber = 7
		if got := dependentStatus(release, open); got != StatusBlocked {
			t.Errorf("%+v: open PR: got %v, want blocked", release, got)
		}
		merged := wholeEpic(doneTicket(), true, true)
		merged[0].PRNumber = 7
		if got := dependentStatus(release, merged); got != StatusOpen {
			t.Errorf("%+v: merged PR: got %v, want open", release, got)
		}
	}
}

func TestResolveEpicHold_WholeEpicBlocksWhileATicketIsOpen(t *testing.T) {
	reopened := append(doneTicket(), Ticket{Number: 2, Identifier: "02", Status: "open"})
	for _, release := range []EpicRelease{{}, {OnDone: true}, {OnDone: true, AutoFFMerge: true}} {
		if got := dependentStatus(release, wholeEpic(reopened, true, true)); got != StatusBlocked {
			t.Errorf("%+v: ticket added to a stamped epic: got %v, want blocked", release, got)
		}
	}
}

func TestResolveEpicHold_EmptyEpicBlocksUntilStamped(t *testing.T) {
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
	}, nil)
	for ref, want := range map[string]string{
		"a":    "a",
		"p:a":  "a",
		"p:12": "12",
		"01":   "d/1",
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
	}, nil)
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

func TestResolveEpicHold_ArchivedEpicIsSatisfied(t *testing.T) {
	archived := map[string]bool{"gone": true}
	for _, ref := range []string{"gone", "p:gone"} {
		epics := wholeEpic(doneTicket(), true, true)
		epics[1].BlockedBy = []string{ref}
		d := resolveEpicHold("p", EpicRelease{}, epics, archived)[1]
		if got := d.RenderedStatus(d.Tickets[0]); got != StatusOpen {
			t.Errorf("%q: got %v, want open", ref, got)
		}
	}
	// A typo still errors, and a live epic wins a name clash.
	epics := wholeEpic(nil, false, false)
	epics[1].BlockedBy = []string{"zz"}
	if d := resolveEpicHold("p", EpicRelease{}, epics, archived)[1]; d.RenderedStatus(d.Tickets[0]) != StatusError {
		t.Errorf("typo: got %v, want error", d.RenderedStatus(d.Tickets[0]))
	}
	epics = wholeEpic(nil, false, false)
	if got := resolveEpicHold("p", EpicRelease{}, epics, map[string]bool{"a": true})[1]; got.RenderedStatus(got.Tickets[0]) != StatusBlocked {
		t.Errorf("live epic with archived twin: got %v, want blocked", got.RenderedStatus(got.Tickets[0]))
	}
}

func TestValidateProject_ArchivedBlockerEpic(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".archive/old/ticket.md", "---\ntype: epic\n---\n# old\n")
	write("live/ticket.md", "---\ntype: epic\nblocked_by: [old]\n---\n# live\n")
	write("live/issues/01-a.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# a\n")
	if err := ValidateProject(root); err != nil {
		t.Errorf("archived blocker: %v", err)
	}
	write("live/ticket.md", "---\ntype: epic\nblocked_by: [typo]\n---\n# live\n")
	if err := ValidateProject(root); err == nil || !strings.Contains(err.Error(), CodeUnknownEpic) {
		t.Errorf("typo: err = %v, want unknown-epic", err)
	}
}

func TestResolveEpicHold_UnknownEpicIsError(t *testing.T) {
	epics := wholeEpic(doneTicket(), true, true)
	epics[1].BlockedBy = []string{"zz"}
	if got := dependentStatus(EpicRelease{}, epics); got != StatusError {
		t.Errorf("got %v, want error", got)
	}
}

func TestResolveEpicHold_EpicLevelErrorSkipsTerminalTickets(t *testing.T) {
	epics := []Epic{{Name: "d", BlockedBy: []string{"zz"}, Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}}
	d := resolveEpicHold("p", EpicRelease{}, epics, nil)[0]
	if got := d.Tickets[0].BlockedByErr; got != "" {
		t.Errorf("done ticket: BlockedByErr = %q, want none", got)
	}
	if got := d.RenderedStatus(d.Tickets[1]); got != StatusError {
		t.Errorf("open ticket: got %v, want error", got)
	}
}

func TestValidateRefs_WholeEpicCycleNamesTheEpic(t *testing.T) {
	epics := []Epic{
		{Name: "a", BlockedBy: []string{"d"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}},
		{Name: "d", BlockedBy: []string{"a"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}},
	}
	errs := ValidateRefs("p", epics, nil)
	msg := errors.Join(errs...).Error()
	if !strings.Contains(msg, "a → d → d/01 → d → a") || !strings.Contains(msg, "d → a → a/01 → a → d") {
		t.Errorf("errs = %v, want a cycle chain naming both epics", errs)
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
	write(filepath.Join(dir, "d", "ticket.md"), "---\nstatus: open\nblocked_by: [a]\n---\n")
	write(filepath.Join(dir, "d", "issues", "01-y.md"), "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n")

	status := func() RenderedStatus {
		epics, err := LoadResolved("p", dir)
		if err != nil {
			t.Fatal(err)
		}
		d := epics[1]
		return d.RenderedStatus(d.Tickets[0])
	}
	if got := status(); got != StatusBlocked {
		t.Errorf("default on-merge, completed_at only: got %v, want blocked", got)
	}
	write(filepath.Join(dir, "project.json"), `{"landing":"on-done"}`)
	if got := status(); got != StatusOpen {
		t.Errorf("on-done, completed_at: got %v, want open", got)
	}
}

func TestFlagDanglingBlockers_CrossEpicRefDoesNotHideDoneTickets(t *testing.T) {
	e := Epic{Name: "d", BlockedBy: []string{"p:other"}, Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	e.flagDanglingBlockers()
	if got := e.RenderedStatus(e.Tickets[0]); got != StatusDone {
		t.Errorf("done ticket: got %v, want done", got)
	}
	if got := e.RenderedStatus(e.Tickets[1]); got != StatusError {
		t.Errorf("open ticket: got %v, want error (fails closed)", got)
	}
}
