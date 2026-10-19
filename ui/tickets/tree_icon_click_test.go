package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tree"
)

func TestModel_ClickOnEpicIconTogglesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	const epicIdx = 1
	epic := m.sidebarTree.Entries()[epicIdx]
	iconX := panelPaddingX + tree.LabelColumn(epic)
	before := len(m.sidebarTree.Entries())

	// A click far right of the icon only selects.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX + 10, Y: epicIdx + 1})
	m = updated.(Model)
	if got := len(m.sidebarTree.Entries()); got != before {
		t.Fatalf("click off the icon changed entries: %d -> %d", before, got)
	}

	for _, x := range []int{iconX, iconX - 1, iconX + 1} {
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: epicIdx + 1})
		m = updated.(Model)
		if got := len(m.sidebarTree.Entries()); got != before-1 {
			t.Fatalf("x=%d: click on icon should collapse the epic, entries %d -> %d", x, before, got)
		}
		if m.sidebarTree.SelectedIndex() != epicIdx {
			t.Fatalf("x=%d: click should select the epic row, got %d", x, m.sidebarTree.SelectedIndex())
		}
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: epicIdx + 1})
		m = updated.(Model)
		if got := len(m.sidebarTree.Entries()); got != before {
			t.Fatalf("x=%d: second click should re-expand, entries %d, want %d", x, got, before)
		}
	}
}

func TestQueueModel_ClickOnTicketIconTogglesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-parent.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-child.md", "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-parent.md"): true,
		ticketPath(root, "alpha", "02-child.md"):  true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	parentIdx := -1
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket && e.HasChildren {
			parentIdx = i
			break
		}
	}
	if parentIdx < 0 {
		t.Fatal("no parent ticket entry")
	}
	parent := m.queueTree.Entries()[parentIdx]
	iconX := panelPaddingX + tree.LabelColumn(parent) + 1
	y := 1 + len(m.queueHeaderBodyLines()) + parentIdx
	before := len(m.queueTree.Entries())

	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX + 10, Y: y})
	m = updated.(QueueModel)
	if got := len(m.queueTree.Entries()); got != before {
		t.Fatalf("click off the icon changed entries: %d -> %d", before, got)
	}

	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX, Y: y})
	m = updated.(QueueModel)
	if got := len(m.queueTree.Entries()); got != before-1 {
		t.Fatalf("click on icon should collapse the ticket, entries %d -> %d", before, got)
	}
	if m.queueTree.SelectedIndex() != parentIdx {
		t.Fatalf("click should select the parent row %d, got %d", parentIdx, m.queueTree.SelectedIndex())
	}

	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX, Y: y})
	m = updated.(QueueModel)
	if got := len(m.queueTree.Entries()); got != before {
		t.Fatalf("second click should re-expand, entries %d, want %d", got, before)
	}
}
