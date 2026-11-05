package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

func TestQueueModelRendersFlatDependencyOrderedEpicPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "03-independent.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "04-independent.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-foundation.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-dependent.md"):   true,
		tickettest.TicketPath(root, "alpha", "03-independent.md"): true,
		tickettest.TicketPath(root, "alpha", "04-independent.md"): true,
		tickettest.TicketPath(root, "beta", "01-other.md"):        true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := m.View().Content

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected a flat dependency-ordered list, not wave grouping:\n%s", content)
	}

	// maxParallel caps each wave: with the default cap, 01/03/04 (all
	// unblocked) don't all fit in wave 1, so 04 spills into wave 2 alongside
	// 02 (only just unblocked once 01 lands) — still ticket-number order
	// within each wave, and 02's row still lands after its blocker 01's.
	alpha := strings.Index(content, "alpha")
	foundation := strings.Index(content, "Foundation")
	independent3 := strings.Index(content, "03")
	dependent := strings.Index(content, "Dependent")
	independent4 := strings.Index(content, "04")
	beta := strings.Index(content, "beta")
	if alpha < 0 || foundation < alpha || independent3 < foundation || dependent < independent3 || independent4 < dependent || beta < independent4 {
		t.Fatalf("expected blockers-before-dependents order within alpha, then epic grouping into beta:\n%s", content)
	}
}

// TestQueueModelOrdersRowsByDependencyNotTicketNumber covers ticket 11: the
// Queue tab's row order must reflect actual execution order (blockers before
// dependents via schedule.PlanWaves), not plain ticket-number order — even
// when the blocker has a higher ticket number than its dependent.
func TestQueueModelOrdersRowsByDependencyNotTicketNumber(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "20-dependent.md", "Status: open\nBlocked by: 50\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "50-blocker.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "60-unrelated.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "10-unrelated.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "20-dependent.md"): true,
		tickettest.TicketPath(root, "alpha", "50-blocker.md"):   true,
		tickettest.TicketPath(root, "alpha", "60-unrelated.md"): true,
		tickettest.TicketPath(root, "alpha", "10-unrelated.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	rows := queueTicketEntries(m)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d: %+v", len(rows), rows)
	}

	indexOf := func(id string) int {
		for i, r := range rows {
			if r.Value.Ticket.Ticket.Identifier == id {
				return i
			}
		}
		t.Fatalf("ticket %q missing from rows: %+v", id, rows)
		return -1
	}

	// Wave 1 (no unmet blocker): 10, 50, 60, tiebroken by ticket number.
	// Wave 2 (unblocked once 50 lands): 20.
	if got := []int{indexOf("10"), indexOf("50"), indexOf("60")}; got[0] > got[1] || got[1] > got[2] {
		t.Fatalf("expected wave-1 tickets 10,50,60 in ticket-number order, got indexes %v", got)
	}
	if indexOf("20") < indexOf("50") {
		t.Fatalf("expected blocker 50's row before its dependent 20's row, got rows %+v", rows)
	}
}

// TestQueueModelNestsChildrenUnderParentAndCollapsesWithHL covers ticket 10's
// acceptance criteria: a checked ticket's children (Parent/Children, ticket
// 03) render nested underneath it in the Queue tab, and "h"/"l" collapse and
// re-expand that nesting.
func TestQueueModelNestsChildrenUnderParentAndCollapsesWithHL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-parent.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "02-child.md", "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "03-other.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-parent.md"): true,
		tickettest.TicketPath(root, "alpha", "02-child.md"):  true,
		tickettest.TicketPath(root, "alpha", "03-other.md"):  true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	rows := queueTicketEntries(m)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[0].Depth != 0 || !rows[0].HasChildren || !rows[0].Expanded {
		t.Fatalf("expected parent 01 at depth 0 with children, got %+v", rows[0])
	}
	if rows[1].Value.Ticket.Ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected child 02 nested at depth 1 right after its parent, got %+v", rows[1])
	}
	if rows[2].Value.Ticket.Ticket.Identifier != "03" || rows[2].Depth != 0 {
		t.Fatalf("expected unrelated ticket 03 at depth 0, got %+v", rows[2])
	}

	content := m.View().Content
	if !strings.Contains(content, "▼") {
		t.Fatalf("expected an expanded-triangle glyph for the parent row:\n%s", content)
	}

	// Select the parent row (now behind the epic's header rows) before "h" collapses its children.
	m = selectFirstQueueTicketRow(t, m)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(QueueModel)
	rows = queueTicketEntries(m)
	if len(rows) != 2 || rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[1].Value.Ticket.Ticket.Identifier != "03" {
		t.Fatalf("expected only 01 and 03 after collapsing 01's children, got %+v", rows)
	}
	if strings.Contains(m.View().Content, "Child") {
		t.Fatalf("expected child ticket hidden after collapse:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(QueueModel)
	rows = queueTicketEntries(m)
	if len(rows) != 3 {
		t.Fatalf("expected \"l\" to re-expand and restore all 3 rows, got %d: %+v", len(rows), rows)
	}
}

// An investigate child nests under its one-off exactly as any fork child nests
// under an epic ticket.
func TestQueueModelNestsInvestigateChildUnderOneOff(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "fix-it", "01-prompt.md", "Status: paused\nPause reason: blocked-pane\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "fix-it", "02-investigate.md", "---\nid: \"02\"\nstatus: open\ntype: investigate\nparent: \"01\"\n---\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "fix-it", "01-prompt.md"):      true,
		tickettest.TicketPath(root, "fix-it", "02-investigate.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	rows := queueTicketEntries(m)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[0].Depth != 0 || !rows[0].HasChildren {
		t.Fatalf("expected one-off 01 at depth 0 with children, got %+v", rows[0])
	}
	if rows[1].Value.Ticket.Ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected investigate child 02 nested at depth 1, got %+v", rows[1])
	}
}

// queueTicketEntries filters m.rows.queueTree.Entries() down to nodeQueueTicket
// rows, dropping the per-epic separator/status/context/error entries
// buildQueueEntries interleaves — the tree's own Depth/HasChildren/
// Expanded on each Entry (not queueRow's mirrored fields, which only exist to
// drive the fold-glyph render) is the nesting truth these tests assert
// against, per ticket 19's live-render-path rewrite.
func queueTicketEntries(m QueueModel) []tree.Entry[queue.Node] {
	var out []tree.Entry[queue.Node]
	for _, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == queue.NodeTicket {
			out = append(out, e)
		}
	}
	return out
}

// TestQueueModelNestsChildrenAtArbitraryDepthAndRespectsCollapse ports
// queue_rows_test.go's TestQueueRowsForEpic_NestsChildrenAtArbitraryDepthAndRespectsCollapse
// (ticket 19): a ticket (01) with a child (02) that itself has a child (03)
// nests two levels deep via m.rows.queueTree.Entries(), leaving an unrelated
// ticket (04) at the top level, and collapsing the grandparent via "h" hides
// both descendants while leaving 04 visible.
func TestQueueModelNestsChildrenAtArbitraryDepthAndRespectsCollapse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "open", "")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "03-c.md", "03", "open", "02")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "04-d.md", "04", "open", "")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-a.md"): true,
		tickettest.TicketPath(root, "alpha", "02-b.md"): true,
		tickettest.TicketPath(root, "alpha", "03-c.md"): true,
		tickettest.TicketPath(root, "alpha", "04-d.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	rows := queueTicketEntries(m)
	wantIDs := []string{"01", "02", "03", "04"}
	wantDepths := []int{0, 1, 2, 0}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, r := range rows {
		if r.Value.Ticket.Ticket.Identifier != wantIDs[i] {
			t.Fatalf("row %d ticket = %q, want %q", i, r.Value.Ticket.Ticket.Identifier, wantIDs[i])
		}
		if r.Depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, wantIDs[i], r.Depth, wantDepths[i])
		}
	}
	if !rows[0].HasChildren || !rows[0].Expanded {
		t.Fatalf("expected ticket 01 to report hasChildren+expanded, got %+v", rows[0])
	}

	m = selectFirstQueueTicketRow(t, m)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(QueueModel)
	rows = queueTicketEntries(m)
	wantAfterCollapse := []string{"01", "04"}
	if len(rows) != len(wantAfterCollapse) {
		t.Fatalf("expected %d rows after collapsing 01, got %d: %+v", len(wantAfterCollapse), len(rows), rows)
	}
	for i, want := range wantAfterCollapse {
		if rows[i].Value.Ticket.Ticket.Identifier != want {
			t.Fatalf("row %d after collapse = %q, want %q", i, rows[i].Value.Ticket.Ticket.Identifier, want)
		}
	}
	if rows[0].Expanded {
		t.Fatalf("expected collapsed ticket 01 to report expanded=false")
	}
}

// TestQueueModelInjectsNonCandidateAncestorDimmedInsteadOfPromotingToRoot
// covers ticket 08: when a checked ticket's real parent isn't itself a
// scheduling candidate (unchecked), the tree stays connected by injecting
// that parent as a dimmed, non-actionable row rather than promoting the
// checked ticket to the top level.
func TestQueueModelInjectsNonCandidateAncestorDimmedInsteadOfPromotingToRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "open", "")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "03-c.md", "03", "open", "")
	checked := map[string]bool{
		// 01 (the parent) is deliberately left unchecked: not a scheduling
		// candidate on its own.
		tickettest.TicketPath(root, "alpha", "02-b.md"): true,
		tickettest.TicketPath(root, "alpha", "03-c.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	rows := queueTicketEntries(m)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (injected 01 + checked 02/03), got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[0].Depth != 0 {
		t.Fatalf("expected injected ancestor 01 at depth 0, got %+v", rows[0])
	}
	if rows[0].Value.Ticket.Actionable {
		t.Fatalf("expected injected ancestor 01 to be marked non-actionable, got %+v", rows[0].Value.Ticket)
	}
	if rows[1].Value.Ticket.Ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected checked ticket 02 nested under injected 01 at depth 1, got %+v", rows[1])
	}
	if !rows[1].Value.Ticket.Actionable {
		t.Fatalf("expected checked ticket 02 to stay marked actionable, got %+v", rows[1].Value.Ticket)
	}
	if rows[2].Value.Ticket.Ticket.Identifier != "03" || rows[2].Depth != 0 {
		t.Fatalf("expected unrelated checked ticket 03 at depth 0, got %+v", rows[2])
	}

	content := m.View().Content
	plain := ansi.Strip(content)
	if !strings.Contains(plain, "01") {
		t.Fatalf("expected injected ancestor 01's row to render:\n%s", plain)
	}
}

// TestQueueModelSharedNonCandidateAncestorInjectedOnce covers the dedup path
// of attachQueueAncestors: two checked tickets sharing the same non-candidate
// parent must only inject that parent once, with both children nested under
// the single injected row.
func TestQueueModelSharedNonCandidateAncestorInjectedOnce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "open", "")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "03-c.md", "03", "open", "01")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "02-b.md"): true,
		tickettest.TicketPath(root, "alpha", "03-c.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	rows := queueTicketEntries(m)
	wantIDs := []string{"01", "02", "03"}
	wantDepths := []int{0, 1, 1}
	if len(rows) != len(wantIDs) {
		t.Fatalf("expected a single injected 01 with both 02 and 03 nested under it, got %d rows: %+v", len(rows), rows)
	}
	for i, want := range wantIDs {
		if rows[i].Value.Ticket.Ticket.Identifier != want {
			t.Fatalf("row %d = %q, want %q: %+v", i, rows[i].Value.Ticket.Ticket.Identifier, want, rows)
		}
		if rows[i].Depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, want, rows[i].Depth, wantDepths[i])
		}
	}
}

// TestQueueModelDoneParentWithOpenForkChildStaysVisible ports
// queue_rows_test.go's TestQueueRowsForEpic_DoneParentWithOpenForkChildStaysVisible
// (ticket 19): a done parent whose fork child (Parent: ticket 03) is still
// open must not be dropped by hideComplete's filterDoneTickets — the child
// stays nested under its still-live parent in m.rows.queueTree.Entries() instead
// of being reattached to the top level.
func TestQueueModelDoneParentWithOpenForkChildStaysVisible(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "done", "")
	tickettest.WriteFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-a.md"): true,
		tickettest.TicketPath(root, "alpha", "02-b.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)

	rows := queueTicketEntries(m)
	if len(rows) != 2 || rows[0].Value.Ticket.Ticket.Identifier != "01" || rows[1].Value.Ticket.Ticket.Identifier != "02" {
		t.Fatalf("expected 01 (waiting-for-children) and nested 02, got %+v", rows)
	}
	if rows[1].Depth != 1 {
		t.Fatalf("expected ticket 02 nested under 01 at depth 1, got depth=%d", rows[1].Depth)
	}
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

func TestQueueModelNeverShowsATicketRunnableWhenOutOfScopeBlockerIsUnmet(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	// Only the dependent ticket is checked — its blocker never got selected,
	// and it isn't done, so ralph-loop's own scheduler would never claim it
	// either (schedule.RunScope.Frontier resolves blockers epic-wide).
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "02-dependent.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := m.View().Content

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected no runnable wave for a ticket whose out-of-scope blocker is unmet:\n%s", content)
	}
	if !strings.Contains(content, "Dependent") {
		t.Fatalf("expected the checked ticket to remain visible for toggling:\n%s", content)
	}
	if !strings.Contains(content, "no unblocked tickets left") {
		t.Fatalf("expected an actionable plan error instead of a misleading wave:\n%s", content)
	}
}

func TestQueueModelSurfacesActionableErrorForDependencyCycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\nBlocked by: 02\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := m.View().Content

	if !strings.Contains(content, "no unblocked tickets left") {
		t.Fatalf("expected an actionable cycle error:\n%s", content)
	}
	if !strings.Contains(content, "First") || !strings.Contains(content, "Second") {
		t.Fatalf("expected both cyclic tickets to remain visible for toggling:\n%s", content)
	}
}

func TestQueueModelIncludesSelectionsAddedAfterLoad(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "01-later.md"
	tickettest.WriteTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	path := tickettest.TicketPath(root, "alpha", name)
	checked := map[string]bool{}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	checked[path] = true
	if content := m.View().Content; !strings.Contains(content, "Later") {
		t.Fatalf("expected cached Queue model to include a later shared selection:\n%s", content)
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
func isSelectableQueueKind(kind queue.NodeKind) bool {
	return kind == queue.NodeTicket || kind == queue.NodeEpicName
}

func selectFirstQueueTicketRow(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	for i, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == queue.NodeTicket {
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
