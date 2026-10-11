package tickets

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// stubClipboard swaps ticketsClipboardWrite for the test's duration, so these
// tests must not run in parallel.
func stubClipboard(t *testing.T) *string {
	t.Helper()
	var got string
	orig := ticketsClipboardWrite
	ticketsClipboardWrite = func(s string) error { got = s; return nil }
	t.Cleanup(func() { ticketsClipboardWrite = orig })
	return &got
}

func selectEpicRow(t *testing.T, m *Model, name string) {
	t.Helper()
	for i, e := range m.sidebarTree.Entries() {
		if e.Value.kind == nodeEpic && !e.Value.archived && m.epics[e.Value.epicIdx].Name == name {
			m.sidebarTree.SetSelectedIndex(i)
			return
		}
	}
	t.Fatalf("no epic row %q", name)
}

func TestYank_EpicRowSummaryIsEpicName(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first.md", "Status: open\n\nBody.\n")
	m := deliverLoad(t, NewModel(root, ui.Settings{}, keys.New(nil)))
	selectEpicRow(t, &m, "my-epic")
	got := stubClipboard(t)

	m.yankTicketSummary()()

	if *got != "my-epic" {
		t.Fatalf("expected epic name yanked, got %q", *got)
	}
}

func TestYank_EpicRowFilePathIsTicketMD(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first.md", "Status: open\n\nBody.\n")
	m := deliverLoad(t, NewModel(root, ui.Settings{}, keys.New(nil)))
	selectEpicRow(t, &m, "my-epic")
	got := stubClipboard(t)

	m.yankTicketFilePath()()

	want := filepath.Join(root, ".scratch", "my-epic", "ticket.md")
	if *got != want {
		t.Fatalf("expected %q yanked, got %q", want, *got)
	}
}

func TestEpicTicketMDPath_ServerModeEpicFindsDirFromTicketFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first.md", "Status: open\n\nBody.\n")
	file := filepath.Join(root, ".scratch", "my-epic", "issues", "01-first.md")
	epic := tickets.Epic{
		Name:    "proj:my-epic",
		Path:    "proj:my-epic",
		Tickets: []tickets.Ticket{{Path: "proj:my-epic/01", File: file}},
	}

	got, ok := epicTicketMDPath(epic)

	want := filepath.Join(root, ".scratch", "my-epic", "ticket.md")
	if !ok || got != want {
		t.Fatalf("expected %q, got %q (ok=%v)", want, got, ok)
	}
}
