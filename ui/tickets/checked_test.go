package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// writeFrontmatterTicket writes a ticket file with real frontmatter fields
// (id/status/split/split_from), unlike writeTicket's LegacyTicketToFrontmatter
// conversion which doesn't carry split/split_from.
func writeFrontmatterTicket(t *testing.T, root, epic, filename, id, status string, split []string, splitFrom string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: %s\ntype: task\n", id, status)
	if len(split) > 0 {
		fmt.Fprintf(&b, "split: [\"%s\"]\n", strings.Join(split, "\", \""))
	}
	if splitFrom != "" {
		fmt.Fprintf(&b, "split_from: %q\n", splitFrom)
	}
	b.WriteString("---\nBody.\n")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func spacePress() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

func TestModel_SpaceTogglesCheckedOnTicketRow(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Move onto the ticket row (row 0 is the epic).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		t.Fatalf("expected selection on ticket row, got row=%+v ok=%v", r, ok)
	}
	ticket := m.epics[r.epicIdx].Tickets[r.ticketIdx]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	if !m.isChecked(ticket.Path) {
		t.Fatalf("expected ticket checked after space")
	}

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	if m.isChecked(ticket.Path) {
		t.Fatalf("expected ticket unchecked after second space")
	}
}

func TestModel_SpaceOnEpicRowChecksAllTickets(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		t.Fatalf("expected initial selection on epic row, got row=%+v ok=%v", r, ok)
	}
	epic := m.epics[r.epicIdx]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	for _, ticket := range epic.Tickets {
		if !m.isChecked(ticket.Path) {
			t.Fatalf("expected ticket %s checked after checking epic", ticket.Identifier)
		}
	}

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	for _, ticket := range epic.Tickets {
		if m.isChecked(ticket.Path) {
			t.Fatalf("expected ticket %s unchecked after unchecking epic", ticket.Identifier)
		}
	}
}

func TestModel_CheckingBlockedTicketOpensConfirmModal(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Rows: [epic, first-ticket, second-ticket]. Move to second-ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		t.Fatalf("expected selection on second-ticket, got row=%+v ok=%v", r, ok)
	}
	blockedTicket := m.epics[r.epicIdx].Tickets[r.ticketIdx]
	blockerTicket := m.epics[r.epicIdx].Tickets[0]
	if blockedTicket.Identifier == blockerTicket.Identifier {
		t.Fatalf("test setup error: expected two distinct tickets")
	}

	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	if !m.confirm.IsOpen {
		t.Fatalf("expected confirm modal open for blocked ticket")
	}
	if m.isChecked(blockedTicket.Path) {
		t.Fatalf("expected blocked ticket not yet checked before confirming")
	}
	content := m.confirm.View(120)
	if !strings.Contains(content, "blocked by") {
		t.Fatalf("expected modal to mention blockers, got:\n%s", content)
	}
	if !strings.Contains(content, "First ticket") {
		t.Fatalf("expected modal to list the blocking ticket's title, got:\n%s", content)
	}
}

func TestModel_CheckingTicketWithAlreadyCheckedBlockersSkipsConfirm(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Check the blocker first (row 1: first-ticket).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	// Move onto the blocked ticket (row 2: second-ticket) and check it.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		t.Fatalf("expected selection on second-ticket, got row=%+v ok=%v", r, ok)
	}
	blockedTicket := m.epics[r.epicIdx].Tickets[r.ticketIdx]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	if m.confirm.IsOpen {
		t.Fatalf("expected no confirmation modal when blocker is already checked")
	}
	if !m.isChecked(blockedTicket.Path) {
		t.Fatalf("expected ticket checked directly since its blocker was already checked")
	}
}

func TestModel_ConfirmingBlockedModalChecksTicketAndBlockers(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, _ := m.selectedRow()
	blockedTicket := m.epics[r.epicIdx].Tickets[r.ticketIdx]
	blockerTicket := m.epics[r.epicIdx].Tickets[0]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	// Confirm ("y" or enter with DefaultYes false -> need explicit accept).
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	if cmd != nil {
		updated, _ = m.Update(cmd())
		m = updated.(Model)
	}

	if m.confirm.IsOpen {
		t.Fatalf("expected modal closed after confirming")
	}
	if !m.isChecked(blockedTicket.Path) {
		t.Fatalf("expected blocked ticket checked after confirming")
	}
	if !m.isChecked(blockerTicket.Path) {
		t.Fatalf("expected blocker ticket also checked after confirming")
	}
}

func TestModel_CancelingBlockedModalLeavesCheckedSetUnchanged(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, _ := m.selectedRow()
	blockedTicket := m.epics[r.epicIdx].Tickets[r.ticketIdx]
	blockerTicket := m.epics[r.epicIdx].Tickets[0]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)

	if m.confirm.IsOpen {
		t.Fatalf("expected modal closed after canceling")
	}
	if m.isChecked(blockedTicket.Path) {
		t.Fatalf("expected blocked ticket to stay unchecked after canceling")
	}
	if m.isChecked(blockerTicket.Path) {
		t.Fatalf("expected blocker ticket to stay unchecked after canceling")
	}
}

func TestModel_MidRunSplitAutoChecksNewSibling(t *testing.T) {
	root := t.TempDir()
	writeFrontmatterTicket(t, root, "my-epic", "01-first-ticket.md", "01", "claimed", nil, "")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	ticket := m.epics[0].Tickets[0]

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	if !m.isChecked(ticket.Path) {
		t.Fatalf("expected ticket checked before simulating the split")
	}

	// Simulate a mid-run split: the parent's `split` frontmatter gains an
	// entry and the new sibling ticket appears on disk, exactly as
	// implement/SKILL.md's split convention writes it.
	writeFrontmatterTicket(t, root, "my-epic", "01-first-ticket.md", "01", "claimed", []string{"01a"}, "")
	writeFrontmatterTicket(t, root, "my-epic", "01a-first-ticket-cont.md", "01a", "open", nil, "01")

	m = deliverLoad(t, m)

	if m.confirm.IsOpen {
		t.Fatalf("expected no confirmation modal for an auto-checked split child")
	}
	var sibling tickets.Ticket
	found := false
	for _, tk := range m.epics[0].Tickets {
		if tk.Identifier == "01a" {
			sibling, found = tk, true
		}
	}
	if !found {
		t.Fatalf("expected sibling ticket 01a to be loaded")
	}
	if !m.isChecked(sibling.Path) {
		t.Fatalf("expected split sibling ticket auto-checked")
	}
}

func TestModel_SplitOnUncheckedTicketDoesNotAutoCheckSibling(t *testing.T) {
	root := t.TempDir()
	writeFrontmatterTicket(t, root, "my-epic", "01-first-ticket.md", "01", "claimed", nil, "")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	writeFrontmatterTicket(t, root, "my-epic", "01-first-ticket.md", "01", "claimed", []string{"01a"}, "")
	writeFrontmatterTicket(t, root, "my-epic", "01a-first-ticket-cont.md", "01a", "open", nil, "01")

	m = deliverLoad(t, m)

	var sibling tickets.Ticket
	found := false
	for _, tk := range m.epics[0].Tickets {
		if tk.Identifier == "01a" {
			sibling, found = tk, true
		}
	}
	if !found {
		t.Fatalf("expected sibling ticket 01a to be loaded")
	}
	if m.isChecked(sibling.Path) {
		t.Fatalf("expected split sibling to stay unchecked when the parent was never checked")
	}
}

func TestModel_CheckedRowsRenderDistinctMarker(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	uncheckedContent := m.View().Content

	updated, _ = m.Update(spacePress())
	m = updated.(Model)
	checkedContent := m.View().Content

	if uncheckedContent == checkedContent {
		t.Fatalf("expected view content to change once ticket is checked")
	}
	if !strings.Contains(checkedContent, m.icons().CheckboxChecked) {
		t.Fatalf("expected checked glyph in view, got:\n%s", checkedContent)
	}
}

func TestModel_CachedModelsRenderSelectionFromSharedQueueStore(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	store := loadQueueStoreAt(t.TempDir() + "/queue.json")

	first := NewModelWithStore(root, ui.Settings{}, keys.New(nil), store)
	first = deliverLoad(t, first)
	updated, _ := first.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	first = updated.(Model)
	updated, _ = first.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	first = updated.(Model)

	cached := NewModelWithStore(root, ui.Settings{}, keys.New(nil), store)
	cached = deliverLoad(t, cached)
	updated, _ = cached.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	cached = updated.(Model)

	ticket := first.epics[0].Tickets[0]
	updated, _ = first.Update(spacePress())
	first = updated.(Model)

	if !cached.isChecked(ticket.Path) {
		t.Fatal("cached model did not observe shared ticket selection")
	}
	if !cached.epicChecked(cached.epics[0]) {
		t.Fatal("cached model did not observe shared epic selection")
	}
}

func TestModel_BlockedConfirmationFailureKeepsPriorQueue(t *testing.T) {
	store := loadQueueStoreAt(t.TempDir() + "/queue.json")
	if err := store.SetTicketChecked([]string{"keep"}, true); err != nil {
		t.Fatal(err)
	}
	store.path = t.TempDir()
	m := Model{queueStore: store}

	updated, cmd := m.handleCheckAddConfirmed(checkAddConfirmedMsg{
		ticketPath:   "ticket",
		blockerPaths: []string{"blocker-a", "blocker-b"},
	})
	if cmd == nil {
		t.Fatal("expected save failure notification")
	}
	m = updated.(Model)
	snapshot := m.queueStore.Snapshot()
	if len(snapshot.TicketChecked) != 1 || !snapshot.TicketChecked["keep"] {
		t.Fatalf("failed confirmation changed checked set: %#v", snapshot)
	}
}
