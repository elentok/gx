package ticketops

import (
	"sync"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets/schema"
)

// recordingSink records the names of the EventSink calls a pause makes. It is
// the only call PauseTicket fires, so it overrides TicketNeedsHuman alone.
type recordingSink struct {
	runlog.NoopEventSink
	mu    sync.Mutex
	calls []string
}

func (s *recordingSink) TicketNeedsHuman(string, string, string, events.Kind, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "TicketNeedsHuman")
}

func (s *recordingSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// legacyClass is the pause class of an old paused (answer) / paused (repair) label.
func legacyClass(s schema.Status) schema.PauseClass {
	return schema.Ticket{Status: s}.PausedClass()
}

// assertPaused checks a parsed ticket is paused with the given class and, when
// reason is non-empty, the given reason.
func assertPaused(t *testing.T, tk schema.Ticket, class schema.PauseClass, reason schema.PauseReason) {
	t.Helper()
	if tk.Status != schema.StatusPaused {
		t.Errorf("Status = %q, want %q", tk.Status, schema.StatusPaused)
		return
	}
	if class != "" && tk.PausedClass() != class {
		t.Errorf("PausedClass = %q, want %q", tk.PausedClass(), class)
	}
	if reason != "" && tk.PauseReason != reason {
		t.Errorf("PauseReason = %q, want %q", tk.PauseReason, reason)
	}
}
