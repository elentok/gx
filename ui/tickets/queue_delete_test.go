package tickets

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func xPress() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'x', Text: "x"}
}

func TestQueueModel_XOpensConfirmModalListingFullCascade(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-root.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-root.md"):      true,
		ticketPath(root, "alpha", "02-dependent.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	m.selected = 0 // row 0 is the first ticket row (no epic header row in queue tab)

	updated, _ := m.Update(xPress())
	m = updated.(QueueModel)

	if !m.confirm.IsOpen {
		t.Fatalf("expected confirm modal open after x")
	}
	content := m.confirm.View(120)
	if !strings.Contains(content, "Root") {
		t.Fatalf("expected modal to mention the target ticket, got:\n%s", content)
	}
	if !strings.Contains(content, "Dependent") {
		t.Fatalf("expected modal to list the full cascade set, got:\n%s", content)
	}

	// Nothing deleted yet.
	if _, err := os.Stat(ticketPath(root, "alpha", "01-root.md")); err != nil {
		t.Fatalf("expected ticket file to still exist before confirming: %v", err)
	}
}

func TestQueueModel_XConfirmedDeletesCascadeAndClearsDoneSurvivor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-root.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-done-dependent.md", "Status: done\nBlocked by: 01\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-behind-done.md", "Status: open\nBlocked by: 02\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-root.md"):           true,
		ticketPath(root, "alpha", "02-done-dependent.md"): true,
		ticketPath(root, "alpha", "03-behind-done.md"):    true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	m.selected = 0

	updated, _ := m.Update(xPress())
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatalf("expected confirm modal open after x")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(QueueModel)
	if cmd == nil {
		t.Fatalf("expected a command from confirming")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(QueueModel)
	if cmd != nil {
		updated, _ = m.Update(cmd())
		m = updated.(QueueModel)
	}

	if m.confirm.IsOpen {
		t.Fatalf("expected modal closed after confirming")
	}

	if _, err := os.Stat(ticketPath(root, "alpha", "01-root.md")); !os.IsNotExist(err) {
		t.Fatalf("expected root ticket deleted, stat err = %v", err)
	}

	donePath := ticketPath(root, "alpha", "02-done-dependent.md")
	if _, err := os.Stat(donePath); err != nil {
		t.Fatalf("expected done dependent to survive: %v", err)
	}
	raw, err := os.ReadFile(donePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "blocked_by") {
		t.Fatalf("expected dangling blocked_by entry cleared, got:\n%s", string(raw))
	}

	if _, err := os.Stat(ticketPath(root, "alpha", "03-behind-done.md")); err != nil {
		t.Fatalf("expected ticket behind the done survivor to remain untouched: %v", err)
	}
}
