package ralphloop

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
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
		if ev.Type == string(events.Paused) {
			repairs = append(repairs, ev)
		}
	}
	if len(repairs) != 1 {
		t.Fatalf("needs-repair events = %d, want 1 (%v)", len(repairs), evs)
	}
	ev := repairs[0]
	if ev.Kind != string(events.IterationError) || ev.Address != canonicalAddr(scratchDir, "my-epic", "01") || !strings.Contains(ev.Reason, "simulated git hiccup") {
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
		Type: events.Paused, Kind: events.IterationError, Reason: "boom",
		Repair: schema.FaultState{Label: "l", Branch: "b", Worktree: "w"},
	})

	assertPaused(t, mustParse(t, path), schema.PauseClassRepair, schema.PauseIterationError)
	evs, _, _ := ReadEvents(scratchDir, "my-epic")
	if len(evs) != 1 || evs[0].Kind != "iteration-error" || evs[0].Reason != "boom" {
		t.Errorf("events = %+v, want one iteration-error event", evs)
	}
	if calls := sink.snapshot(); len(calls) != 1 || calls[0] != "TicketNeedsHuman" {
		t.Errorf("sink calls = %v, want one TicketNeedsHuman", calls)
	}
}

// A park write on a done ticket is refused under the ticket lock: nothing is
// written, no event is appended, no one is notified, and the refusal is logged.
// Not parallel: it captures the process-wide log output.
func TestPark_RefusesDoneTicket(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	// One kind per park writer: needs-repair, stubbed and bare needs-answer.
	for _, kind := range []events.Kind{events.IterationError, events.BlockedPane, events.ZeroCommit} {
		t.Run(string(kind), func(t *testing.T) {
			logged.Reset()
			content := "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n# A\n"
			scratchDir := writeEpic(t, "my-epic", map[string]string{"01-a.md": content})
			path := ticketPath(scratchDir, "my-epic", "01-a.md")
			sink := &recordingSink{}

			_, err := park(sink, parkRequest{
				ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
				Type: events.Paused, Kind: kind, Reason: "late", Repair: schema.FaultState{Label: "l"},
			})

			if !errors.Is(err, ErrParkDone) {
				t.Fatalf("err = %v, want ErrParkDone", err)
			}
			if raw, _ := os.ReadFile(path); string(raw) != content {
				t.Errorf("ticket rewritten:\n%s", raw)
			}
			if evs, _, _ := ReadEvents(scratchDir, "my-epic"); len(evs) != 0 {
				t.Errorf("events = %+v, want none", evs)
			}
			if calls := sink.snapshot(); len(calls) != 0 {
				t.Errorf("sink calls = %v, want none", calls)
			}
			if !strings.Contains(logged.String(), "01") || !strings.Contains(logged.String(), "done") {
				t.Errorf("log = %q, want a refusal naming the ticket", logged.String())
			}
		})
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
				Type: events.Paused, Kind: kind, Reason: "why", Event: Event{Pane: "p1"},
			}); err != nil {
				t.Fatal(err)
			}

			got := mustParse(t, path)
			assertPaused(t, got, schema.PauseClassAnswer, schema.ParsePauseReason(string(kind)))
			evs, _, _ := ReadEvents(scratchDir, "my-epic")
			if len(evs) != 1 || evs[0].Type != "paused" || evs[0].Kind != string(kind) || evs[0].Pane != "p1" {
				t.Errorf("events = %+v, want one needs-answer event kind %s", evs, kind)
			}
			if calls := sink.snapshot(); len(calls) != 1 || calls[0] != "TicketNeedsHuman" {
				t.Errorf("sink calls = %v, want one TicketNeedsHuman", calls)
			}
		})
	}
}

func TestPark_LogsGxQuestionPerKind(t *testing.T) {
	t.Parallel()
	for kind, summary := range gxQuestions {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
			})
			path := ticketPath(scratchDir, "my-epic", "01-a.md")
			if _, err := park(&recordingSink{}, parkRequest{
				ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
				Type: events.Paused, Kind: kind, Reason: "why",
			}); err != nil {
				t.Fatal(err)
			}
			evs, err := tickets.ReadLog(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(evs) != 1 || evs[0].By != "gx" || evs[0].PauseKind != string(kind) || evs[0].Summary != summary {
				t.Errorf("log = %+v, want one by-gx %s question %q", evs, kind, summary)
			}
		})
	}
}

func TestPark_SelfReportedLogsNoQuestion(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	if _, err := park(&recordingSink{}, parkRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.Paused, Kind: events.SelfReported, Reason: "why",
	}); err != nil {
		t.Fatal(err)
	}
	if evs, _ := tickets.ReadLog(path); len(evs) != 0 {
		t.Errorf("log = %+v, want empty", evs)
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
		Type: events.Paused, Kind: events.BudgetKilled, Reason: "boom",
	})
	assertPaused(t, mustParse(t, path), schema.PauseClassRepair, schema.PauseBudgetKilled)

	if err := Claim(path); err != nil {
		t.Fatal(err)
	}
	if got := mustParse(t, path); got.PauseReason != "" || got.Status == schema.StatusPaused {
		t.Errorf("after claim = (%q, %q), want pause reason cleared", got.Status, got.PauseReason)
	}
	evs, _, _ := ReadEvents(scratchDir, "my-epic")
	if len(evs) != 1 || evs[0].Kind != "budget-killed" {
		t.Errorf("events = %+v, want the park event kept", evs)
	}
}

// openQuestionSummary is the summary of the ticket's open log question, "" when none.
func openQuestionSummary(t *testing.T, path string) string {
	t.Helper()
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if q := tickets.OpenQuestion(evs); q != nil {
		return q.Summary
	}
	return ""
}

// Seam 1: Park derives the event type from the kind, and a needs-answer park
// never writes a Needs Answer/Handoff section into the markdown: the question
// lives in the log.
func TestPark_DerivesTypeFromKindAndKeepsConversationOutOfMarkdown(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind     events.Kind
		status   schema.Status
		question bool // the log carries an open question afterwards
	}{
		{events.Manual, schema.StatusLegacyRepair, false},
		{events.AmbiguousBase, schema.StatusLegacyAnswer, true},
		{events.BlockedPane, schema.StatusLegacyAnswer, true},
		{events.StrandedCommits, schema.StatusLegacyAnswer, true},
		{events.BackgroundTaskGate, schema.StatusLegacyAnswer, true},
		{events.ZeroCommit, schema.StatusLegacyAnswer, true},
		{events.BudgetKilled, schema.StatusLegacyRepair, false},
		{events.HandleMismatch, schema.StatusLegacyRepair, false},
		{events.AmbiguousMerge, schema.StatusLegacyRepair, false},
		{events.IterationError, schema.StatusLegacyRepair, false},
	}
	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
			})
			path := ticketPath(scratchDir, "my-epic", "01-a.md")
			if err := Park(scratchDir, "my-epic", "01", path, c.kind, "the-reason", Event{}); err != nil {
				t.Fatal(err)
			}
			got := mustParse(t, path)
			assertPaused(t, got, legacyClass(c.status), schema.ParsePauseReason(string(c.kind)))
			if data, _ := os.ReadFile(path); strings.Contains(string(data), "## Needs Answer") || strings.Contains(string(data), "## Handoff") {
				t.Errorf("park wrote a conversation section into the .md:\n%s", data)
			}
			qevs, err := tickets.ReadLog(path)
			if err != nil {
				t.Fatal(err)
			}
			if open := tickets.OpenQuestion(qevs); (open != nil) != c.question {
				t.Errorf("open question = %+v, want present=%v", open, c.question)
			}
			evs, _, _ := ReadEvents(scratchDir, "my-epic")
			if len(evs) != 1 || evs[0].Type != "paused" || evs[0].Kind != string(c.kind) {
				t.Errorf("events = %+v, want one paused event kind %s", evs, c.kind)
			}
		})
	}
}
