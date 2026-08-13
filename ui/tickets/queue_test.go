package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tree"
)

func TestQueueModelRendersFlatDependencyOrderedEpicPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-independent.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "04-independent.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-foundation.md"):  true,
		ticketPath(root, "alpha", "02-dependent.md"):   true,
		ticketPath(root, "alpha", "03-independent.md"): true,
		ticketPath(root, "alpha", "04-independent.md"): true,
		ticketPath(root, "beta", "01-other.md"):        true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
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
// dependents via ralphloop.PlanWaves), not plain ticket-number order — even
// when the blocker has a higher ticket number than its dependent.
func TestQueueModelOrdersRowsByDependencyNotTicketNumber(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "20-dependent.md", "Status: open\nBlocked by: 50\n\nBody.\n")
	writeTicket(t, root, "alpha", "50-blocker.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "60-unrelated.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "10-unrelated.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "20-dependent.md"): true,
		ticketPath(root, "alpha", "50-blocker.md"):   true,
		ticketPath(root, "alpha", "60-unrelated.md"): true,
		ticketPath(root, "alpha", "10-unrelated.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	rows := m.rows()
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d: %+v", len(rows), rows)
	}

	indexOf := func(id string) int {
		for i, r := range rows {
			if r.ticket.Identifier == id {
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
	writeTicket(t, root, "alpha", "01-parent.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-child.md", "---\nid: \"02\"\nstatus: open\ntype: task\nparent: \"01\"\n---\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-other.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-parent.md"): true,
		ticketPath(root, "alpha", "02-child.md"):  true,
		ticketPath(root, "alpha", "03-other.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	rows := queueTicketEntries(m)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Value.ticket.ticket.Identifier != "01" || rows[0].Depth != 0 || !rows[0].HasChildren || !rows[0].Expanded {
		t.Fatalf("expected parent 01 at depth 0 with children, got %+v", rows[0])
	}
	if rows[1].Value.ticket.ticket.Identifier != "02" || rows[1].Depth != 1 {
		t.Fatalf("expected child 02 nested at depth 1 right after its parent, got %+v", rows[1])
	}
	if rows[2].Value.ticket.ticket.Identifier != "03" || rows[2].Depth != 0 {
		t.Fatalf("expected unrelated ticket 03 at depth 0, got %+v", rows[2])
	}

	content := m.View().Content
	if !strings.Contains(content, "▶") {
		t.Fatalf("expected an expanded-triangle glyph for the parent row:\n%s", content)
	}

	// Select the parent row (now behind the epic's header rows) before "h" collapses its children.
	m = selectFirstQueueTicketRow(t, m)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(QueueModel)
	rows = queueTicketEntries(m)
	if len(rows) != 2 || rows[0].Value.ticket.ticket.Identifier != "01" || rows[1].Value.ticket.ticket.Identifier != "03" {
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

// queueTicketEntries filters m.queueTree.Entries() down to nodeQueueTicket
// rows, dropping the per-epic separator/status/context/error/park-reason
// entries buildQueueEntries interleaves — the tree's own Depth/HasChildren/
// Expanded on each Entry (not queueRow's mirrored fields, which only exist to
// drive the fold-glyph render) is the nesting truth these tests assert
// against, per ticket 19's live-render-path rewrite.
func queueTicketEntries(m QueueModel) []tree.Entry[queueNode] {
	var out []tree.Entry[queueNode]
	for _, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket {
			out = append(out, e)
		}
	}
	return out
}

// TestQueueModelNestsChildrenAtArbitraryDepthAndRespectsCollapse ports
// queue_rows_test.go's TestQueueRowsForEpic_NestsChildrenAtArbitraryDepthAndRespectsCollapse
// (ticket 19): a ticket (01) with a child (02) that itself has a child (03)
// nests two levels deep via m.queueTree.Entries(), leaving an unrelated
// ticket (04) at the top level, and collapsing the grandparent via "h" hides
// both descendants while leaving 04 visible.
func TestQueueModelNestsChildrenAtArbitraryDepthAndRespectsCollapse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "open", "")
	writeFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	writeFrontmatterTicket(t, root, "alpha", "03-c.md", "03", "open", "02")
	writeFrontmatterTicket(t, root, "alpha", "04-d.md", "04", "open", "")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-a.md"): true,
		ticketPath(root, "alpha", "02-b.md"): true,
		ticketPath(root, "alpha", "03-c.md"): true,
		ticketPath(root, "alpha", "04-d.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	rows := queueTicketEntries(m)
	wantIDs := []string{"01", "02", "03", "04"}
	wantDepths := []int{0, 1, 2, 0}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, r := range rows {
		if r.Value.ticket.ticket.Identifier != wantIDs[i] {
			t.Fatalf("row %d ticket = %q, want %q", i, r.Value.ticket.ticket.Identifier, wantIDs[i])
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
		if rows[i].Value.ticket.ticket.Identifier != want {
			t.Fatalf("row %d after collapse = %q, want %q", i, rows[i].Value.ticket.ticket.Identifier, want)
		}
	}
	if rows[0].Expanded {
		t.Fatalf("expected collapsed ticket 01 to report expanded=false")
	}
}

// TestQueueModelDoneParentWithOpenForkChildStaysVisible ports
// queue_rows_test.go's TestQueueRowsForEpic_DoneParentWithOpenForkChildStaysVisible
// (ticket 19): a done parent whose fork child (Parent: ticket 03) is still
// open must not be dropped by hideComplete's filterDoneTickets — the child
// stays nested under its still-live parent in m.queueTree.Entries() instead
// of being reattached to the top level.
func TestQueueModelDoneParentWithOpenForkChildStaysVisible(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFrontmatterTicket(t, root, "alpha", "01-a.md", "01", "done", "")
	writeFrontmatterTicket(t, root, "alpha", "02-b.md", "02", "open", "01")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-a.md"): true,
		ticketPath(root, "alpha", "02-b.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)

	rows := queueTicketEntries(m)
	if len(rows) != 2 || rows[0].Value.ticket.ticket.Identifier != "01" || rows[1].Value.ticket.ticket.Identifier != "02" {
		t.Fatalf("expected 01 (waiting-for-children) and nested 02, got %+v", rows)
	}
	if rows[1].Depth != 1 {
		t.Fatalf("expected ticket 02 nested under 01 at depth 1, got depth=%d", rows[1].Depth)
	}
}

// TestQueueModelLOnLeafRowFocusesPreview covers ticket 12: "l"/"right"/"enter"
// on a leaf row (no children) hands focus straight to the preview panel,
// mirroring Tickets' focusPreviewOrExpand.
func TestQueueModelLOnLeafRowFocusesPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{ticketPath(root, "alpha", "01-solo.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)

	if m.focus != focusSidebar {
		t.Fatalf("expected initial focus on the list, got focus=%v", m.focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(QueueModel)
	if m.focus != focusPreview {
		t.Fatalf("expected \"l\" on a leaf row to focus the preview, got focus=%v", m.focus)
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
	writeTicket(t, root, "alpha", "01-parent.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-child.md", "---\nid: \"02\"\nstatus: open\ntype: task\nparent: \"01\"\n---\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-parent.md"): true,
		ticketPath(root, "alpha", "02-child.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)
	m.runningEpics = map[string]bool{"already-running": true}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	if m.focus != focusPreview {
		t.Fatalf("expected enter on already-expanded parent row to focus preview, got focus=%v", m.focus)
	}
	rows := m.rows()
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
	writeTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-solo.md"): true}

	for _, key := range []tea.KeyPressMsg{
		{Code: 'h', Text: "h"},
		{Code: tea.KeyLeft},
		{Code: tea.KeyEsc},
	} {
		m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(QueueModel)

		updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
		m = updated.(QueueModel)
		if m.focus != focusPreview {
			t.Fatalf("setup: expected \"l\" to focus preview, got focus=%v", m.focus)
		}

		updated, _ = m.Update(key)
		m = updated.(QueueModel)
		if m.focus != focusSidebar {
			t.Fatalf("expected %+v to return focus to the list, got focus=%v", key, m.focus)
		}
	}
}

// TestQueueModelEnterStillLaunchesCheckedQueueWhenActionable covers the
// enter-key precedence ticket 12 introduces: launching the checked queue
// (enter's pre-existing job) still wins over the focus-toggle when there's
// something actionable to launch, so the two meanings of "enter" don't fight
// over the same keypress on the common case of a leaf row with a runnable
// plan checked.
func TestQueueModelEnterStillLaunchesCheckedQueueWhenActionable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-solo.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	if !m.implementAgentMenuOpen {
		t.Fatalf("expected enter to open the implement-agent menu when a plan is checked and runnable")
	}
	if m.focus != focusSidebar {
		t.Fatalf("expected focus to stay on the list when enter launches instead of toggling, got focus=%v", m.focus)
	}
}

func TestQueueModelNeverShowsATicketRunnableWhenOutOfScopeBlockerIsUnmet(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	// Only the dependent ticket is checked — its blocker never got selected,
	// and it isn't done, so ralph-loop's own scheduler would never claim it
	// either (ralphloop.RunScope.Frontier resolves blockers epic-wide).
	checked := map[string]bool{
		ticketPath(root, "alpha", "02-dependent.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	content := m.View().Content

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected no runnable wave for a ticket whose out-of-scope blocker is unmet:\n%s", content)
	}
	if !strings.Contains(content, "Dependent") {
		t.Fatalf("expected the checked ticket to remain visible for toggling:\n%s", content)
	}
	if !strings.Contains(content, "no unblocked tickets") {
		t.Fatalf("expected an actionable plan error instead of a misleading wave:\n%s", content)
	}
}

func TestQueueModelSurfacesActionableErrorForDependencyCycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\nBlocked by: 02\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	content := m.View().Content

	if !strings.Contains(content, "no unblocked tickets") {
		t.Fatalf("expected an actionable cycle error:\n%s", content)
	}
	if !strings.Contains(content, "First") || !strings.Contains(content, "Second") {
		t.Fatalf("expected both cyclic tickets to remain visible for toggling:\n%s", content)
	}
}

// TestQueueModelPollSyncsExecutionTicketsToWidenedRunScope covers ticket 06:
// a ticket added mid-run via "a" (cmdAddToLiveQueue, ralphloop.RunScope.Add)
// must show up in the Queue tab's list/count instead of staying frozen to
// m.executionTickets' kickoff snapshot.
func TestQueueModelPollSyncsExecutionTicketsToWidenedRunScope(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry singleton
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	var epic tickets.Epic
	for _, e := range m.epics {
		if e.Name == "alpha" {
			epic = e
		}
	}

	previousRegistry := ralphLoopRegistry
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	scope, err := ralphloop.ResolveRunScope(epic, []string{"01"})
	if err != nil {
		t.Fatal(err)
	}
	r.setScope("alpha", scope)
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previousRegistry
	})

	m.runningEpics = map[string]bool{"alpha": true}
	m.executionTickets = map[string]bool{"alpha/01": true}
	m.runTicketIDs = map[string][]string{"alpha": {"01"}}

	if done, total := m.checkedProgress(); total != 1 {
		t.Fatalf("precondition: checkedProgress() = %d/%d, want total=1", done, total)
	}

	// Mid-run widening, mirroring cmdAddToLiveQueue's effect on the live scope.
	scope.Add("02")

	// syncExecutionScope runs synchronously inside Update, so the widened
	// ticket is visible without following the returned poll/spinner commands
	// (which would otherwise recurse for as long as "alpha" stays running).
	updated, _ := m.Update(implementPollMsg{epicName: "alpha"})
	m = updated.(QueueModel)

	if !m.executionTickets["alpha/02"] {
		t.Fatalf("executionTickets = %v, want alpha/02 added after the poll observes the widened scope", m.executionTickets)
	}
	if done, total := m.checkedProgress(); total != 2 {
		t.Fatalf("checkedProgress() = %d/%d, want total=2 after the widened ticket is picked up", done, total)
	}
}

func TestQueueModelBannerWhileRunningAggregatesCheckedEpics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-done.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-running.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "beta", "01-running.md", "Status: claimed\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-done.md"):    true,
		ticketPath(root, "alpha", "02-running.md"): true,
		ticketPath(root, "beta", "01-running.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	m.executionStartedAt = now.Add(-time.Hour - 3*time.Minute)
	m.now = func() time.Time { return now }
	m.runningEpics = map[string]bool{"alpha": true, "beta": true}
	m.executionTickets = map[string]bool{"alpha/01": true, "alpha/02": true, "beta/01": true}

	content := m.View().Content
	want := "1 of 3 done"
	if !strings.Contains(content, want) || !strings.Contains(content, "implementing...") {
		t.Fatalf("running title missing %q and/or \"implementing...\":\n%s", want, content)
	}
}

func TestQueueModelBannerWhenCompletedAggregatesLandedTicketMetrics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "beta", "01-third.md", "Status: claimed\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "beta", "01-third.md"):   true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	m.executionStartedAt = now.Add(-time.Hour - 3*time.Minute)
	m.now = func() time.Time { return now }
	m.runningEpics = map[string]bool{"beta": true}
	m.executionTickets = map[string]bool{"alpha/01": true, "alpha/02": true, "beta/01": true}

	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 12000\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: task\nactual_context_window: 7000\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "beta", "01-third.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 5000\n---\n\nBody.\n")

	updated, cmd := m.Update(implementPollMsg{epicName: "beta"})
	m = deliverQueueCommands(t, updated.(QueueModel), cmd)

	content := m.View().Content
	wantTitle := "Queue · done, took 1h03m"
	wantBody := "context windows: total 24.0k tok, avg 8.0k tok, max 12.0k tok"
	if !strings.Contains(content, wantTitle) || !strings.Contains(content, wantBody) {
		done, total := m.completedExecutionProgress()
		t.Fatalf("completion header missing %q and/or %q (completed=%v, progress=%d/%d):\n%s", wantTitle, wantBody, m.executionCompletedAt, done, total, content)
	}
}

// TestQueueHeaderStateMatchesPrototype covers ticket 05's Option B redesign
// (.scratch/tickets-queue-batch3/issues/assets/08-header-prototype.md): the
// title always encodes run state, and the body carries at most one
// state-specific line — one assertion per state.
func TestQueueHeaderStateMatchesPrototype(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	base := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	base.executionTickets = map[string]bool{"alpha/01": true}

	t.Run("not started", func(t *testing.T) {
		m := base
		if got, want := m.queueHeaderTitle(), "Queue"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
		if lines := m.queueHeaderBodyLines(); len(lines) != 0 {
			t.Fatalf("body lines = %v, want none", lines)
		}
	})

	t.Run("running", func(t *testing.T) {
		m := base
		m.runningEpics = map[string]bool{"alpha": true}
		got := m.queueHeaderTitle()
		if !strings.HasPrefix(got, "Queue · 0 of 1 done · ") || !strings.HasSuffix(got, " implementing...") {
			t.Fatalf("title = %q, want \"Queue · 0 of 1 done · <spinner> implementing...\"", got)
		}
		if lines := m.queueHeaderBodyLines(); len(lines) != 0 {
			t.Fatalf("body lines = %v, want none while running", lines)
		}
	})

	t.Run("paused with in-flight iterations", func(t *testing.T) {
		m := base
		m.paused = true
		m.runningEpics = map[string]bool{"alpha": true}
		if got, want := m.queueHeaderTitle(), "Queue · paused (0 of 1 done)"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
		want := []string{"Queue paused — in-flight iterations will finish"}
		if got := m.queueHeaderBodyLines(); !slices.Equal(got, want) {
			t.Fatalf("body lines = %v, want %v", got, want)
		}
	})

	t.Run("paused with nothing in flight", func(t *testing.T) {
		m := base
		m.paused = true
		if lines := m.queueHeaderBodyLines(); len(lines) != 0 {
			t.Fatalf("body lines = %v, want none when runningEpics is empty", lines)
		}
	})

	t.Run("idle but globally paused", func(t *testing.T) {
		m := base
		m.paused = true
		m.executionTickets = map[string]bool{}
		if got, want := m.queueHeaderTitle(), "Queue"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
		if lines := m.queueHeaderBodyLines(); len(lines) != 0 {
			t.Fatalf("body lines = %v, want none for an idle queue", lines)
		}
	})

	t.Run("foreign attach overrides state", func(t *testing.T) {
		m := base
		m.foreignAttachPID = 12345
		if got, want := m.queueHeaderTitle(), "Queue · attached to gx pid 12345"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
	})

	t.Run("no foreign attach uses normal state", func(t *testing.T) {
		m := base
		m.foreignAttachPID = 0
		if got, want := m.queueHeaderTitle(), "Queue"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
	})

	t.Run("completed", func(t *testing.T) {
		completedRoot := t.TempDir()
		writeTicket(t, completedRoot, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
		writeRawQueueTicket(t, completedRoot, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 12000\n---\n\nBody.\n")
		checked := map[string]bool{ticketPath(completedRoot, "alpha", "01-first.md"): true}
		m := loadQueueModel(t, NewQueueModel(completedRoot, ui.Settings{}, checked, keys.Manager{}))
		completedAt := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
		m.executionStartedAt = completedAt.Add(-time.Hour - 3*time.Minute)
		m.executionCompletedAt = completedAt
		m.executionTickets = map[string]bool{"alpha/01": true}

		if got, want := m.queueHeaderTitle(), "Queue · done, took 1h03m"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
		want := []string{"context windows: total 12.0k tok, avg 12.0k tok, max 12.0k tok"}
		if got := m.queueHeaderBodyLines(); !slices.Equal(got, want) {
			t.Fatalf("body lines = %v, want %v", got, want)
		}
	})
}

// TestQueueModelRowsRenderWithNoCheckbox covers ticket 08: the Queue tab is
// read-only for selection, so its rows must not render a checkbox glyph
// (checking/selecting only happens in the Tickets tab).
func TestQueueModelRowsRenderWithNoCheckbox(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "01-first.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", name): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	content := m.View().Content
	icons := ui.Icons(false)
	if strings.Contains(content, icons.CheckboxChecked) || strings.Contains(content, icons.CheckboxUnchecked) {
		t.Fatalf("expected no checkbox glyph in Queue tab rows:\n%s", content)
	}
}

// TestQueueModelClearAllRequiresConfirmation covers ticket 08's "C" keymap:
// pressing it opens a confirmation, and only accepting it clears every
// queued ticket.
func TestQueueModelClearAllRequiresConfirmation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	alpha := ticketPath(root, "alpha", "01-first.md")
	beta := ticketPath(root, "beta", "01-first.md")
	checked := map[string]bool{alpha: true, beta: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatal("expected \"C\" to open a confirmation before clearing")
	}
	if !checked[alpha] || !checked[beta] {
		t.Fatal("expected nothing cleared before the confirmation is accepted")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	if checked[alpha] || checked[beta] {
		t.Fatal("expected accepting the confirmation to clear every queued ticket")
	}
}

// TestQueueModelClearCompleteRequiresConfirmation covers ticket 08's "c"
// keymap: only done tickets (and epics left with nothing visible) are
// cleared, after confirmation.
func TestQueueModelClearCompleteRequiresConfirmation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-open.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-done.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-done.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-done.md", "---\nid: \"02\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "beta", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	open := ticketPath(root, "alpha", "01-open.md")
	alphaDone := ticketPath(root, "alpha", "02-done.md")
	betaDone := ticketPath(root, "beta", "01-done.md")
	checked := map[string]bool{open: true, alphaDone: true, betaDone: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatal("expected \"c\" to open a confirmation before clearing")
	}
	if !checked[alphaDone] || !checked[betaDone] {
		t.Fatal("expected nothing cleared before the confirmation is accepted")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	if checked[alphaDone] || checked[betaDone] {
		t.Fatal("expected accepting the confirmation to clear done tickets")
	}
	if !checked[open] {
		t.Fatal("expected non-done tickets to remain checked")
	}

	content := m.View().Content
	if strings.Contains(content, "beta") {
		t.Fatalf("expected beta epic to disappear once its only ticket cleared:\n%s", content)
	}
}

// TestQueueModelHideCompleteToggleHidesDoneTicketsButKeepsPlanValidation
// covers ticket 09: "tc" hides StatusDone rows from rows()/View() without
// affecting epicWaves' plan validation (which must keep treating the hidden
// ticket as queued), and toggling "tc" again restores it.
func TestQueueModelHideCompleteToggleHidesDoneTicketsButKeepsPlanValidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	first := ticketPath(root, "alpha", "01-first.md")
	second := ticketPath(root, "alpha", "02-second.md")
	checked := map[string]bool{first: true, second: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	if content := m.View().Content; !strings.Contains(content, "First") {
		t.Fatalf("expected done ticket visible by default:\n%s", content)
	}
	if len(m.rows()) != 2 {
		t.Fatalf("expected both tickets in rows() by default, got %d", len(m.rows()))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)

	rows := m.rows()
	if len(rows) != 1 || rows[0].ticket.Path != second {
		t.Fatalf("expected only the non-done ticket in rows() after tc, got %+v", rows)
	}
	content := m.View().Content
	if strings.Contains(content, "First") {
		t.Fatalf("expected done ticket hidden after tc:\n%s", content)
	}
	if !strings.Contains(content, "Second") {
		t.Fatalf("expected non-done ticket still visible after tc:\n%s", content)
	}
	if strings.Contains(content, "no unblocked tickets") {
		t.Fatalf("expected plan validation to still see the hidden-but-queued blocker:\n%s", content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if len(m.rows()) != 2 {
		t.Fatalf("expected tc toggled again to restore both tickets, got %d", len(m.rows()))
	}
}

// TestQueueModelTChordDoesNotCollideWithClearKeymaps covers ticket 09: the
// "t"-prefix chord swallows its second key without triggering "c"'s clear
// behavior, and plain "c"/"C" (with no preceding "t") still open their clear
// confirmations unaffected.
func TestQueueModelTChordDoesNotCollideWithClearKeymaps(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-done.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-open.md", "Status: open\n\nBody.\n")
	done := ticketPath(root, "alpha", "01-done.md")
	open := ticketPath(root, "alpha", "02-open.md")
	checked := map[string]bool{done: true, open: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	if m.queueTree.SelectedIndex() != 0 {
		t.Fatalf("expected initial selection at row 0, got %d", m.queueTree.SelectedIndex())
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(QueueModel)
	if m.queueTree.SelectedIndex() != 1 {
		t.Fatalf("expected \"t\",\"j\" to still move the selection down, got %d", m.queueTree.SelectedIndex())
	}
	if m.hideComplete {
		t.Fatal("expected \"t\" followed by an unrelated key not to toggle hideComplete")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(QueueModel)
	if m.hideComplete {
		t.Fatal("expected \"t\" followed by an unrelated key not to toggle hideComplete")
	}
	if m.confirm.IsOpen {
		t.Fatal("expected \"t\",\"q\" not to open any confirmation")
	}
	if cmd == nil || !nav.IsBack(cmd()) {
		t.Fatal("expected \"t\",\"q\" to still trigger \"q\"'s own nav.Back() action")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatal("expected plain \"c\" (no preceding \"t\") to still open the clear-complete confirmation")
	}
	if m.hideComplete {
		t.Fatal("expected plain \"c\" not to toggle hideComplete")
	}
}

func TestQueueModelIncludesSelectionsAddedAfterLoad(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "01-later.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", name)
	checked := map[string]bool{}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	checked[path] = true
	if content := m.View().Content; !strings.Contains(content, "Later") {
		t.Fatalf("expected cached Queue model to include a later shared selection:\n%s", content)
	}
}

func TestQueueModelEnterChoosesAgentAndStartsOneEpicSubset(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-unchecked.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "beta", "01-other.md"):   true,
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	runOptions := make(chan ralphloop.RunOptions, 1)
	releaseRun := make(chan struct{})
	runReturned := make(chan struct{})
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		runOptions <- opts
		<-releaseRun
		close(runReturned)
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		close(releaseRun)
		select {
		case <-runReturned:
		case <-time.After(time.Second):
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	content := m.View().Content
	if !strings.Contains(content, "Choose the agent") {
		t.Fatalf("expected Enter to open the agent picker:\n%s", content)
	}
	if !strings.Contains(content, "First") {
		t.Fatalf("expected queue panel to remain visible behind the agent picker overlay:\n%s", content)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	if cmd == nil {
		t.Fatal("expected choosing Codex to start execution")
	}
	updated, _ = m.Update(cmd())
	m = updated.(QueueModel)
	if !m.runningEpics["alpha"] {
		t.Fatalf("expected alpha running after kickoff, got %v", m.runningEpics)
	}

	select {
	case opts := <-runOptions:
		if opts.EpicName != "alpha" || opts.Agent != ralphloop.AgentCodex {
			t.Fatalf("unexpected run target: epic=%q agent=%q", opts.EpicName, opts.Agent)
		}
		if strings.Join(opts.TicketIDs, ",") != "01,02" {
			t.Fatalf("expected checked alpha subset [01 02], got %v", opts.TicketIDs)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ralph-loop kickoff")
	}
}

func TestQueueModelEnterStartsFullEligibleSelectionDynamic(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-done.md", "Status: done\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	runOptions := make(chan ralphloop.RunOptions, 1)
	releaseRun := make(chan struct{})
	runReturned := make(chan struct{})
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		runOptions <- opts
		<-releaseRun
		close(runReturned)
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		close(releaseRun)
		select {
		case <-runReturned:
		case <-time.After(time.Second):
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	if cmd == nil {
		t.Fatal("expected choosing Codex to start execution")
	}
	updated, _ = m.Update(cmd())
	m = updated.(QueueModel)

	select {
	case opts := <-runOptions:
		if opts.EpicName != "alpha" || opts.Agent != ralphloop.AgentCodex {
			t.Fatalf("unexpected run target: epic=%q agent=%q", opts.EpicName, opts.Agent)
		}
		if len(opts.TicketIDs) != 0 {
			t.Fatalf("expected dynamic (empty) TicketIDs for full-eligible selection, got %v", opts.TicketIDs)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ralph-loop kickoff")
	}
}

func TestQueueModelSchedulesCheckedEpicsInCheckOrderAndBackfillsAtCap(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "gamma", "01-first.md", "Status: open\n\nBody.\n")
	alpha := ticketPath(root, "alpha", "01-first.md")
	beta := ticketPath(root, "beta", "01-first.md")
	gamma := ticketPath(root, "gamma", "01-first.md")
	checked := map[string]bool{alpha: true, beta: true, gamma: true}
	checkOrder := map[string]uint64{gamma: 1, alpha: 2, beta: 3}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	starts := make(chan string, 3)
	releases := map[string]chan struct{}{
		"alpha": make(chan struct{}),
		"beta":  make(chan struct{}),
		"gamma": make(chan struct{}),
	}
	var mu sync.Mutex
	active, maxActive := 0, 0
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		mu.Lock()
		active++
		maxActive = max(maxActive, active)
		mu.Unlock()
		starts <- opts.EpicName
		<-releases[opts.EpicName]
		mu.Lock()
		active--
		mu.Unlock()
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(2)
	t.Cleanup(func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}, checkOrder))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if first, second := <-starts, <-starts; first != "gamma" || second != "alpha" {
		t.Fatalf("start order = [%s %s], want [gamma alpha]", first, second)
	}
	select {
	case name := <-starts:
		t.Fatalf("epic %q started before a slot was free", name)
	default:
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(QueueModel)
	close(releases["gamma"])
	waitForEpicToFinish(t, "gamma")
	updated, cmd = m.Update(implementPollMsg{epicName: "gamma"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	select {
	case name := <-starts:
		t.Fatalf("epic %q backfilled while the queue was paused", name)
	default:
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	select {
	case name := <-starts:
		if name != "beta" {
			t.Fatalf("backfill epic = %q, want beta", name)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for beta to backfill gamma's slot")
	}
	mu.Lock()
	gotMaxActive := maxActive
	mu.Unlock()
	if gotMaxActive != 2 {
		t.Fatalf("maximum concurrent epics = %d, want 2", gotMaxActive)
	}
}

func TestQueueModelReactivationRecoversTwoConcurrentEpicsFromRegistry(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry singleton
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: claimed\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"): true,
		ticketPath(root, "beta", "01-first.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	r := newLoopRegistry(2)
	r.tryStart("alpha", 0, 1)
	r.tryStart("beta", 0, 1)
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: "01"})
	r.reduceLiveEvent("beta", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: "01"})
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 4000})
	r.reduceLiveEvent("beta", ralphloop.LiveEvent{Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 6000})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		r.finish("beta", nil)
		ralphLoopRegistry = previous
	})

	// This model never received implementStartedMsg for either epic — as if
	// both runs were launched while the Queue tab was backgrounded elsewhere.
	// OnPageActivated must recover both from the registry's snapshots alone.
	updated, _ := m.Update(m.OnPageActivated()())
	m = updated.(QueueModel)

	if !m.runningEpics["alpha"] || !m.runningEpics["beta"] {
		t.Fatalf("expected both epics reflected as running from registry snapshots, got %v", m.runningEpics)
	}
	content := m.View().Content
	if !strings.Contains(content, "0 of 2 done") {
		t.Fatalf("expected aggregated progress across both concurrent epics:\n%s", content)
	}
}

func TestQueueModelReactivationBackfillsPendingEpicAfterMissedCompletion(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	alpha := ticketPath(root, "alpha", "01-first.md")
	beta := ticketPath(root, "beta", "01-first.md")
	checked := map[string]bool{alpha: true, beta: true}
	checkOrder := map[string]uint64{alpha: 1, beta: 2}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	starts := make(chan string, 2)
	releases := map[string]chan struct{}{
		"alpha": make(chan struct{}),
		"beta":  make(chan struct{}),
	}
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		starts <- opts.EpicName
		<-releases[opts.EpicName]
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1) // one slot: beta must wait for alpha
	t.Cleanup(func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}, checkOrder))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if got := <-starts; got != "alpha" {
		t.Fatalf("expected alpha to start first, got %q", got)
	}

	// Simulate the tab being backgrounded from here on: alpha finishes but no
	// implementPollMsg is ever delivered to this model instance (the app
	// shell would drop it since another tab was active).
	close(releases["alpha"])
	waitForEpicToFinish(t, "alpha")

	select {
	case name := <-starts:
		t.Fatalf("epic %q backfilled before reactivation", name)
	default:
	}

	updated, syncCmd := m.Update(m.OnPageActivated()())
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, syncCmd)

	select {
	case name := <-starts:
		if name != "beta" {
			t.Fatalf("backfill epic = %q, want beta", name)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for beta to backfill alpha's slot after reactivation")
	}
	close(releases["beta"])
}

func deliverQueueCommands(t *testing.T, m QueueModel, cmd tea.Cmd) QueueModel {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			m = deliverQueueCommands(t, m, nested)
		}
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(QueueModel)
}

func waitForEpicToFinish(t *testing.T, epicName string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !ralphLoopRegistry.isRunningEpic(epicName) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for epic %q to finish", epicName)
}

func loadQueueModel(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	msg := m.Init()()
	updated, _ := m.Update(msg)
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	return updated.(QueueModel)
}

// selectFirstQueueTicketRow moves the selection to the first nodeQueueTicket
// entry, skipping the per-epic separator/status/context header rows that now
// precede it. Several tests assumed SetSelectedIndex(0)/SelectedIndex()==0
// landed on the first ticket row before buildQueueEntries started emitting
// those headers.
func selectFirstQueueTicketRow(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket {
			m.queueTree.SetSelectedIndex(i)
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

func TestQueueModelPersistsRunningThenDoneStatusThroughStore(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	started := make(chan ralphloop.EventSink, 1)
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, sink ralphloop.EventSink) error {
		started <- sink
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	sink := <-started
	if got := store.Snapshot().Status[path]; got != queueStatusRunning {
		t.Fatalf("status after start = %v, want running", got)
	}

	ticket := m.epics[0].Tickets[0]
	sink.IterationStarted(ticket, "iter-01", "", "")
	sink.IterationFinished(ticket, "alpha", ralphloop.IterationStats{})
	close(release)
	waitForEpicToFinish(t, "alpha")

	updated, cmd = m.Update(implementPollMsg{epicName: "alpha"})
	m = deliverQueueCommands(t, updated.(QueueModel), cmd)

	if got := store.Snapshot().Status[path]; got != queueStatusDone {
		t.Fatalf("status after completion = %v, want done", got)
	}
}

func TestQueueModelPersistsErroredStatusOnFailure(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		<-release
		return errors.New("boom")
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if got := store.Snapshot().Status[path]; got != queueStatusRunning {
		t.Fatalf("status after start = %v, want running", got)
	}

	close(release)
	waitForEpicToFinish(t, "alpha")

	updated, cmd = m.Update(implementPollMsg{epicName: "alpha"})
	m = deliverQueueCommands(t, updated.(QueueModel), cmd)

	if got := store.Snapshot().Status[path]; got != queueStatusErrored {
		t.Fatalf("status after failure = %v, want errored", got)
	}
}

func TestQueueModelPauseDoesNotRewriteRunningStatus(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(QueueModel)
	if !strings.Contains(m.View().Content, "Queue paused") {
		t.Fatalf("expected paused banner:\n%s", m.View().Content)
	}
	if got := store.Snapshot().Status[path]; got != queueStatusRunning {
		t.Fatalf("status while paused = %v, want running (unchanged)", got)
	}

	close(release)
}

func TestQueueModelMidRunSelectionChangeDoesNotRewriteProgressTotals(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry/runRalphLoop singletons
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-later.md", "Status: open\n\nBody.\n")
	alpha1 := ticketPath(root, "alpha", "01-first.md")
	alpha2 := ticketPath(root, "alpha", "02-second.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.SetChecked([]string{alpha1, alpha2}, true); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if done, total := m.checkedProgress(); total != 2 {
		t.Fatalf("progress before selection change = %d/%d, want total 2", done, total)
	}

	beta1 := ticketPath(root, "beta", "01-later.md")
	if err := store.Check(beta1); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)

	if done, total := m.checkedProgress(); total != 2 {
		t.Fatalf("progress after adding a future selection = %d/%d, want total still 2", done, total)
	}

	close(release)
}

// TestQueueModelRestoresAllStatusesAsInitialTab simulates a restart landing
// directly on Queue (ticket 21): a QueueStore pre-populated with every status
// (as a prior process session would have left it) must be fully reflected in
// a freshly constructed QueueModel with no prior Tickets-tab visit.
func TestQueueModelRestoresAllStatusesAsInitialTab(t *testing.T) {
	t.Parallel()
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-pending.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-running.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-done.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "04-errored.md", "Status: open\n\nBody.\n")
	pending := ticketPath(root, "alpha", "01-pending.md")
	running := ticketPath(root, "alpha", "02-running.md")
	done := ticketPath(root, "alpha", "03-done.md")
	errored := ticketPath(root, "alpha", "04-errored.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	for path, status := range map[string]queueItemStatus{
		pending: queueStatusPending,
		running: queueStatusRunning,
		done:    queueStatusDone,
		errored: queueStatusErrored,
	} {
		if err := store.Check(path); err != nil {
			t.Fatal(err)
		}
		if err := store.SetStatus(path, status); err != nil {
			t.Fatal(err)
		}
	}

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))

	for path, want := range map[string]queueItemStatus{
		pending: queueStatusPending,
		running: queueStatusRunning,
		done:    queueStatusDone,
		errored: queueStatusErrored,
	} {
		if !m.checked[path] {
			t.Fatalf("expected %s checked after restart", path)
		}
		if got := m.queueStatus[path]; got != want {
			t.Fatalf("status for %s = %v, want %v", path, got, want)
		}
	}
}

// TestTicketsAndQueueMatchAfterRestartRegardlessOfNavigationOrder covers the
// "identical state in either navigation order" acceptance criterion: both
// tabs read the same QueueStore, so a Tickets-first-then-Queue construction
// and a Queue-first-then-Tickets construction must agree.
func TestTicketsAndQueueMatchAfterRestartRegardlessOfNavigationOrder(t *testing.T) {
	t.Parallel()
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	queuedPath := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(queuedPath); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(queuedPath, queueStatusDone); err != nil {
		t.Fatal(err)
	}
	// checkedPath exercises the independent Tickets-tab checked set (ticket
	// 15's decoupling): it's checked without being queued, so it must not
	// show up in queueFirst.checked (queue membership) at all.
	checkedPath := ticketPath(root, "alpha", "01-first.md") + "-checked-only"
	if err := store.SetTicketChecked([]string{checkedPath}, true); err != nil {
		t.Fatal(err)
	}

	queueFirst := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.New(nil), store))
	ticketsModel := NewModelWithStore(root, ui.Settings{}, keys.New(nil), store)
	ticketsModel = deliverLoad(t, ticketsModel)

	_, ticketsQueued := ticketsModel.queueStatus[queuedPath]
	if queueFirst.checked[queuedPath] != ticketsQueued {
		t.Fatalf("queued mismatch: queue=%v tickets=%v", queueFirst.checked[queuedPath], ticketsQueued)
	}
	if queueFirst.queueStatus[queuedPath] != ticketsModel.queueStatus[queuedPath] {
		t.Fatalf("status mismatch: queue=%v tickets=%v", queueFirst.queueStatus[queuedPath], ticketsModel.queueStatus[queuedPath])
	}
	if queueFirst.checked[checkedPath] {
		t.Fatalf("checked-only ticket must not appear as queued: %#v", queueFirst.checked)
	}
	if !ticketsModel.isChecked(checkedPath) {
		t.Fatalf("expected checked-only ticket to be checked on the Tickets tab")
	}
}

// TestQueueModelShowsSameStatusAsTicketsTab covers ticket 25's first
// request: the Queue tab must show each ticket's status icon, blocked-by
// suffix, and (for a landed ticket) the same elapsed/tokens metrics the
// Tickets tab renders (view.go's renderTicketRow), not just a bare checkbox
// and title.
func TestQueueModelShowsSameStatusAsTicketsTab(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "03-done.md", "---\nid: \"03\"\nstatus: done\ntype: task\nactual_context_window: 12000\nelapsed_time: 754\n---\n\nDone.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-foundation.md"): true,
		ticketPath(root, "alpha", "02-dependent.md"):  true,
		ticketPath(root, "alpha", "03-done.md"):       true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	content := m.View().Content

	if !strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected blocked-by suffix for the dependent ticket:\n%s", content)
	}
	if !strings.Contains(content, "12.0k tok") || !strings.Contains(content, "12m34s") {
		t.Fatalf("expected the done ticket's elapsed/tokens metrics line:\n%s", content)
	}
}

func TestRenderQueueTicketRow_CommitlessSuffix(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open", Commitless: true},
	}}

	line := m.renderQueueTicketRow(queueRow{epic: epic, ticket: epic.Tickets[0]}, 0)
	if !strings.Contains(line, "Open ticket (commitless)") {
		t.Fatalf("title line = %q, want title followed by \" (commitless)\"", line)
	}
}

// TestRenderQueueTicketRow_IconColumnAlignsRegardlessOfChildren mirrors
// TestRenderTicketRow_IconColumnAlignsRegardlessOfChildren (ticket 10) for
// the Queue tab's own row renderer: same-depth siblings' icon columns must
// line up whether or not each one has children, since the triangle column is
// reserved at a fixed width for every row.
func TestRenderQueueTicketRow_IconColumnAlignsRegardlessOfChildren(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Parent ticket", Status: "open"},
		{Identifier: "02", Title: "Leaf ticket", Status: "open"},
	}}

	withChildren := m.renderQueueTicketRow(queueRow{epic: epic, ticket: epic.Tickets[0], hasChildren: true, expanded: true}, 0)
	childless := m.renderQueueTicketRow(queueRow{epic: epic, ticket: epic.Tickets[1]}, 1)

	iconOffset := func(line string) int {
		stripped := ansi.Strip(line)
		return lipgloss.Width(stripped[:strings.Index(stripped, m.icons().TicketOpen)])
	}
	if got, want := iconOffset(withChildren), iconOffset(childless); got != want {
		t.Fatalf("withChildren ticket's icon column = %d, want %d (same as childless sibling)\nchildless: %q\nwithChildren: %q", got, want, childless, withChildren)
	}
	if strings.Contains(ansi.Strip(childless), m.icons().TriangleExpanded) || strings.Contains(ansi.Strip(childless), m.icons().TriangleCollapsed) {
		t.Fatalf("childless row unexpectedly contains a triangle glyph: %q", childless)
	}
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " "))]
}

// TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow covers ticket 10:
// renderLiveTicketRow already prefixes its output with 2 spaces, so the
// caller must not add its own on top or a running row ends up indented 4
// spaces instead of 2.
func TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow(t *testing.T) {
	t.Parallel()
	var m QueueModel
	m.runningEpics = map[string]bool{"epic": true}
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Normal ticket", Status: "open"},
		{Identifier: "02", Title: "Running ticket", Status: "claimed"},
	}}
	m.live = map[string]map[string]liveTicketState{
		"epic": {"02": {running: true, label: "iter-01"}},
	}

	normalLine := m.renderQueueTicketRow(queueRow{epic: epic, ticket: epic.Tickets[0]}, 0)
	liveLine := m.renderQueueTicketRow(queueRow{epic: epic, ticket: epic.Tickets[1]}, 1)

	normalIndent := leadingWhitespace(ansi.Strip(normalLine))
	liveIndent := leadingWhitespace(ansi.Strip(liveLine))
	if liveIndent != normalIndent {
		t.Fatalf("live row indent = %q, want %q (matching normal row): live=%q normal=%q", liveIndent, normalIndent, liveLine, normalLine)
	}
}

func TestRenderQueueTicketRow_DoneMetricsLineMatchesTitleColor(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}

	line := m.renderQueueTicketRow(queueRow{epic: epic, ticket: epic.Tickets[0]}, 0)
	wantSuffix := " " + statusDoneStyle.Italic(true).Render(formatMetricsLine(5, 100, 0))
	if !strings.HasSuffix(line, wantSuffix) {
		t.Fatalf("row line = %q, want it to end with %q", line, wantSuffix)
	}
}

// TestQueueModelScrollsWithKeysAndMouse covers ticket 25's third request: the
// Queue tab must scroll (keyboard, including ctrl+d/ctrl+u, and mouse wheel)
// once its content overflows the visible viewport — previously it had no
// scroll offset at all, so content past the panel's height was simply
// unreachable.
func TestQueueModelScrollsWithKeysAndMouse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 40; i++ {
		writeTicket(t, root, "alpha", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	checked := map[string]bool{}
	for i := 1; i <= 40; i++ {
		checked[ticketPath(root, "alpha", fmt.Sprintf("%02d-ticket.md", i))] = true
	}

	m := NewQueueModel(root, ui.Settings{}, checked, keys.Manager{})
	msg := m.Init()()
	updated, _ := m.Update(msg)
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m = updated.(QueueModel)

	if m.queueTree.ScrollOffset() != 0 {
		t.Fatalf("expected no initial scroll, got offset %d", m.queueTree.ScrollOffset())
	}

	// ctrl+d pages the selection (and viewport) down, twice so there's slack
	// for ctrl+u to give back — a single page-down can land the selection
	// exactly at the viewport's top line, which a lone page-up wouldn't need
	// to scroll further for (it'd already be visible).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	if m.queueTree.ScrollOffset() == 0 {
		t.Fatalf("expected ctrl+d to scroll the viewport down, got offset %d", m.queueTree.ScrollOffset())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	afterPageDown := m.queueTree.ScrollOffset()

	// ctrl+u pages back up — repeated past the top so the selection (and so
	// the viewport, via ensureQueueVisible) actually has to move rather than
	// staying put because the target row was already in view.
	for range 3 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		m = updated.(QueueModel)
	}
	if m.queueTree.SelectedIndex() != 0 {
		t.Fatalf("expected ctrl+u to page the selection back to the top, got %d", m.queueTree.SelectedIndex())
	}
	if m.queueTree.ScrollOffset() >= afterPageDown {
		t.Fatalf("expected ctrl+u to scroll the viewport back up from %d, got %d", afterPageDown, m.queueTree.ScrollOffset())
	}

	// Mouse wheel scrolls the viewport without needing a key press.
	before := m.queueTree.ScrollOffset()
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(QueueModel)
	if m.queueTree.ScrollOffset() <= before {
		t.Fatalf("expected mouse wheel down to increase scroll offset from %d, got %d", before, m.queueTree.ScrollOffset())
	}
}

// TestQueueModelShowsPreviewPaneForSelectedTicket covers ticket 15's core
// acceptance criterion: the Queue tab, previously a single full-width list
// with no preview at all, now shows the same shared preview pane
// (renderTicketPreview) as the Tickets tab for whichever row is selected.
func TestQueueModelShowsPreviewPaneForSelectedTicket(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\nType: task\n\nDistinctive queue-preview body.\n")

	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	m = selectFirstQueueTicketRow(t, m)

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Preview") {
		t.Fatalf("expected a Preview panel title in the Queue tab, got:\n%s", content)
	}
	if !strings.Contains(content, "Distinctive queue-preview body.") {
		t.Fatalf("expected the selected ticket's body rendered in the preview pane, got:\n%s", content)
	}
	if !strings.Contains(content, "Status: open") {
		t.Fatalf("expected prettified frontmatter in the Queue tab's preview pane, got:\n%s", content)
	}
}

// TestQueueModelMouseClickSelectsRowOnly covers ticket 05c: clicking a row
// in the Queue tab must select it (like arrowing there) and do nothing else
// — no confirm modal, no other side effect.
func TestQueueModelMouseClickSelectsRowOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-third.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "alpha", "03-third.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	if m.queueTree.SelectedIndex() != 0 {
		t.Fatalf("expected initial selection at row 0, got %d", m.queueTree.SelectedIndex())
	}

	// Ticket rows sit at whatever entry index the tree gave them, after the
	// epic's separator/status/context header entries; handleQueueMouseClick
	// (queue.go) maps mouse.Y to an entry index via
	// mouse.Y-1-len(queueHeaderBodyLines()), with no per-row Y offsets table
	// anymore now every entry is exactly one line.
	var ticketRows []int
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket {
			ticketRows = append(ticketRows, i)
		}
	}
	if len(ticketRows) != 3 {
		t.Fatalf("expected 3 ticket rows, got %d: %v", len(ticketRows), ticketRows)
	}
	rowY := func(entryIdx int) int {
		return 1 + len(m.queueHeaderBodyLines()) + entryIdx
	}

	// Click on the third row.
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: rowY(ticketRows[2])})
	m = updated.(QueueModel)
	if m.queueTree.SelectedIndex() != ticketRows[2] {
		t.Fatalf("expected click to select row %d, got %d", ticketRows[2], m.queueTree.SelectedIndex())
	}
	if m.confirm.IsOpen {
		t.Fatalf("expected click-to-select to not open the confirm modal")
	}

	// A non-left click must not move the selection.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseRight, Y: rowY(ticketRows[0])})
	m = updated.(QueueModel)
	if m.queueTree.SelectedIndex() != ticketRows[2] {
		t.Fatalf("expected non-left click to leave selection at row %d, got %d", ticketRows[2], m.queueTree.SelectedIndex())
	}

	// A click above the body (row -1) must not crash or change selection.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: 0})
	m = updated.(QueueModel)
	if m.queueTree.SelectedIndex() != ticketRows[2] {
		t.Fatalf("expected out-of-bounds click to leave selection at row %d, got %d", ticketRows[2], m.queueTree.SelectedIndex())
	}
}

// TestQueueModelClickInsidePreviewBoundsFocusesIt covers ticket 01: a click
// landing inside the Queue tab's preview panel bounds hands focus to it,
// mirroring the Tickets tab's TestModel_ClickInsidePreviewBoundsFocusesItAndRoutesWheel
// (mouse_focus_test.go) via the shared previewFocus.clickToFocus helper.
func TestQueueModelClickInsidePreviewBoundsFocusesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	if m.focus != focusSidebar {
		t.Fatalf("expected initial focus on sidebar, got focus=%v", m.focus)
	}

	px, py, _, _ := m.previewRect()
	updated, _ := m.Update(tea.MouseClickMsg{X: px + 1, Y: py + 1, Button: tea.MouseLeft})
	m = updated.(QueueModel)

	if m.focus != focusPreview {
		t.Fatalf("expected preview focus after click inside preview bounds, got focus=%v", m.focus)
	}
}

// TestQueueModelPreviewScrollsPastTruncationPoint covers ticket 11: the
// Queue tab's preview now wraps the shared previewFocus viewport instead of
// truncating to the panel's visible height, so its bottom marker — well past
// where the old truncate-only rendering would have cut off — is reachable by
// scrolling ("b" jumps straight there since the Queue tab has no
// focus-toggle of its own yet, ticket 12).
func TestQueueModelPreviewScrollsPastTruncationPoint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-ticket.md", "Status: open\n\nTOPMARKERXYZ\n\n"+strings.Repeat("Filler line of body text.\n\n", 80)+"BOTTOMMARKERXYZ\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-ticket.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	m = selectFirstQueueTicketRow(t, m)

	initial := ansi.Strip(m.previewVP.View())
	if !strings.Contains(initial, "TOPMARKERXYZ") {
		t.Fatalf("expected top marker visible before scrolling, got:\n%s", initial)
	}
	if strings.Contains(initial, "BOTTOMMARKERXYZ") {
		t.Fatalf("expected bottom marker truncated out of the initial view, got:\n%s", initial)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(QueueModel)

	scrolled := ansi.Strip(m.previewVP.View())
	if !strings.Contains(scrolled, "BOTTOMMARKERXYZ") {
		t.Fatalf("expected 'b' to scroll the preview down to the bottom marker, got:\n%s", scrolled)
	}
}

// TestQueueModelGAndGGJumpSelectionToLastAndFirstRow covers ticket 11's
// "G"/"gg" bindings on the Queue tab, mirroring the Tickets tab's own
// TestModel_GAndGGJumpSidebarSelectionToLastAndFirstRow (preview_test.go).
func TestQueueModelGAndGGJumpSelectionToLastAndFirstRow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-third.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "alpha", "03-third.md"):  true,
	}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(QueueModel)
	last := len(m.queueTree.Entries()) - 1
	if last <= 0 {
		t.Fatalf("expected more than one row in test setup, got %d", last+1)
	}
	if m.queueTree.SelectedIndex() != last {
		t.Fatalf("expected 'G' to select the last row (%d), got %d", last, m.queueTree.SelectedIndex())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(QueueModel)
	if m.queueTree.SelectedIndex() != 0 {
		t.Fatalf("expected 'gg' to select the first row (0), got %d", m.queueTree.SelectedIndex())
	}
}

func ticketPath(root, epic, name string) string {
	return filepath.Join(root, ".scratch", epic, "issues", name)
}

func writeRawQueueTicket(t *testing.T, root, epic, name, content string) {
	t.Helper()
	if err := os.WriteFile(ticketPath(root, epic, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestQueueSearch_SlashEntersInputMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)

	if m.search.Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after '/', got mode=%v", m.search.Mode())
	}
}

func TestQueueSearch_TypedCharactersFilterAndHighlight(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	for _, r := range "/first" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match for %q, got %d", "first", m.search.MatchesCount())
	}

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]
	rows := m.rows()
	var ticketRows []int
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket {
			ticketRows = append(ticketRows, i)
		}
	}
	if len(ticketRows) != 2 {
		t.Fatalf("expected 2 ticket rows, got %d: %v", len(ticketRows), ticketRows)
	}
	matchedLine := m.renderQueueTicketRow(rows[0], ticketRows[0])
	nonMatchedLine := m.renderQueueTicketRow(rows[1], ticketRows[1])
	if strings.Contains(matchedLine, dimPrefix) {
		t.Fatalf("expected matching row undimmed, got: %q", matchedLine)
	}
	if !strings.Contains(nonMatchedLine, dimPrefix) {
		t.Fatalf("expected non-matching row dimmed while searching, got: %q", nonMatchedLine)
	}
}

func TestQueueSearch_EscExitsSearchMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(QueueModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)

	if m.search.Mode() != search.SearchModeNone {
		t.Fatalf("expected esc to fully clear search mode, got %v", m.search.Mode())
	}
	if m.search.HasQuery() {
		t.Fatalf("expected esc to clear the query")
	}
}

func TestQueueSearch_EnterExitsInputButKeepsResults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	for _, r := range "/first" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if m.search.Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after enter")
	}
}

// TestQueueSearch_DigitsTypeIntoQueryNotBoundKeys covers this Queue tab's
// digit-routing counterpart to the Tickets tab fix (ticket 14): while search
// input is active, digit keys must type into the query rather than falling
// through to any of handleQueueKey's own bindings.
func TestQueueSearch_DigitsTypeIntoQueryNotBoundKeys(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = updated.(QueueModel)

	if m.search.Query() != "1" {
		t.Fatalf("expected digit to type into the search query, got query=%q", m.search.Query())
	}
	if !m.InputFocused() {
		t.Fatalf("expected InputFocused()=true while search input is active")
	}
}

func TestQueueEditChordLaunchesEditorOnSelectedTicket(t *testing.T) {
	// not parallel-safe: t.Setenv (EDITOR) is process-wide
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	chords := []struct {
		name   string
		second string
	}{
		{"ee (in place)", "e"},
		{"es (split)", "s"},
		{"ev (vsplit)", "v"},
		{"et (tab)", "t"},
	}

	for _, tt := range chords {
		t.Run(tt.name, func(t *testing.T) {
			m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(QueueModel)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: tt.second})
			m = updated.(QueueModel)
			if cmd == nil {
				t.Fatalf("expected e%s to launch an editor command", tt.second)
			}

			// handleEditFileFinished completes the round trip the same way
			// regardless of how the launch itself resolved (in-place exec,
			// split, or a "not supported" warning for this test's plain
			// terminal setting) — the acceptance criterion is that the
			// finished-edit message routes back into QueueModel without a
			// type-assertion panic.
			updated, _ = m.Update(editFileFinishedMsg{})
			m = updated.(QueueModel)
		})
	}
}

func TestQueueEditChordCancelsOnEsc(t *testing.T) {
	// not parallel-safe: t.Setenv (EDITOR) is process-wide
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)
	if cmd != nil {
		t.Fatalf("expected e-esc to cancel the chord without launching a command")
	}

	// A follow-up "esc" (outside a chord) still navigates back, confirming the
	// cancel didn't leave the chord's prefix stuck mid-sequence.
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)
	if cmd == nil {
		t.Fatalf("expected plain esc after cancel to navigate back")
	}
}
