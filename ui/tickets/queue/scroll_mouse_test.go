package queue

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/queue/header"
)

// TestQueueModelScrollsWithKeysAndMouse covers ticket 25's third request: the
// Queue tab must scroll (keyboard, including ctrl+d/ctrl+u, and mouse wheel)
// once its content overflows the visible viewport — previously it had no
// scroll offset at all, so content past the panel's height was simply
// unreachable.
func TestQueueModelScrollsWithKeysAndMouse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 40; i++ {
		tickettest.WriteTicket(t, root, "alpha", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	checked := map[string]bool{}
	for i := 1; i <= 40; i++ {
		checked[tickettest.TicketPath(root, "alpha", fmt.Sprintf("%02d-ticket.md", i))] = true
	}

	m := seedFromDisk(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m = updated.(QueueModel)

	if m.rows.Tree.ScrollOffset() != 0 {
		t.Fatalf("expected no initial scroll, got offset %d", m.rows.Tree.ScrollOffset())
	}

	// ctrl+d pages the selection (and viewport) down, twice so there's slack
	// for ctrl+u to give back — a single page-down can land the selection
	// exactly at the viewport's top line, which a lone page-up wouldn't need
	// to scroll further for (it'd already be visible).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	if m.rows.Tree.ScrollOffset() == 0 {
		t.Fatalf("expected ctrl+d to scroll the viewport down, got offset %d", m.rows.Tree.ScrollOffset())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	afterPageDown := m.rows.Tree.ScrollOffset()

	// ctrl+u pages back up — repeated past the top so the selection (and so
	// the viewport, via ensureQueueVisible) actually has to move rather than
	// staying put because the target row was already in view.
	for range 3 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		m = updated.(QueueModel)
	}
	if want := firstSelectableQueueIndex(t, m); m.rows.Tree.SelectedIndex() != want {
		t.Fatalf("expected ctrl+u to page the selection back to the top (%d), got %d", want, m.rows.Tree.SelectedIndex())
	}
	if m.rows.Tree.ScrollOffset() >= afterPageDown {
		t.Fatalf("expected ctrl+u to scroll the viewport back up from %d, got %d", afterPageDown, m.rows.Tree.ScrollOffset())
	}

	// Mouse wheel scrolls the viewport without needing a key press.
	before := m.rows.Tree.ScrollOffset()
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(QueueModel)
	if m.rows.Tree.ScrollOffset() <= before {
		t.Fatalf("expected mouse wheel down to increase scroll offset from %d, got %d", before, m.rows.Tree.ScrollOffset())
	}
}

// queueModelForHoverScroll builds a QueueModel with 40 tickets (so the tree
// overflows a short window) and a long preview body for the first ticket (so
// the preview overflows too), at a window small enough that both panes have
// something to scroll. Non-stacked layout (width > 100) puts the tree at
// x in [0, sidebarW) and the preview at x in [sidebarW+1, width), both
// spanning y in [0, contentHeight) — see previewRect/queueTreeRect.
func queueModelForHoverScroll(t *testing.T) QueueModel {
	t.Helper()
	root := t.TempDir()
	var longBody strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&longBody, "Line %d of a long body.\n", i)
	}
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\n"+longBody.String())
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	for i := 2; i <= 40; i++ {
		filename := fmt.Sprintf("%02d-ticket.md", i)
		tickettest.WriteTicket(t, root, "alpha", filename, "Status: open\n\nBody.\n")
		checked[tickettest.TicketPath(root, "alpha", filename)] = true
	}

	m := seedFromDisk(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 20})
	m = updated.(QueueModel)
	m = selectFirstQueueTicketRow(t, m)

	if m.rows.Tree.ScrollOffset() != 0 || m.preview.VP.YOffset() != 0 {
		t.Fatalf("expected both panes to start unscrolled, got tree=%d preview=%d", m.rows.Tree.ScrollOffset(), m.preview.VP.YOffset())
	}
	return m
}

// TestQueueModelHoverScrollFollowsCursorNotFocus covers ticket 03: the Queue
// tab's tree/preview wheel routing follows the cursor's hover position, not
// m.focus — mirroring the Tickets tab's sidebar/preview pair and ui/commit's
// diff/filetree pair (ticket 01's HoverHitTest helper).
func TestQueueModelHoverScrollFollowsCursorNotFocus(t *testing.T) {
	t.Parallel()

	t.Run("focus pinned to preview, hover over tree scrolls tree", func(t *testing.T) {
		t.Parallel()
		m := queueModelForHoverScroll(t)
		m.preview.Focus = preview.FocusPreview

		updated, _ := m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
		m = updated.(QueueModel)

		if m.rows.Tree.ScrollOffset() == 0 {
			t.Fatalf("expected hovering the tree to scroll it")
		}
		if m.preview.VP.YOffset() != 0 {
			t.Fatalf("expected the preview's scroll offset to stay unchanged, got %d", m.preview.VP.YOffset())
		}
		if m.preview.Focus != preview.FocusPreview {
			t.Fatalf("expected focus to stay on the preview, got %v", m.preview.Focus)
		}
	})

	t.Run("focus pinned to tree, hover over preview scrolls preview", func(t *testing.T) {
		t.Parallel()
		m := queueModelForHoverScroll(t)
		m.preview.Focus = preview.FocusSidebar

		px := m.layout().previewRect().X
		updated, _ := m.Update(tea.MouseWheelMsg{X: px + 1, Y: 1, Button: tea.MouseWheelDown})
		m = updated.(QueueModel)

		if m.preview.VP.YOffset() == 0 {
			t.Fatalf("expected hovering the preview to scroll it")
		}
		if m.rows.Tree.ScrollOffset() != 0 {
			t.Fatalf("expected the tree's scroll offset to stay unchanged, got %d", m.rows.Tree.ScrollOffset())
		}
		if m.preview.Focus != preview.FocusSidebar {
			t.Fatalf("expected focus to stay on the tree, got %v", m.preview.Focus)
		}
	})
}

// TestQueueModelHoverScrollNoOpsWithNoOverflow covers ticket 03's acceptance
// criterion that a pane with nothing to scroll no-ops the wheel rather than
// falling back to the focused pane.
func TestQueueModelHoverScrollNoOpsWithNoOverflow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	m = selectFirstQueueTicketRow(t, m)

	treeBefore := m.rows.Tree.ScrollOffset()
	previewBefore := m.preview.VP.YOffset()

	px := m.layout().previewRect().X
	updated, _ := m.Update(tea.MouseWheelMsg{X: px + 1, Y: 1, Button: tea.MouseWheelDown})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
	m = updated.(QueueModel)

	if m.rows.Tree.ScrollOffset() != treeBefore {
		t.Fatalf("expected tree scroll offset unchanged, got %d want %d", m.rows.Tree.ScrollOffset(), treeBefore)
	}
	if m.preview.VP.YOffset() != previewBefore {
		t.Fatalf("expected preview scroll offset unchanged, got %d want %d", m.preview.VP.YOffset(), previewBefore)
	}
}

// TestQueueModelMouseWheelWhileHelpOpenScrollsHelpNotQueue covers the missing
// help.IsOpen guard on the top-level tea.MouseWheelMsg case: while the help
// modal is open, wheel events must scroll it instead of reaching the queue
// tree behind it.
func TestQueueModelMouseWheelWhileHelpOpenScrollsHelpNotQueue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 40; i++ {
		tickettest.WriteTicket(t, root, "alpha", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	checked := map[string]bool{}
	for i := 1; i <= 40; i++ {
		checked[tickettest.TicketPath(root, "alpha", fmt.Sprintf("%02d-ticket.md", i))] = true
	}

	m := seedFromDisk(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m = updated.(QueueModel)

	m.help.Open(m.width, m.height)

	prevQueueOffset := m.rows.Tree.ScrollOffset()
	prevHelpOffset := m.help.Viewport.YOffset()

	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(QueueModel)

	if m.rows.Tree.ScrollOffset() != prevQueueOffset {
		t.Fatalf("expected queue tree not to scroll while help is open, before=%d after=%d", prevQueueOffset, m.rows.Tree.ScrollOffset())
	}
	if m.help.Viewport.YOffset() <= prevHelpOffset {
		t.Fatalf("expected help viewport to scroll on wheel while open, before=%d after=%d", prevHelpOffset, m.help.Viewport.YOffset())
	}
}

// TestQueueModelShowsPreviewPaneForSelectedTicket covers ticket 15's core
// acceptance criterion: the Queue tab, previously a single full-width list
// with no preview at all, now shows the same shared preview pane
// (renderTicketPreview) as the Tickets tab for whichever row is selected.
func TestQueueModelShowsPreviewPaneForSelectedTicket(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\nType: implement\n\nDistinctive queue-preview body.\n")

	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
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
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "03-third.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
		tickettest.TicketPath(root, "alpha", "03-third.md"):  true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	if want := firstSelectableQueueIndex(t, m); m.rows.Tree.SelectedIndex() != want {
		t.Fatalf("expected initial selection at the first selectable row (%d), got %d", want, m.rows.Tree.SelectedIndex())
	}

	// Ticket rows sit at whatever entry index the tree gave them, after the
	// epic's separator/status/context header entries; handleQueueMouseClick
	// (queue.go) maps mouse.Y to an entry index via
	// mouse.Y-1-len(queueHeaderBodyLines()), with no per-row Y offsets table
	// anymore now every entry is exactly one line.
	var ticketRows []int
	for i, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == NodeTicket {
			ticketRows = append(ticketRows, i)
		}
	}
	if len(ticketRows) != 3 {
		t.Fatalf("expected 3 ticket rows, got %d: %v", len(ticketRows), ticketRows)
	}
	rowY := func(entryIdx int) int {
		return 1 + len(m.header().BodyLines()) + entryIdx
	}

	// Click on the third row.
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: rowY(ticketRows[2])})
	m = updated.(QueueModel)
	if m.rows.Tree.SelectedIndex() != ticketRows[2] {
		t.Fatalf("expected click to select row %d, got %d", ticketRows[2], m.rows.Tree.SelectedIndex())
	}
	if m.confirm.IsOpen {
		t.Fatalf("expected click-to-select to not open the confirm modal")
	}

	// A click above the body (row -1) must not crash or change selection.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: 0})
	m = updated.(QueueModel)
	if m.rows.Tree.SelectedIndex() != ticketRows[2] {
		t.Fatalf("expected out-of-bounds click to leave selection at row %d, got %d", ticketRows[2], m.rows.Tree.SelectedIndex())
	}
}

// TestQueueModelRightClickOpensContextMenu covers ticket 12: a right click on
// a ticket row or epic header selects it and opens the menu "m" opens; on
// another header line or the preview it does nothing.
func TestQueueModelRightClickOpensContextMenu(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	rowOf := map[NodeKind]int{}
	var secondTicket int
	for i, e := range m.rows.Tree.Entries() {
		if _, seen := rowOf[e.Value.Kind]; !seen {
			rowOf[e.Value.Kind] = i
		}
		if e.Value.Kind == NodeTicket {
			secondTicket = i
		}
	}
	rowY := func(entryIdx int) int { return 1 + header.HeaderReservedLines + entryIdx }
	start := m.rows.Tree.SelectedIndex()
	px, py := m.layout().previewRect().X, m.layout().previewRect().Y

	for name, click := range map[string]tea.MouseClickMsg{
		"preview": {Button: tea.MouseRight, X: px, Y: py},
		"context": {Button: tea.MouseRight, Y: rowY(rowOf[NodeEpicStatusLine])},
	} {
		updated, cmd := m.Update(click)
		got := updated.(QueueModel)
		if got.rows.Tree.SelectedIndex() != start || got.menus.Menu.IsOpen || got.menus.Pending != "" || cmd != nil {
			t.Errorf("%s: expected a right click to do nothing, got selection=%d menuOpen=%v pending=%q cmd=%v",
				name, got.rows.Tree.SelectedIndex(), got.menus.Menu.IsOpen, got.menus.Pending, cmd != nil)
		}
	}

	updated, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseRight, Y: rowY(secondTicket)})
	m = updated.(QueueModel)
	if m.rows.Tree.SelectedIndex() != secondTicket {
		t.Fatalf("expected right click to select row %d, got %d", secondTicket, m.rows.Tree.SelectedIndex())
	}
	if want := tickettest.TicketPath(root, "alpha", "02-second.md"); m.menus.Pending != want || cmd == nil {
		t.Fatalf("expected the ticket menu resolving for %q, got pending=%q cmd=%v", want, m.menus.Pending, cmd != nil)
	}

	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseRight, Y: rowY(rowOf[NodeEpicName])})
	m = updated.(QueueModel)
	if m.rows.Tree.SelectedIndex() != rowOf[NodeEpicName] || !m.menus.Menu.IsOpen || !strings.Contains(m.menus.Menu.View(), "Add epic to queue") {
		t.Fatalf("expected the epic menu open on the epic header, got selection=%d view:\n%s", m.rows.Tree.SelectedIndex(), m.menus.Menu.View())
	}
}

// TestQueueMouseClickRowMappingStaysCorrectAcrossBannerTransition covers
// ticket 21: the run-state banner's slot is now a constant height
// (queueHeaderReservedLines), so mouse.Y's mapping to a tree row must stay
// correct across a banner-appear/disappear transition even with no
// intervening resize — the bug the reserved slot fixes.
func TestQueueMouseClickRowMappingStaysCorrectAcrossBannerTransition(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	var ticketRows []int
	for i, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == NodeTicket {
			ticketRows = append(ticketRows, i)
		}
	}
	if len(ticketRows) != 2 {
		t.Fatalf("expected 2 ticket rows, got %d: %v", len(ticketRows), ticketRows)
	}
	rowY := 1 + header.HeaderReservedLines + ticketRows[1]

	// Idle -> running with no WindowSizeMsg in between: the reserved slot's
	// height (and thus SetVisibleHeight/the mouse-click offset) must not
	// change.
	m.server.RunningEpics = map[string]bool{"alpha": true}

	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: rowY})
	m = updated.(QueueModel)
	if m.rows.Tree.SelectedIndex() != ticketRows[1] {
		t.Fatalf("expected click to select row %d after banner transition, got %d", ticketRows[1], m.rows.Tree.SelectedIndex())
	}
}

// TestQueueModelClickInsidePreviewBoundsFocusesIt covers ticket 01: a click
// landing inside the Queue tab's preview panel bounds hands focus to it,
// mirroring the Tickets tab's TestModel_ClickInsidePreviewBoundsFocusesItAndRoutesWheel
// (mouse_focus_test.go) via the shared previewFocus.clickToFocus helper.
func TestQueueModelClickInsidePreviewBoundsFocusesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	if m.preview.Focus != preview.FocusSidebar {
		t.Fatalf("expected initial focus on sidebar, got focus=%v", m.preview.Focus)
	}

	px, py := m.layout().previewRect().X, m.layout().previewRect().Y
	updated, _ := m.Update(tea.MouseClickMsg{X: px + 1, Y: py + 1, Button: tea.MouseLeft})
	m = updated.(QueueModel)

	if m.preview.Focus != preview.FocusPreview {
		t.Fatalf("expected preview focus after click inside preview bounds, got focus=%v", m.preview.Focus)
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
	tickettest.WriteTicket(t, root, "alpha", "01-ticket.md", "Status: open\n\nTOPMARKERXYZ\n\n"+strings.Repeat("Filler line of body text.\n\n", 80)+"BOTTOMMARKERXYZ\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-ticket.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	m = selectFirstQueueTicketRow(t, m)

	initial := ansi.Strip(m.preview.VP.View())
	if !strings.Contains(initial, "TOPMARKERXYZ") {
		t.Fatalf("expected top marker visible before scrolling, got:\n%s", initial)
	}
	if strings.Contains(initial, "BOTTOMMARKERXYZ") {
		t.Fatalf("expected bottom marker truncated out of the initial view, got:\n%s", initial)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(QueueModel)

	scrolled := ansi.Strip(m.preview.VP.View())
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
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "03-third.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
		tickettest.TicketPath(root, "alpha", "03-third.md"):  true,
	}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(QueueModel)
	last := len(m.rows.Tree.Entries()) - 1
	if last <= 0 {
		t.Fatalf("expected more than one row in test setup, got %d", last+1)
	}
	if m.rows.Tree.SelectedIndex() != last {
		t.Fatalf("expected 'G' to select the last row (%d), got %d", last, m.rows.Tree.SelectedIndex())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(QueueModel)
	if want := firstSelectableQueueIndex(t, m); m.rows.Tree.SelectedIndex() != want {
		t.Fatalf("expected 'gg' to select the first selectable row (%d), got %d", want, m.rows.Tree.SelectedIndex())
	}
}

// TestQueueModelCursorSkipsFillerRowsBetweenEpics: moving down off the last
// ticket row of one epic lands on the next epic's header, skipping its blank
// separator row.
func TestQueueModelCursorSkipsFillerRowsBetweenEpics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"): true,
		tickettest.TicketPath(root, "beta", "01-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	var alphaTicket, betaHeader int
	for i, e := range m.rows.Tree.Entries() {
		switch {
		case e.Value.Kind == NodeTicket && e.Value.Epic.Name == "alpha":
			alphaTicket = i
		case e.Value.Kind == NodeEpicName && e.Value.Epic.Name == "beta":
			betaHeader = i
		}
	}
	if betaHeader <= alphaTicket+1 {
		t.Fatalf("expected beta's separator row between alpha's ticket and beta's header, got alpha=%d beta=%d", alphaTicket, betaHeader)
	}
	m.rows.Tree.SetSelectedIndex(alphaTicket)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(QueueModel)
	if m.rows.Tree.SelectedIndex() != betaHeader {
		t.Fatalf("expected 'j' off alpha's ticket to land on beta's header (%d), got %d", betaHeader, m.rows.Tree.SelectedIndex())
	}
}

// TestQueueModelPageDownLandsOnSelectableRow covers ticket 17's paging
// requirement: ctrl+d moves the selection by list.DefaultScroll rows without
// any skip of its own (queue.go's BindingPageDown case), so a page that
// would land squarely on a filler row must be nudged onto the nearest ticket
// row instead.
func TestQueueModelPageDownLandsOnSelectableRow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 4; i++ {
		tickettest.WriteTicket(t, root, "alpha", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	tickettest.WriteTicket(t, root, "beta", "01-ticket.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{}
	for i := 1; i <= 4; i++ {
		checked[tickettest.TicketPath(root, "alpha", fmt.Sprintf("%02d-ticket.md", i))] = true
	}
	checked[tickettest.TicketPath(root, "beta", "01-ticket.md")] = true

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)

	start := firstSelectableQueueIndex(t, m)
	// A 7-row page (list.DefaultScroll) from alpha's header lands on beta's
	// separator — the case this test pins.
	entries := m.rows.Tree.Entries()
	if landing := start + 7; landing >= len(entries) || isSelectableQueueKind(entries[landing].Value.Kind) {
		t.Fatalf("test setup expected ctrl+d's raw landing index (%d) to be a filler row, got %+v", landing, entries)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	entries = m.rows.Tree.Entries()
	sel := m.rows.Tree.SelectedIndex()
	if sel < 0 || sel >= len(entries) || !isSelectableQueueKind(entries[sel].Value.Kind) {
		t.Fatalf("expected ctrl+d to land the selection on a selectable row, got index %d (%+v)", sel, entries)
	}
}

// TestQueueModelClampSelectedSkipsFillerRowAfterRebuild covers ticket 17's
// rebuild requirement: clampSelected's SetEntries re-clamps selection by
// entry count alone, which can leave the cursor sitting on a header row that
// shifted into the previously-selected index once an epic's tickets are
// filtered out from under it (e.g. the "tc" hide-completed toggle) —
// clampSelected must nudge off that row rather than leaving it there.
func TestQueueModelClampSelectedSkipsFillerRowAfterRebuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-done.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-open.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-open.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-done.md"): true,
		tickettest.TicketPath(root, "alpha", "02-open.md"): true,
		tickettest.TicketPath(root, "beta", "01-open.md"):  true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	var openTicketIdx int
	for i, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == NodeTicket && e.Value.Epic.Name == "alpha" && e.Value.Ticket.Ticket.Identifier == "02" {
			openTicketIdx = i
		}
	}
	m.rows.Tree.SetSelectedIndex(openTicketIdx)

	// Toggling hideComplete removes alpha's done ticket (but not its still-open
	// sibling, so alpha's header rows stay) and rebuilds via clampSelected;
	// the removed row shifts beta's header block one slot earlier, landing
	// squarely where alpha's open ticket used to sit only if we'd left the
	// selection unclamped — assert it actually lands on a ticket row.
	m.rows.HideComplete = true
	m.clampSelected()

	entries := m.rows.Tree.Entries()
	sel := m.rows.Tree.SelectedIndex()
	if sel < 0 || sel >= len(entries) || !isSelectableQueueKind(entries[sel].Value.Kind) {
		t.Fatalf("expected rebuild to leave the selection on a selectable row, got index %d (%+v)", sel, entries)
	}
}
