package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

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
