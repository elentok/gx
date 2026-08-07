package tickets

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// deliverAutoRefreshReload extracts and runs just the reload half of an
// autoRefreshMsg tick's tea.Batch(reload, cmdAutoRefresh()) result, feeding
// the reload's message back into Update — without invoking cmdAutoRefresh's
// own tea.Tick, which would block the test for autoRefreshInterval.
func deliverAutoRefreshReload[M tea.Model](t *testing.T, m M, cmd tea.Cmd) M {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected an autoRefreshMsg tick to produce a reload cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("expected autoRefreshMsg to batch a reload cmd, got %T", cmd())
	}
	updated, _ := m.Update(batch[0]())
	return updated.(M)
}

func TestModel_AutoRefreshesDataFromDiskWithoutManualReload(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected second ticket absent before it's written, got:\n%s", m.View().Content)
	}

	// Simulate a status change made outside this Update loop (another
	// process, ralph-loop, a manual edit) between poll ticks.
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	_, cmd := m.Update(autoRefreshMsg{})
	m = deliverAutoRefreshReload(t, m, cmd)

	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected second ticket visible after autoRefreshMsg tick, got:\n%s", m.View().Content)
	}
}

func TestQueueModel_AutoRefreshesDataFromDiskWithoutManualReload(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	// A ticket's status changes on disk (e.g. ralph-loop claims it) with no
	// manual reload action from this tab.
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")

	_, cmd := m.Update(autoRefreshMsg{})
	m = deliverAutoRefreshReload(t, m, cmd)

	got := m.epics[0].Tickets[0].Status
	if got != "claimed" {
		t.Fatalf("expected ticket status reloaded to 'claimed' after autoRefreshMsg tick, got %q", got)
	}
}

func TestQueueModel_SplitInsertsChildrenAfterOriginalPosition(t *testing.T) {
	root := t.TempDir()
	writeFrontmatterTicket(t, root, "alpha", "01-x.md", "01", "claimed", nil, "")
	writeFrontmatterTicket(t, root, "alpha", "02-y.md", "02", "open", nil, "")
	writeFrontmatterTicket(t, root, "alpha", "03-z.md", "03", "open", nil, "")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	xPath := ticketPath(root, "alpha", "01-x.md")
	yPath := ticketPath(root, "alpha", "02-y.md")
	zPath := ticketPath(root, "alpha", "03-z.md")
	for _, path := range []string{xPath, yPath, zPath} {
		if err := store.Check(path); err != nil {
			t.Fatal(err)
		}
	}

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))

	order := identifiersInOrder(t, m)
	if got := strings.Join(order, ","); got != "01,02,03" {
		t.Fatalf("expected initial queue order X,Y,Z, got %s", got)
	}

	// Simulate ticket 01 splitting mid-run into 01a/01b.
	writeFrontmatterTicket(t, root, "alpha", "01-x.md", "01", "claimed", []string{"01a", "01b"}, "")
	writeFrontmatterTicket(t, root, "alpha", "01a-x-cont.md", "01a", "open", nil, "01")
	writeFrontmatterTicket(t, root, "alpha", "01b-x-cont2.md", "01b", "open", nil, "01")

	_, cmd := m.Update(autoRefreshMsg{})
	m = deliverAutoRefreshReload(t, m, cmd)

	order = identifiersInOrder(t, m)
	if got := strings.Join(order, ","); got != "01,01a,01b,02,03" {
		t.Fatalf("expected split children inserted right after 01 (01,01a,01b,02,03), got %s", got)
	}
}

func identifiersInOrder(t *testing.T, m QueueModel) []string {
	t.Helper()
	rows := m.rows()
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ticket.Identifier
	}
	return out
}
