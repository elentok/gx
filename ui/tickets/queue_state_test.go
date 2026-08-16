package tickets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestQueueStoreRoundTripAndSnapshotIsolation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "queue.json")
	store := loadQueueStoreAt(path)
	for i, status := range []queueItemStatus{queueStatusPending, queueStatusRunning, queueStatusDone, queueStatusErrored} {
		item := string(rune('a' + i))
		if err := store.Check(item); err != nil {
			t.Fatal(err)
		}
		if err := store.SetStatus(item, status); err != nil {
			t.Fatal(err)
		}
	}

	reloaded := loadQueueStoreAt(path)
	snapshot := reloaded.Snapshot()
	if len(snapshot.Status) != 4 || snapshot.Status["d"] != queueStatusErrored {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	snapshot.Status["a"] = queueStatusDone
	delete(snapshot.Checked, "b")
	again := reloaded.Snapshot()
	if again.Status["a"] != queueStatusPending || !again.Checked["b"] {
		t.Fatalf("caller mutated store: %#v", again)
	}
}

func TestQueueStoreConcurrentSnapshots(t *testing.T) {
	t.Parallel()
	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check("ticket"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = store.Snapshot()
			}
		}()
	}
	wg.Wait()
}

func TestQueueStoreFailedWriteDoesNotPublishMutation(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	store := loadQueueStoreAt(path)
	if err := store.Check("ticket"); err == nil {
		t.Fatal("expected write failure")
	}
	if store.Snapshot().Checked["ticket"] {
		t.Fatal("failed mutation became visible")
	}
}

func TestQueueStoreSetCheckedPreservesUnrelatedEntries(t *testing.T) {
	t.Parallel()
	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check("keep"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus("keep", queueStatusRunning); err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()

	if err := store.SetChecked([]string{"ticket", "blocker"}, true); err != nil {
		t.Fatal(err)
	}
	afterAdd := store.Snapshot()
	if afterAdd.Status["keep"] != queueStatusRunning || afterAdd.Order["keep"] != before.Order["keep"] {
		t.Fatalf("unrelated entry changed: before=%#v after=%#v", before, afterAdd)
	}
	if afterAdd.Status["ticket"] != queueStatusPending || afterAdd.Status["blocker"] != queueStatusPending {
		t.Fatalf("batch additions = %#v", afterAdd)
	}
	if afterAdd.Order["ticket"] >= afterAdd.Order["blocker"] {
		t.Fatalf("batch chronology = %#v", afterAdd.Order)
	}

	if err := store.SetChecked([]string{"ticket", "blocker"}, false); err != nil {
		t.Fatal(err)
	}
	afterRemove := store.Snapshot()
	if len(afterRemove.Checked) != 1 || !afterRemove.Checked["keep"] || afterRemove.Status["keep"] != queueStatusRunning {
		t.Fatalf("batch removal dropped unrelated entry: %#v", afterRemove)
	}
}

func TestQueueStoreSetCheckedFailedWritePublishesNothing(t *testing.T) {
	t.Parallel()
	store := loadQueueStoreAt(t.TempDir())
	if err := store.SetChecked([]string{"ticket", "blocker"}, true); err == nil {
		t.Fatal("expected write failure")
	}
	if snapshot := store.Snapshot(); len(snapshot.Checked) != 0 {
		t.Fatalf("failed batch became visible: %#v", snapshot)
	}
}

func TestQueueStoreIncompleteStateFallsBackWhole(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(path, []byte(`{"items":{"ticket":"running"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if snapshot := loadQueueStoreAt(path).Snapshot(); len(snapshot.Checked) != 0 || len(snapshot.Order) != 0 {
		t.Fatalf("partially loaded state: %#v", snapshot)
	}
}

// TestQueueStoreTicketCheckedIsIndependentOfQueued exercises ticket 14's
// decoupled API: SetTicketChecked/IsTicketChecked never touch queue
// membership, and SetQueued/EnqueueAndClearChecked never touch the
// independent checked set except where clearedPaths says to.
func TestQueueStoreTicketCheckedIsIndependentOfQueued(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "queue.json")
	store := loadQueueStoreAt(path)

	if err := store.SetTicketChecked([]string{"a", "b"}, true); err != nil {
		t.Fatal(err)
	}
	if !store.IsTicketChecked("a") || store.IsQueued("a") {
		t.Fatalf("checking a ticket must not queue it: checked=%v queued=%v", store.IsTicketChecked("a"), store.IsQueued("a"))
	}

	if err := store.SetQueued([]string{"a"}, true); err != nil {
		t.Fatal(err)
	}
	if !store.IsQueued("a") || !store.IsTicketChecked("a") || !store.IsTicketChecked("b") {
		t.Fatalf("queueing a ticket must not clear the independent checked set")
	}

	reloaded := loadQueueStoreAt(path)
	snapshot := reloaded.Snapshot()
	if !snapshot.TicketChecked["a"] || !snapshot.TicketChecked["b"] || !snapshot.Checked["a"] {
		t.Fatalf("independent checked set did not round-trip: %#v", snapshot)
	}
}

// TestQueueStoreEnqueueAndClearChecked exercises the atomic transfer method
// ticket 10's replaceQueuedSelection is designed to use: it replaces queue
// membership wholesale and clears exactly clearedPaths from the checked set,
// leaving any other checked entry untouched.
func TestQueueStoreEnqueueAndClearChecked(t *testing.T) {
	t.Parallel()
	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.SetTicketChecked([]string{"a", "b", "keep-checked"}, true); err != nil {
		t.Fatal(err)
	}

	queued := map[string]queueItemStatus{"a": queueStatusPending, "b": queueStatusPending}
	order := map[string]uint64{"a": 1, "b": 2}
	if err := store.EnqueueAndClearChecked(queued, order, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	if snapshot.Status["a"] != queueStatusPending || snapshot.Status["b"] != queueStatusPending {
		t.Fatalf("enqueued paths not reflected in queue status: %#v", snapshot.Status)
	}
	if snapshot.TicketChecked["a"] || snapshot.TicketChecked["b"] {
		t.Fatalf("cleared paths still checked: %#v", snapshot.TicketChecked)
	}
	if !snapshot.TicketChecked["keep-checked"] {
		t.Fatalf("unrelated checked entry was cleared: %#v", snapshot.TicketChecked)
	}
}

// TestQueueStoreMigratesLegacyItemsIntoIndependentCheckedSet asserts ticket
// 13's migration design: a pre-decoupling queue-state.json (no "checked"
// key) has every one of its Items entries treated as both checked and
// queued on first load, since the old format never distinguished the two.
func TestQueueStoreMigratesLegacyItemsIntoIndependentCheckedSet(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "queue.json")
	legacy := `{"items":{"a":"pending","b":"done"},"check_order":{"a":1,"b":2}}`
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	store := loadQueueStoreAt(path)
	snapshot := store.Snapshot()
	if snapshot.Status["a"] != queueStatusPending || snapshot.Status["b"] != queueStatusDone {
		t.Fatalf("legacy queue status lost: %#v", snapshot.Status)
	}
	if snapshot.Order["a"] != 1 || snapshot.Order["b"] != 2 {
		t.Fatalf("legacy check_order lost: %#v", snapshot.Order)
	}
	if !snapshot.TicketChecked["a"] || !snapshot.TicketChecked["b"] {
		t.Fatalf("legacy items not migrated into independent checked set: %#v", snapshot.TicketChecked)
	}
	if snapshot.TicketCheckOrder["a"] != 1 || snapshot.TicketCheckOrder["b"] != 2 {
		t.Fatalf("legacy check_order not migrated into independent check order: %#v", snapshot.TicketCheckOrder)
	}

	if err := store.SetTicketChecked([]string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	reread, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk persistedQueueState
	if err := json.Unmarshal(reread, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Checked["a"] || !onDisk.Checked["b"] {
		t.Fatalf("post-migration write did not persist the independent checked shape: %#v", onDisk)
	}
	if onDisk.Items["a"] != queueStatusPending || onDisk.Items["b"] != queueStatusDone {
		t.Fatalf("post-migration write lost queue membership: %#v", onDisk.Items)
	}
}

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
	codexOnPath = func() bool { return true }
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

// TestModel_CheckingTicketPersistsToCheckedSet covers ticket 15's decoupling:
// "space" persists to the independent Tickets-tab checked set, not to queue
// membership/status (that's "i"'s job — see implement_test.go).
func TestModel_CheckingTicketPersistsToCheckedSet(t *testing.T) {
	// not parallel-safe: reassigns the package-level queueStateDirFn singleton
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

	snapshot := LoadQueueStore().Snapshot()
	if !snapshot.TicketChecked[ticket.Path] {
		t.Fatalf("expected ticket persisted to the independent checked set, got %#v", snapshot.TicketChecked)
	}
	if _, queued := snapshot.Status[ticket.Path]; queued {
		t.Fatalf("checking a ticket must not queue it: %#v", snapshot.Status)
	}
}

func TestModel_UncheckingTicketRemovesFromCheckedSet(t *testing.T) {
	// not parallel-safe: reassigns the package-level queueStateDirFn singleton
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

	got := LoadQueueStore().Snapshot().TicketChecked
	if _, ok := got[ticket.Path]; ok {
		t.Fatalf("expected ticket removed from persisted checked set after uncheck, got %v", got)
	}
}

// TestModel_RestoresCheckedSetAndQueueStatusOnStartup covers ticket 15's
// decoupling: the independent checked set and queue membership/status are
// two separate persisted concepts, and both survive a restart independently.
func TestModel_RestoresCheckedSetAndQueueStatusOnStartup(t *testing.T) {
	// not parallel-safe: reassigns the package-level queueStateDirFn singleton
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
	if err := m.queueStore.SetQueued([]string{ticket.Path}, true); err != nil {
		t.Fatal(err)
	}
	if err := m.queueStore.SetStatus(ticket.Path, queueStatusDone); err != nil {
		t.Fatal(err)
	}

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
