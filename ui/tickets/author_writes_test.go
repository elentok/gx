package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// writeStoreTicket writes name under store's gx:alpha epic and returns its path.
func writeStoreTicket(t *testing.T, store, name, raw string) string {
	t.Helper()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func readStatus(t *testing.T, file string) string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "status: "); ok {
			return v
		}
	}
	return ""
}

// applyStatus runs the status write and returns the toast it produced, if any.
func applyStatus(t *testing.T, m Model, status schema.Status) string {
	t.Helper()
	_, cmd := m.applyStatusChange(status)
	if cmd == nil {
		t.Fatal("status change issued nothing")
	}
	if msg, ok := cmd().(notify.NotifyMsg); ok {
		return msg.Message
	}
	return ""
}

func TestAuthorWrites_OpenAndDraftWriteTheStoreFileAndPingTheServer(t *testing.T) {
	var calls []string
	m, file := serverModelWithStoreTicket(t, fakeServerAPI{calls: &calls}, false)
	// The served File points elsewhere: the write must follow the address.
	m.epics[0].Tickets[0].File = filepath.Join(t.TempDir(), "stale.md")

	if toast := applyStatus(t, m, schema.StatusOpen); toast != "" || readStatus(t, file) != "open" {
		t.Fatalf("open: toast %q, status %q", toast, readStatus(t, file))
	}
	if toast := applyStatus(t, m, schema.StatusDraft); toast != "" || readStatus(t, file) != "draft" {
		t.Fatalf("draft: toast %q, status %q", toast, readStatus(t, file))
	}
	if want := []string{"changed gx:alpha/20", "changed gx:alpha/20"}; !slices.Equal(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestAuthorWrites_WorkWithoutAServer(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, fakeServerAPI{}, false)
	m.serverAPI = nil
	if toast := applyStatus(t, m, schema.StatusOpen); toast != "" || readStatus(t, file) != "open" {
		t.Fatalf("toast %q, status %q", toast, readStatus(t, file))
	}
}

func TestAuthorWrites_DoneOnlyOnMapEpics(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, fakeServerAPI{}, false)
	if toast := applyStatus(t, m, schema.StatusDone); toast != doneIsSetByServer || readStatus(t, file) != "draft" {
		t.Errorf("loop epic: toast %q, status %q", toast, readStatus(t, file))
	}

	m, file = serverModelWithStoreTicket(t, fakeServerAPI{}, true)
	if toast := applyStatus(t, m, schema.StatusDone); toast != "" || readStatus(t, file) != "done" {
		t.Errorf("map epic: toast %q, status %q", toast, readStatus(t, file))
	}
}

func TestAuthorWrites_DoneRefusedWhileBlocked(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, fakeServerAPI{}, true)
	m.epics[0].Tickets = append(m.epics[0].Tickets, gxtickets.Ticket{Identifier: "19", Number: 19, Path: "gx:alpha/19", Status: "open"})
	m.epics[0].Tickets[0].BlockedBy = []string{"19"}
	if toast := applyStatus(t, m, schema.StatusDone); !strings.Contains(toast, "unresolved blocked_by") || readStatus(t, file) != "draft" {
		t.Errorf("toast %q, status %q", toast, readStatus(t, file))
	}
}

func TestAuthorWrites_OpenRefusedOnAnEmptyBody(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, fakeServerAPI{}, false)
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: draft\ntype: research\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if toast := applyStatus(t, m, schema.StatusOpen); !strings.Contains(toast, "empty body") || readStatus(t, file) != "draft" {
		t.Errorf("toast %q, status %q", toast, readStatus(t, file))
	}
}

func TestQueueModel_DeleteWithoutAServerWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-root.md", "Status: open\n\nBody.\n")
	file := ticketPath(root, "alpha", "01-root.md")
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), map[string]bool{file: true}))
	m = selectFirstQueueTicketRow(t, m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if next.(QueueModel).confirm.IsOpen {
		t.Fatal("x opened a confirm with no server")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Message != serverNotRunning {
		t.Fatalf("result = %+v, want the %q toast", msg, serverNotRunning)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("ticket file gone: %v", err)
	}
}

func TestQueueModel_EditTargetIsTheAddressFile(t *testing.T) {
	store := t.TempDir()
	file := writeStoreTicket(t, store, "01-first.md", "---\nid: \"01\"\nstatus: open\n---\n\nBody.\n")
	m := newMaintenanceQueue(t, fakeServerAPI{})
	m.settings.TicketStore = store
	m.epics[0].Tickets[0].File = filepath.Join(t.TempDir(), "stale.md")

	if got, ok := m.selectedEditTarget(); !ok || got != file {
		t.Errorf("edit target = %q, %v, want %q", got, ok, file)
	}
}
