package fakes

import (
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

// ParseRawTicket parses ticket file text (through the legacy shim).
func ParseRawTicket(t *testing.T, raw string) schema.Ticket {
	t.Helper()
	tk, err := schema.ParseTicketFromRaw(raw, "ticket.md")
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v\n%s", err, raw)
	}
	return tk
}

// RawIsPaused reports whether ticket file text is a paused ticket of the
// given class. An empty class matches either class.
func RawIsPaused(t *testing.T, raw string, class schema.PauseClass) bool {
	t.Helper()
	tk := ParseRawTicket(t, raw)
	return tk.Status == schema.StatusPaused && (class == "" || tk.PausedClass() == class)
}

// AssertPaused checks a parsed ticket is paused with the given class and, when
// reason is non-empty, the given reason.
func AssertPaused(t *testing.T, tk schema.Ticket, class schema.PauseClass, reason schema.PauseReason) {
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

// AssertRawPaused is AssertPaused for ticket file text.
func AssertRawPaused(t *testing.T, raw string, class schema.PauseClass, reason schema.PauseReason) {
	t.Helper()
	tk := ParseRawTicket(t, raw)
	AssertPaused(t, tk, class, reason)
	if t.Failed() {
		t.Logf("ticket:\n%s", raw)
	}
}
