package queue

import (
	"testing"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/preview"
	rowspkg "github.com/elentok/gx/ui/tickets/queue/rows"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

// queueTicketEntries filters m.rows.queueTree.Entries() down to nodeQueueTicket
// rows, dropping the per-epic separator/status/context/error entries
// buildQueueEntries interleaves — the tree's own Depth/HasChildren/
// Expanded on each Entry (not queueRow's mirrored fields, which only exist to
// drive the fold-glyph render) is the nesting truth these tests assert
// against, per ticket 19's live-render-path rewrite.
func queueTicketEntries(m QueueModel) []tree.Entry[rowspkg.Node] {
	var out []tree.Entry[rowspkg.Node]
	for _, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == rowspkg.NodeTicket {
			out = append(out, e)
		}
	}
	return out
}

// TestQueueModelLOnLeafRowFocusesPreview covers ticket 12: "l"/"right"/"enter"
// on a leaf row (no children) hands focus straight to the preview panel,
// mirroring the Tickets tab's own focus-or-expand dispatch.
func TestQueueModelLOnLeafRowFocusesPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-solo.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)

	if m.preview.Focus != preview.FocusSidebar {
		t.Fatalf("expected initial focus on the list, got focus=%v", m.preview.Focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(QueueModel)
	if m.preview.Focus != preview.FocusPreview {
		t.Fatalf("expected \"l\" on a leaf row to focus the preview, got focus=%v", m.preview.Focus)
	}
}

// TestQueueModelEnterOnExpandedParentFocusesPreview covers the parent-row
// case: since the parent starts expanded, "enter" must not collapse it — it
// focuses the preview instead, exactly like the leaf-row case above. A run is
// already in flight (m.runningEpics) so enter's other job — launching the
// checked queue — isn't actionable and falls through to the focus-toggle (see
// TestQueueModelEnterStillLaunchesCheckedQueueWhenActionable for the
// opposite, launch-wins case).
func TestQueueModelEnterOnExpandedParentFocusesPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-parent.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "02-child.md", "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-parent.md"): true,
		tickettest.TicketPath(root, "alpha", "02-child.md"):  true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)
	m.server.RunningEpics = map[string]bool{"already-running": true}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	if m.preview.Focus != preview.FocusPreview {
		t.Fatalf("expected enter on already-expanded parent row to focus preview, got focus=%v", m.preview.Focus)
	}
	rows := queueTicketEntries(m)
	if len(rows) != 2 {
		t.Fatalf("expected parent to stay expanded (2 rows) after enter, got %d: %+v", len(rows), rows)
	}
}

// TestQueueModelHLeftEscReturnFocusFromPreview covers "h"/"left"/"esc" handing
// focus back to the list once the preview has it, mirroring Tickets'
// handlePreviewKey — each key tested from a fresh preview-focused model so one
// doesn't mask another's regression.
func TestQueueModelHLeftEscReturnFocusFromPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-solo.md"): true}

	for _, key := range []tea.KeyPressMsg{
		{Code: 'h', Text: "h"},
		{Code: tea.KeyLeft},
		{Code: tea.KeyEsc},
	} {
		m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(QueueModel)

		updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
		m = updated.(QueueModel)
		if m.preview.Focus != preview.FocusPreview {
			t.Fatalf("setup: expected \"l\" to focus preview, got focus=%v", m.preview.Focus)
		}

		updated, _ = m.Update(key)
		m = updated.(QueueModel)
		if m.preview.Focus != preview.FocusSidebar {
			t.Fatalf("expected %+v to return focus to the list, got focus=%v", key, m.preview.Focus)
		}
	}
}

// withChecked seeds a local-mode QueueModel's checked set, standing in for
// the queue a server load would supply.
func withChecked(m QueueModel, checked map[string]bool) QueueModel {
	m.rows.Checked = checked
	return m
}

// seedFromDisk fills the rows from the fixture's store, standing in for the
// state the app shell delivers.
func seedFromDisk(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	epics, err := tickets.Load(tabapi.ScratchDir(m.settings.TicketStore, m.worktreeRoot))
	if err != nil {
		t.Fatal(err)
	}
	m.applyEpics(epics, false)
	m.server.Summaries = map[string]wire.EpicSummary{}
	for _, sum := range fixtureSummaries(epics, nil) {
		m.server.Summaries[sum.Address] = sum
	}
	return m
}

func loadQueueModel(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	m = seedFromDisk(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	return updated.(QueueModel)
}

// selectFirstQueueTicketRow moves the selection to the first nodeQueueTicket
// entry, skipping the per-epic separator/status/context header rows that now
// precede it. Several tests assumed SetSelectedIndex(0)/SelectedIndex()==0
// landed on the first ticket row before buildQueueEntries started emitting
// those headers.
// firstSelectableQueueIndex returns the index of the first row the
// selectability policy set in NewQueueModel considers selectable — the
// per-epic separator/status/context header rows never do, so tests that
// assert the cursor landed "at the top" must compare against this instead of
// a hardcoded 0.
func firstSelectableQueueIndex(t *testing.T, m QueueModel) int {
	t.Helper()
	for i, e := range m.rows.Tree.Entries() {
		if isSelectableQueueKind(e.Value.Kind) {
			return i
		}
	}
	t.Fatalf("expected at least one selectable row, found none")
	return -1
}

// isSelectableQueueKind is a ticket row or an epic header.
func isSelectableQueueKind(kind rowspkg.NodeKind) bool {
	return kind == rowspkg.NodeTicket || kind == rowspkg.NodeEpicName
}

func selectFirstQueueTicketRow(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	for i, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == rowspkg.NodeTicket {
			m.rows.Tree.SetSelectedIndex(i)
			// syncQueuePreviewViewport normally runs inside Update; moving
			// selection directly on queueTree bypasses that, so re-run it
			// here to keep previewVP in sync with the new selection.
			m.syncQueuePreviewViewport()
			return m
		}
	}
	t.Fatalf("expected at least one ticket row, found none")
	return m
}
