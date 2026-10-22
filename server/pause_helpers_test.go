package server_test

import (
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

// legacyClass is the pause class of an old needs-answer / needs-repair label,
// or "" for any other status.
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
