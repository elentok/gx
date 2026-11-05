package servertest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// WaitStatus waits for the ticket at ticketPath to reach want.
func WaitStatus(t *testing.T, ticketPath string, want schema.Status) {
	t.Helper()
	var tk schema.Ticket
	var parseErr error
	if !Poll(30*time.Second, func() bool {
		// A read can land between a rewrite's truncate and its content under load:
		// a ticket that does not parse yet is not at the wanted status yet.
		if tk, parseErr = schema.ParseTicket(ticketPath); parseErr != nil {
			return false
		}
		// A legacy asking / stuck want means paused in that class.
		if class := (schema.Ticket{Status: want}).PausedClass(); class != "" {
			return tk.Status == schema.StatusPaused && tk.PausedClass() == class
		}
		return tk.Status == want
	}) {
		body, _ := os.ReadFile(ticketPath)
		t.Fatalf("status = %q, want %q (last parse error: %v):\n%s", tk.Status, want, parseErr, body)
	}
}

// AssertPauseRecordsRun checks the pause event names the run's runner, label and
// session: late-finish adoption reads them after the pause drops runs.json.
func AssertPauseRecordsRun(t *testing.T, store string, pause events.Type, runner string, session agentrunner.Session) {
	t.Helper()
	// The pause writes the ticket's status before its run-log event.
	var logged []runlog.Event
	var i int
	if !Poll(30*time.Second, func() bool {
		logged, _, _ = runlog.ReadEvents(filepath.Join(store, "proj"), "epic-a")
		i = slices.IndexFunc(logged, func(ev runlog.Event) bool { return ev.Type == string(pause) })
		return i >= 0
	}) {
		t.Fatalf("no %s event in the run log: %v", pause, logged)
	}
	got := logged[i]
	sessionOK := got.AgentSession == session.SessionID || (session.SessionID == "" && got.AgentSession != "")
	if got.Runner != runner || got.Label != session.Label || !sessionOK {
		t.Errorf("pause event runner/label/session = %q/%q/%q, want %q/%q/%q",
			got.Runner, got.Label, got.AgentSession, runner, session.Label, session.SessionID)
	}
}

// AppendPauseRound logs a zero-commit pause question, and its cli close when closed.
func AppendPauseRound(t *testing.T, ticketPath string, closed bool) {
	t.Helper()
	q, err := tickets.AppendLog(ticketPath, tickets.LogEvent{Type: tickets.EventPauseQuestion, PauseKind: "zero-commit"})
	if err != nil {
		t.Fatal(err)
	}
	if !closed {
		return
	}
	if _, err := tickets.AppendLog(ticketPath, tickets.LogEvent{Type: tickets.EventPauseClosed, Question: q.Seq, Source: tickets.SourceCLI}); err != nil {
		t.Fatal(err)
	}
}
