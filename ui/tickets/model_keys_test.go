package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tree"
)

// selectArchivedTicketRow expands the Archived section then its single
// archived epic and moves the cursor onto that epic's one ticket row,
// mirroring TestModel_ExpandArchivedSectionLoadsAndPopulatesCollapsedByDefault's
// expand sequence (both the section and the epic default to collapsed, ticket
// 04's closedEpicDefaults).
func selectArchivedTicketRow(t *testing.T, m Model) Model {
	t.Helper()
	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	m = deliverCmd(t, m, cmd)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, ok := m.selectedRow()
	if !ok || r.isEpic() || !r.archived {
		t.Fatalf("expected selection on archived ticket row, got row=%+v ok=%v", r, ok)
	}
	return m
}

// TestModel_MutatingKeysAreNoOpOnArchivedRow covers 05's core gate: every
// mutating keybinding is a no-op on a row sourced from the Archived section
// — no write, no state mutation, no navigation away — and reports why via an
// info notification.
func TestModel_MutatingKeysAreNoOpOnArchivedRow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m = selectArchivedTicketRow(t, m)
	selectionBefore, _ := m.selectedRow()
	ticket := m.epicAt(selectionBefore).Tickets[selectionBefore.ticketIdx]

	checkedBefore := len(m.checked)
	queueBefore := m.queueStore.Snapshot()

	presses := []tea.KeyPressMsg{
		{Code: 's', Text: "s"},
		{Code: 'r', Text: "r"},
		{Code: 'a', Text: "a"},
		{Code: 'D', Text: "D"},
		{Code: ' ', Text: " "},
		{Code: 'm', Text: "m"},
	}
	for _, press := range presses {
		updated, cmd := m.Update(press)
		m = updated.(Model)
		if cmd == nil {
			t.Fatalf("press %q: expected an info notification explaining the no-op", press.Text)
		}
		nm, ok := cmd().(notify.NotifyMsg)
		if !ok || nm.Kind != notify.KindInfo || !strings.Contains(nm.Message, "archived") {
			t.Fatalf("press %q: expected archived info notification, got %#v (ok=%v)", press.Text, nm, ok)
		}
		selectionAfter, ok := m.selectedRow()
		if !ok || selectionAfter != selectionBefore {
			t.Fatalf("press %q moved selection away from archived row: before=%+v after=%+v ok=%v", press.Text, selectionBefore, selectionAfter, ok)
		}
	}

	if len(m.checked) != checkedBefore {
		t.Fatalf("expected checked set unaffected by mutating keys on archived row, before=%d after=%d", checkedBefore, len(m.checked))
	}
	if m.isChecked(ticket.Path) {
		t.Fatalf("expected archived ticket to remain unchecked after space")
	}
	afterQueue := m.queueStore.Snapshot()
	if len(afterQueue.TicketChecked) != len(queueBefore.TicketChecked) {
		t.Fatalf("expected queue store unaffected by mutating keys on archived row")
	}
}

// TestModel_ReadOnlyKeysStillWorkOnArchivedRow covers 05's other half: edit,
// yank, and hide-done toggle stay unaffected on an archived-sourced row.
func TestModel_ReadOnlyKeysStillWorkOnArchivedRow(t *testing.T) {
	root := t.TempDir()
	writeArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: open\n\nBody.\n")
	t.Setenv("EDITOR", "true")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m = selectArchivedTicketRow(t, m)
	selectionBefore, _ := m.selectedRow()

	// Edit-in-editor ("ee") must still resolve and launch, exactly as for a
	// non-archived row.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected edit-in-editor to return a launch command on an archived row")
	}

	// Yank summary still fires (not swallowed by the archived guard).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected yank-summary to return a command on an archived row")
	}

	// Hide-done toggle still flips.
	hideDoneBefore := m.hideDone
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if m.hideDone == hideDoneBefore {
		t.Fatalf("expected hide-done toggle to flip on an archived row")
	}

	selectionAfter, ok := m.selectedRow()
	if !ok || selectionAfter.ticketIdx != selectionBefore.ticketIdx || !selectionAfter.archived {
		t.Fatalf("expected selection to remain on the archived ticket row, before=%+v after=%+v", selectionBefore, selectionAfter)
	}
	if !strings.Contains(m.View().Content, "old-epic") {
		t.Fatalf("expected archived epic still rendered after read-only keys, got:\n%s", m.View().Content)
	}
}

// TestModel_ExpandArchivedSectionZeroCountDoesNotEngageLazyLoad covers 06a's
// first fix via the real key-handling path (handleKey), not by setting state
// directly: expanding the Archived header when the up-front count is 0 must
// render the shared empty placeholder without ever issuing a
// tickets.LoadArchived command or moving archivedLazy out of LazyIdle.
func TestModel_ExpandArchivedSectionZeroCountDoesNotEngageLazyLoad(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-open-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.sidebarTree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when expanding a zero-count Archived section")
	}
	if m.archivedLazy.State() != tree.LazyIdle {
		t.Fatalf("archivedLazy.State() = %v, want LazyIdle after expanding a zero-count section", m.archivedLazy.State())
	}
	if !strings.Contains(m.View().Content, "no archived epics") {
		t.Fatalf("expected empty-archive placeholder, got:\n%s", m.View().Content)
	}
}
