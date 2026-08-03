package tickets

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// TestMain points queueStateDirFn at a scratch directory for the whole
// package's test binary, so every test that exercises checked-set mutations
// (this file's own tests, and checked_test.go's pre-existing space-press
// tests) writes queue-state.json under a throwaway dir instead of the real
// machine's ~/.config/gx.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gx-tickets-queue-state")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	queueStateDirFn = func() (string, error) { return dir, nil }
	os.Exit(m.Run())
}

func withQueueStateDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	prev := queueStateDirFn
	queueStateDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { queueStateDirFn = prev })
	return tmp
}

func TestLoadQueueStateMissingFileReturnsEmpty(t *testing.T) {
	withQueueStateDir(t)

	got := loadQueueState()
	if len(got) != 0 {
		t.Fatalf("expected empty queue state, got %v", got)
	}
}

func TestLoadQueueStateCorruptFileReturnsEmpty(t *testing.T) {
	tmp := withQueueStateDir(t)
	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue-state.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	got := loadQueueState()
	if len(got) != 0 {
		t.Fatalf("expected empty queue state for corrupt file, got %v", got)
	}
}

func TestSaveQueueStateRoundTrips(t *testing.T) {
	withQueueStateDir(t)

	items := map[string]queueItemStatus{
		"/repo/.scratch/epic/issues/01.md": queueStatusRunning,
		"/repo/.scratch/epic/issues/02.md": queueStatusDone,
	}
	if err := saveQueueState(items); err != nil {
		t.Fatalf("saveQueueState: %v", err)
	}

	got := loadQueueState()
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2: %v", len(got), got)
	}
	if got["/repo/.scratch/epic/issues/01.md"] != queueStatusRunning {
		t.Fatalf("status = %v, want running", got["/repo/.scratch/epic/issues/01.md"])
	}
	if got["/repo/.scratch/epic/issues/02.md"] != queueStatusDone {
		t.Fatalf("status = %v, want done", got["/repo/.scratch/epic/issues/02.md"])
	}
}

func TestModel_CheckingTicketPersistsPendingStatus(t *testing.T) {
	withQueueStateDir(t)
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	ticket := m.epics[0].Tickets[0]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	got := loadQueueState()
	if got[ticket.Path] != queueStatusPending {
		t.Fatalf("persisted status = %v, want pending", got[ticket.Path])
	}
}

func TestModel_UncheckingTicketRemovesPersistedStatus(t *testing.T) {
	withQueueStateDir(t)
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	ticket := m.epics[0].Tickets[0]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	got := loadQueueState()
	if _, ok := got[ticket.Path]; ok {
		t.Fatalf("expected ticket removed from persisted state after uncheck, got %v", got)
	}
}

func TestModel_RestoresCheckedSetAndStatusOnStartup(t *testing.T) {
	withQueueStateDir(t)
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	ticket := m.epics[0].Tickets[0]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	m.setQueueItemStatus(ticket.Path, queueStatusDone)

	// Simulate a restart: a fresh Model built against the same worktree/config dir.
	restarted := NewModel(root, ui.Settings{}, keys.New(nil))
	restarted = deliverLoad(t, restarted)
	updated, _ = restarted.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	restarted = updated.(Model)

	if !restarted.isChecked(ticket.Path) {
		t.Fatalf("expected ticket still checked after restart")
	}
	if restarted.queueStatus[ticket.Path] != queueStatusDone {
		t.Fatalf("status after restart = %v, want done", restarted.queueStatus[ticket.Path])
	}
}
