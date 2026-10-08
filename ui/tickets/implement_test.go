package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// Without a server there is no queue: "r" only tells the user to start one,
// and leaves the checked selection alone.
func TestModel_ReplaceQueueKeyWithoutServerNotifies(t *testing.T) {
	t.Parallel()
	m := Model{
		checked:    map[string]bool{"/alpha/01.md": true},
		checkOrder: map[string]uint64{"/alpha/01.md": 1},
	}

	updated, cmd := m.handleReplaceQueueKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	notifyMsg, ok := cmd().(notify.NotifyMsg)
	if !ok || notifyMsg.Message != "start the server to queue tickets" {
		t.Fatalf("expected the start-the-server info notification, got %#v", notifyMsg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation modal without a server")
	}
	if !m.checked["/alpha/01.md"] {
		t.Fatalf("checked set = %v, want it untouched", m.checked)
	}
}

// TestModel_AddToQueueKeyNotRunningEpic covers ticket 10: "a" against an
// epic under the cursor that has no live run is a no-op with an info
// notification, and never opens the confirmation modal.
func TestModel_AddToQueueKeyNotRunningEpic(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
	}}
	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{"/alpha/01.md": true}}

	updated, cmd := m.handleAddToQueueKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || !strings.Contains(notifyMsg.Message, "isn't running") {
		t.Fatalf("cmd() = %#v, want an \"isn't running\" notification", msg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation for a non-running epic")
	}
}
