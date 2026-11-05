package ticketops

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestPause_WritesTicketEventAndNotifies(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
	sink := &recordingSink{}

	PauseTicket(sink, PauseRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.Paused, Kind: events.IterationError, Reason: "boom",
		Repair: schema.FaultState{Label: "l", Branch: "b", Worktree: "w"},
	})

	assertPaused(t, fixture.MustParse(t, path), schema.PauseClassRepair, schema.PauseIterationError)
	evs, _, _ := runlog.ReadEvents(scratchDir, "my-epic")
	if len(evs) != 1 || evs[0].Kind != "iteration-error" || evs[0].Reason != "boom" {
		t.Errorf("events = %+v, want one iteration-error event", evs)
	}
	if calls := sink.snapshot(); len(calls) != 1 || calls[0] != "TicketNeedsHuman" {
		t.Errorf("sink calls = %v, want one TicketNeedsHuman", calls)
	}
}

// A pause write on a done ticket is refused under the ticket lock: nothing is
// written, no event is appended, no one is notified, and the refusal is logged.
// Not parallel: it captures the process-wide log output.
func TestPause_RefusesDoneTicket(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	// One kind per pause writer: paused (repair), stubbed and bare paused (answer).
	for _, kind := range []events.Kind{events.IterationError, events.BlockedPane, events.ZeroCommit} {
		t.Run(string(kind), func(t *testing.T) {
			logged.Reset()
			content := "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n# A\n"
			scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{"01-a.md": content})
			path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
			sink := &recordingSink{}

			_, err := PauseTicket(sink, PauseRequest{
				ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
				Type: events.Paused, Kind: kind, Reason: "late", Repair: schema.FaultState{Label: "l"},
			})

			if !errors.Is(err, ErrPauseDone) {
				t.Fatalf("err = %v, want ErrPauseDone", err)
			}
			if raw, _ := os.ReadFile(path); string(raw) != content {
				t.Errorf("ticket rewritten:\n%s", raw)
			}
			if evs, _, _ := runlog.ReadEvents(scratchDir, "my-epic"); len(evs) != 0 {
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

// Seam B: finish-time paused (answer) pauses (zero-commit, self-reported) each write
// the ticket's pause_kind and append one paused (answer) event with that kind.
func TestPause_AskingKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []events.Kind{events.ZeroCommit, events.SelfReported} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
			})
			path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
			sink := &recordingSink{}

			if _, err := PauseTicket(sink, PauseRequest{
				ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
				Type: events.Paused, Kind: kind, Reason: "why", Event: runlog.Event{Pane: "p1"},
			}); err != nil {
				t.Fatal(err)
			}

			got := fixture.MustParse(t, path)
			assertPaused(t, got, schema.PauseClassAnswer, schema.ParsePauseReason(string(kind)))
			evs, _, _ := runlog.ReadEvents(scratchDir, "my-epic")
			if len(evs) != 1 || evs[0].Type != "paused" || evs[0].Kind != string(kind) || evs[0].Pane != "p1" {
				t.Errorf("events = %+v, want one paused event kind %s", evs, kind)
			}
			if calls := sink.snapshot(); len(calls) != 1 || calls[0] != "TicketNeedsHuman" {
				t.Errorf("sink calls = %v, want one TicketNeedsHuman", calls)
			}
		})
	}
}

func TestPause_LogsGxQuestionPerKind(t *testing.T) {
	t.Parallel()
	for kind, summary := range gxQuestions {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
			})
			path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
			if _, err := PauseTicket(&recordingSink{}, PauseRequest{
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

func TestPause_RepairKindOpensRepairRoundWithNoQuestion(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
	if _, err := PauseTicket(&recordingSink{}, PauseRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.Paused, Kind: events.BudgetKilled, Reason: "why",
	}); err != nil {
		t.Fatal(err)
	}
	evs, _ := tickets.ReadLog(path)
	if len(evs) != 1 || evs[0].Type != tickets.EventPauseRepair || evs[0].Summary != "" || evs[0].Text != "" {
		t.Fatalf("log = %+v, want one question-less pause.repair", evs)
	}
	if tickets.OpenQuestion(evs) != nil {
		t.Error("a repair round must not read as an open question")
	}
}

// Seam B: a paused (repair) pause stamps the event's kind into frontmatter as
// pause_kind; a claim (resume) clears it while the event stays in the log.
func TestPause_RepairStampsPauseKindClearedOnClaim(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")

	PauseTicket(&recordingSink{}, PauseRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.Paused, Kind: events.BudgetKilled, Reason: "boom",
	})
	assertPaused(t, fixture.MustParse(t, path), schema.PauseClassRepair, schema.PauseBudgetKilled)

	if err := Claim(path); err != nil {
		t.Fatal(err)
	}
	if got := fixture.MustParse(t, path); got.PauseReason != "" || got.Status == schema.StatusPaused {
		t.Errorf("after claim = (%q, %q), want pause reason cleared", got.Status, got.PauseReason)
	}
	evs, _, _ := runlog.ReadEvents(scratchDir, "my-epic")
	if len(evs) != 1 || evs[0].Kind != "budget-killed" {
		t.Errorf("events = %+v, want the pause event kept", evs)
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

// A blocked pane's reason can run past the log's summary limit (it names the
// prompt and every way to answer); the question must still open, with the whole
// reason in its text.
func TestPause_BlockedPaneLongReasonStillOpensAQuestion(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n"})
	path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
	reason := strings.Repeat("blocked on a prompt gx did not send; ", 6)

	got, err := PauseTicket(&recordingSink{}, PauseRequest{
		ScratchDir: scratchDir, EpicName: "my-epic", Ticket: "01", Path: path,
		Type: events.Paused, Kind: events.BlockedPane, Reason: reason,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != reason {
		t.Errorf("reason = %q, want it unchanged (no failed-logging suffix)", got)
	}
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	q := tickets.OpenQuestion(evs)
	if q == nil || !strings.Contains(q.Text, reason) {
		t.Errorf("open question = %+v, want one carrying the whole reason", q)
	}
}

// Seam 1: Pause derives the event type from the kind, and a paused (answer) pause
// never writes a Handoff section into the markdown: the question
// lives in the log.
func TestPause_DerivesTypeFromKindAndKeepsConversationOutOfMarkdown(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind     events.Kind
		status   schema.Status
		question bool // the log carries an open question afterwards
	}{
		// Answer class (gx receives the reply), but written as a repair round: no question.
		{events.Manual, schema.StatusLegacyAnswer, false},
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
			scratchDir := fixture.WriteEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
			})
			path := fixture.TicketPath(scratchDir, "my-epic", "01-a.md")
			if err := Pause(scratchDir, "my-epic", "01", path, c.kind, "the-reason", runlog.Event{}); err != nil {
				t.Fatal(err)
			}
			got := fixture.MustParse(t, path)
			assertPaused(t, got, legacyClass(c.status), schema.ParsePauseReason(string(c.kind)))
			if data, _ := os.ReadFile(path); strings.Contains(string(data), "## Handoff") {
				t.Errorf("pause wrote a conversation section into the .md:\n%s", data)
			}
			qevs, err := tickets.ReadLog(path)
			if err != nil {
				t.Fatal(err)
			}
			if open := tickets.OpenQuestion(qevs); (open != nil) != c.question {
				t.Errorf("open question = %+v, want present=%v", open, c.question)
			}
			evs, _, _ := runlog.ReadEvents(scratchDir, "my-epic")
			if len(evs) != 1 || evs[0].Type != "paused" || evs[0].Kind != string(c.kind) {
				t.Errorf("events = %+v, want one paused event kind %s", evs, c.kind)
			}
		})
	}
}
