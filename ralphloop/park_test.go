package ralphloop

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets/schema"
)

// Seam B: a ticket parked by the loop's catch-all writes the ticket, appends
// exactly one needs-repair event with a kind and address, and notifies.
func TestRun_CatchAll_ParksThroughOnePath(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps()
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, wtPath, branch, base string) error {
		if strings.Contains(wtPath, "my-epic-item-01") {
			return errors.New("simulated git hiccup")
		}
		return origAddWorktree(repoDir, wtPath, branch, base)
	}
	d.ParkTimer = func(dur time.Duration) <-chan time.Time {
		// End the run: a human closes the parked ticket.
		if err := SetStatus(path, "done"); err != nil {
			t.Errorf("SetStatus: %v", err)
		}
		return readyTimer(dur)
	}
	sink := &recordingSink{}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	evs, _, err := ReadEvents(scratchDir, "my-epic")
	if err != nil {
		t.Fatal(err)
	}
	var repairs []Event
	for _, ev := range evs {
		if ev.Type == string(events.NeedsRepair) {
			repairs = append(repairs, ev)
		}
	}
	if len(repairs) != 1 {
		t.Fatalf("needs-repair events = %d, want 1 (%v)", len(repairs), evs)
	}
	ev := repairs[0]
	if ev.Kind != string(events.IterationError) || ev.Address != "01" || !strings.Contains(ev.Reason, "simulated git hiccup") {
		t.Errorf("event = %+v, want kind iteration-error, address 01, reason naming the error", ev)
	}
}

func TestPark_WritesTicketEventAndNotifies(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	sink := &recordingSink{}

	park(sink, parkRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.NeedsRepair, Kind: events.IterationError, Reason: "boom",
		Repair: schema.NeedsRepairState{Label: "l", Branch: "b", Worktree: "w"},
	})

	if got := mustParse(t, path); got.Status != schema.StatusNeedsRepair {
		t.Errorf("Status = %q, want needs-repair", got.Status)
	}
	evs, _, _ := ReadEvents(scratchDir, "my-epic")
	if len(evs) != 1 || evs[0].Kind != "iteration-error" || evs[0].Reason != "boom" {
		t.Errorf("events = %+v, want one iteration-error event", evs)
	}
	if calls := sink.snapshot(); len(calls) != 1 || calls[0] != "TicketNeedsHuman" {
		t.Errorf("sink calls = %v, want one TicketNeedsHuman", calls)
	}
}

// Seam B: finish-time needs-answer parks (zero-commit, self-reported) each write
// the ticket's park_kind and append one needs-answer event with that kind.
func TestPark_NeedsAnswerKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []events.Kind{events.ZeroCommit, events.SelfReported} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
			})
			path := ticketPath(scratchDir, "my-epic", "01-a.md")
			sink := &recordingSink{}

			if _, err := park(sink, parkRequest{
				ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
				Type: events.NeedsAnswer, Kind: kind, Reason: "why", Event: Event{Pane: "p1"},
			}); err != nil {
				t.Fatal(err)
			}

			got := mustParse(t, path)
			if got.Status != schema.StatusNeedsAnswer || got.ParkKind != schema.ParkKind(kind) {
				t.Errorf("ticket = (%q, %q), want needs-answer/%s", got.Status, got.ParkKind, kind)
			}
			evs, _, _ := ReadEvents(scratchDir, "my-epic")
			if len(evs) != 1 || evs[0].Type != "needs-answer" || evs[0].Kind != string(kind) || evs[0].Pane != "p1" {
				t.Errorf("events = %+v, want one needs-answer event kind %s", evs, kind)
			}
			if calls := sink.snapshot(); len(calls) != 1 || calls[0] != "TicketNeedsHuman" {
				t.Errorf("sink calls = %v, want one TicketNeedsHuman", calls)
			}
		})
	}
}

// Seam B: a needs-repair park stamps the event's kind into frontmatter as
// park_kind; a claim (resume) clears it while the event stays in the log.
func TestPark_NeedsRepairStampsParkKindClearedOnClaim(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")

	park(&recordingSink{}, parkRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.NeedsRepair, Kind: events.BudgetKilled, Reason: "boom",
	})
	if got := mustParse(t, path); got.ParkKind != schema.ParkKind(events.BudgetKilled) {
		t.Fatalf("ParkKind = %q, want budget-killed", got.ParkKind)
	}

	if err := Claim(path); err != nil {
		t.Fatal(err)
	}
	if got := mustParse(t, path); got.ParkKind != "" {
		t.Errorf("ParkKind after claim = %q, want cleared", got.ParkKind)
	}
	evs, _, _ := ReadEvents(scratchDir, "my-epic")
	if len(evs) != 1 || evs[0].Kind != "budget-killed" {
		t.Errorf("events = %+v, want the park event kept", evs)
	}
}
