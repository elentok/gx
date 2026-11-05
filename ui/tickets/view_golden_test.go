package tickets

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
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

// goldenQueue builds a server-mode QueueModel at the golden size, with the
// clock fixed.
func goldenQueue(t *testing.T) QueueModel {
	t.Helper()
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil)).
		WithServerLink(tickettest.FakeServerAPI{}, nil).
		WithTickets(tickettest.FakeServerAPI{}).
		WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{}}).
		WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{}}).
		WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: goldenWidth, Height: goldenHeight})
	return next.(QueueModel)
}

// goldenQueueState delivers snap and queue to m and fixes the clock.
func goldenQueueState(m QueueModel, snap wire.Snapshot, queue []wire.QueueItem, now time.Time) QueueModel {
	m, _ = deliverQueueTo(m, snap, queue)
	m.now = func() time.Time { return now }
	return m
}

// setGoldenLive stamps the live row's token count, the one live field the
// state delivery does not carry.
func setGoldenLive(m QueueModel, epic, id string, tokens int) QueueModel {
	l := m.server.Live[epic][id]
	l.Tokens = tokens
	m.server.Live[epic][id] = l
	return m
}

func goldenSnapshotTickets(status string, claimedAt time.Time) []wire.TicketInfo {
	return []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "done", ElapsedTime: 754, ActualContextWindow: 45_200, ActualCost: 1.5},
		{Address: "gx:alpha/02", Title: "Second", Status: status, ClaimedAt: claimedAt},
		{Address: "gx:alpha/03", Title: "Third", Status: "open", BlockedBy: []string{"gx:alpha/02"}},
	}
}
