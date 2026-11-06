package tickets

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenWidth  = 100
	goldenHeight = 30
)

// assertGolden compares the ANSI-stripped text with testdata/<name>.golden, so
// a colour profile change cannot flip a golden. Later tickets never run
// -update on these goldens: a moved golden must show as a 100% rename.
func assertGolden(t *testing.T, name, text string) {
	t.Helper()
	text = ansi.Strip(text)
	golden := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if text != string(want) {
		t.Errorf("render mismatch (run with -update to accept)\ngot:\n%s\nwant:\n%s", text, want)
	}
}

// Fixture helpers: the white-box part of the golden setup lives here, so a
// type split edits these and never the goldens.

// goldenModel loads the disk fixture in root into a Model at the golden size.
func goldenModel(t *testing.T, root string) Model {
	t.Helper()
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoadDefaults(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: goldenWidth, Height: goldenHeight})
	return updated.(Model)
}

// sizeGoldenModel resizes a Model to the golden size.
func sizeGoldenModel(m Model) Model {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: goldenWidth, Height: goldenHeight})
	return updated.(Model)
}

// selectGoldenTicket selects the first ticket row and syncs the preview.
func selectGoldenTicket(t *testing.T, m Model) Model {
	t.Helper()
	m = selectTicketRow(t, m)
	m.syncPreviewViewport()
	return m
}

// goldenModelView renders a Model and returns the plain view text.
func goldenModelView(m Model) string { return m.View().Content }

func TestModelViewGolden_SidebarExpandedEpicAndDoneEpic(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-first-ticket.md", "Status: done\n\n# First ticket\n\nShipped.\n")
	tickettest.WriteTicket(t, root, "open-epic", "02-second-ticket.md", "Status: open\n\n# Second ticket\n\nStill to do.\n")
	tickettest.WriteTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\n# Only ticket\n\nDone.\n")

	m := goldenModel(t, root)
	expandOpenEpics(&m)
	m = selectGoldenTicket(t, m)
	assertGolden(t, "model_sidebar_epics", goldenModelView(m))
}

func TestModelViewGolden_PausedRowWithRepairPreview(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-stuck-ticket.md",
		"Status: paused\nPause reason: blocked-pane\n\n# Stuck ticket\n\nBody.\n\n"+tickets.LegacyRepairHeading+"\n\nThe agent pane blocked.\n")

	m := goldenModel(t, root)
	expandOpenEpics(&m)
	m = selectGoldenTicket(t, m)
	assertGolden(t, "model_paused_repair", goldenModelView(m))
}

func TestModelViewGolden_ServerModeClaimedTicketMetrics(t *testing.T) {
	snap := wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "Claimed ticket", Status: "claimed", ElapsedTime: 754, ActualContextWindow: 45_200, ActualCost: 1.5},
		{Address: "gx:alpha/02", Title: "Open ticket", Status: "open"},
	}}
	m := withState(newServerModel(t), snap)
	m = selectGoldenTicket(t, sizeGoldenModel(m))
	assertGolden(t, "model_server_claimed", goldenModelView(m))
}
