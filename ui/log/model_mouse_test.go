package log

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
)

// setupSplitWithDetail opens the Log tab's split view (list + commit-detail)
// on a commit with a long diff and many changed files, wide enough (detail
// pane ≥90 cols) for the commit-detail pane's own diff/filetree sub-regions
// to lay out side by side. The list panel's rows are then swapped for
// synthetic scrollable content, matching this file's other tests' need for a
// list that can actually scroll independent of the real repo's commit count.
func setupSplitWithDetail(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)

	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	// "0scroll.txt" sorts before the file%03d.txt names, so the commit-detail
	// pane's default file selection (selectFirstCommitFile) lands on it,
	// making its long diff the one that's actually visible/scrollable.
	testutil.WriteFile(t, repo, "0scroll.txt", strings.Join(before, "\n")+"\n")
	for i := 1; i <= 40; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("file%03d.txt", i), "original")
	}
	testutil.CommitAll(t, repo, "base")

	testutil.WriteFile(t, repo, "0scroll.txt", strings.Join(after, "\n")+"\n")
	for i := 1; i <= 40; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("file%03d.txt", i), "changed")
	}
	testutil.CommitAll(t, repo, "change")

	m := newTestModelDefault(repo, "", settings)
	m.width = 250
	m.height = 24
	m, _ = m.syncSplitSize()
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	m.listPanel = m.listPanel.WithRows(commitRows(30))
	m = m.withSyncedListSize()

	return m
}

// diffCoords and filetreeCoords return coordinates (in the split view's own
// coordinate space) that land inside the commit-detail pane's diff and
// filetree sub-regions respectively, given the detail pane's origin. They
// rely on ui/commit's FILE_TREE_MAX_WIDTH (45) capping how far right the
// filetree/diff seam can fall.
func diffCoords(m Model) (x, y int) {
	col, row, _ := m.split.DetailOrigin()
	return col + 50, row + 1
}

func filetreeCoords(m Model) (x, y int) {
	col, row, _ := m.split.DetailOrigin()
	return col + 1, row + 1
}

// TestClickOnDetailPaneFocusesItAndRoutesWheelToCommitDetail is a regression
// test for the split's detail pane not being click-focusable: clicking inside
// its bounds should hand it focus via splitview.SetDetailFocused, and a
// subsequent wheel event at the same coordinates should then route through
// commit.Model's own handleMouseWheel instead of scrolling the log list.
func TestClickOnDetailPaneFocusesItAndRoutesWheelToCommitDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(30))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)

	// Enter split mode (mirrors model_split_test.go's approach).
	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.withSyncedListSize()
	m = m.withSyncedDetailSize()
	// esc once to hand focus back to the list, so the click is what moves it.
	m.split = m.split.SetDetailFocused(false)

	if !m.split.IsListFocused() {
		t.Fatal("expected list focused before the click")
	}

	col, row, visible := m.split.DetailOrigin()
	if !visible {
		t.Fatal("expected detail pane visible in split mode")
	}

	prevOffset := m.listPanel.list.Offset()

	next, _ = m.Update(tea.MouseClickMsg{X: col + 1, Y: row, Button: tea.MouseLeft})
	m = next.(Model)

	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after click inside detail bounds")
	}

	next, _ = m.Update(tea.MouseWheelMsg{X: col + 1, Y: row, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() != prevOffset {
		t.Fatalf("expected wheel event to leave the log list's scroll offset untouched (routed to detail instead), got offset=%d want %d", m.listPanel.list.Offset(), prevOffset)
	}
}

// TestWheelOverListScrollsListRegardlessOfFocus proves routing follows the
// cursor, not leftover click-based focus: with keyboard focus pinned to the
// detail pane, a wheel event over the list's coordinates still scrolls the
// list, leaves the detail pane's scroll state untouched, and leaves focus on
// the detail pane.
func TestWheelOverListScrollsListRegardlessOfFocus(t *testing.T) {
	t.Parallel()
	m := setupSplitWithDetail(t)
	m.split = m.split.SetDetailFocused(true)

	prevListOffset := m.listPanel.list.Offset()
	prevDiffOffset := m.commitDetail.DiffScrollOffset()
	prevFiletreeOffset := m.commitDetail.FileTreeScrollOffset()

	next, _ := m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() == prevListOffset {
		t.Fatal("expected wheel over list coordinates to scroll the list")
	}
	if m.commitDetail.DiffScrollOffset() != prevDiffOffset {
		t.Fatal("expected diff scroll state to be untouched by a wheel event over the list")
	}
	if m.commitDetail.FileTreeScrollOffset() != prevFiletreeOffset {
		t.Fatal("expected filetree scroll state to be untouched by a wheel event over the list")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected focus to remain on the detail pane")
	}
}

// TestWheelOverDetailDiffScrollsDiffRegardlessOfFocus is the nested-hover
// case: with keyboard focus pinned to the list, a wheel event over the
// detail pane's diff sub-region scrolls the diff, leaves the list's and the
// filetree's scroll state unchanged, and leaves focus on the list.
func TestWheelOverDetailDiffScrollsDiffRegardlessOfFocus(t *testing.T) {
	t.Parallel()
	m := setupSplitWithDetail(t)
	m.split = m.split.SetDetailFocused(false)

	prevListOffset := m.listPanel.list.Offset()
	prevFiletreeOffset := m.commitDetail.FileTreeScrollOffset()

	x, y := diffCoords(m)
	next, _ := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.commitDetail.DiffScrollOffset() == 0 {
		t.Fatal("expected wheel over the diff sub-region to scroll the diff")
	}
	if m.commitDetail.FileTreeScrollOffset() != prevFiletreeOffset {
		t.Fatal("expected filetree scroll state to be untouched by a wheel event over the diff")
	}
	if m.listPanel.list.Offset() != prevListOffset {
		t.Fatal("expected list scroll state to be untouched by a wheel event over the diff")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected focus to remain on the list")
	}
}

// TestWheelOverDetailFiletreeScrollsFiletreeRegardlessOfFocus mirrors the
// diff case for the filetree sub-region.
func TestWheelOverDetailFiletreeScrollsFiletreeRegardlessOfFocus(t *testing.T) {
	t.Parallel()
	m := setupSplitWithDetail(t)
	m.split = m.split.SetDetailFocused(false)

	prevListOffset := m.listPanel.list.Offset()
	prevDiffOffset := m.commitDetail.DiffScrollOffset()

	x, y := filetreeCoords(m)
	next, _ := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.commitDetail.FileTreeScrollOffset() == 0 {
		t.Fatal("expected wheel over the filetree sub-region to scroll the filetree")
	}
	if m.commitDetail.DiffScrollOffset() != prevDiffOffset {
		t.Fatal("expected diff scroll state to be untouched by a wheel event over the filetree")
	}
	if m.listPanel.list.Offset() != prevListOffset {
		t.Fatal("expected list scroll state to be untouched by a wheel event over the filetree")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected focus to remain on the list")
	}
}

// TestWheelOverRegionWithoutOverflowNoOps covers a region with nothing to
// scroll: the wheel event should leave every scroll state unchanged rather
// than falling back to whichever region is focused.
func TestWheelOverRegionWithoutOverflowNoOps(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(1))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)

	prevOffset := m.listPanel.list.Offset()

	next, _ = m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() != prevOffset {
		t.Fatal("expected wheel over a region with nothing to scroll to leave scroll state unchanged")
	}
}

// TestWheelStillScrollsListWhenDetailNotFocused is a control: without any
// click ever moving focus to the detail pane, a wheel event over the list
// still scrolls the log list.
func TestWheelStillScrollsListWhenDetailNotFocused(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(30))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)

	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.withSyncedListSize()
	m = m.withSyncedDetailSize()
	m.split = m.split.SetDetailFocused(false)

	prevOffset := m.listPanel.list.Offset()

	next, _ = m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() == prevOffset {
		t.Fatal("expected the log list to still scroll on wheel events when the detail pane isn't focused")
	}
}

// TestMouseWheelWhileHelpOpenScrollsHelpNotList is a regression test for
// mouse-wheel events being swallowed while the help modal is open instead of
// scrolling its content.
func TestMouseWheelWhileHelpOpenScrollsHelpNotList(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(30))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	m = next.(Model)
	m.help.Open(m.width, m.height)

	prevListOffset := m.listPanel.list.Offset()
	prevHelpOffset := m.help.Viewport.YOffset()

	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() != prevListOffset {
		t.Fatalf("expected the log list not to scroll while help is open, before=%d after=%d", prevListOffset, m.listPanel.list.Offset())
	}
	if m.help.Viewport.YOffset() <= prevHelpOffset {
		t.Fatalf("expected help viewport to scroll on wheel while open, before=%d after=%d", prevHelpOffset, m.help.Viewport.YOffset())
	}
}
