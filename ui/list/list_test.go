package list_test

import (
	"testing"

	"github.com/elentok/gx/ui/list"
)

func newModel(selected, offset int) *list.Model {
	m := &list.Model{}
	// Use Navigate to set initial state indirectly via exported methods.
	// We set via SetSelected and then force offset via ScrollViewport trick.
	// Instead, just use the zero value and navigate to position.
	_ = selected
	_ = offset
	return m
}

// helper to build a model with specific selected and offset via exported API
func buildModel(selected, offset, total, visibleH int) *list.Model {
	m := &list.Model{}
	// Scroll to the desired offset first
	if offset > 0 {
		m.ScrollViewport(offset, total, visibleH)
	}
	// Then set selection
	m.SetSelected(selected, total)
	// Ensure offset is still correct (SetSelected doesn't change offset)
	// Manually scroll if needed — we use Navigate from 0
	return m
}

// simpleModel builds a model by directly setting via Navigate from 0
func simpleModel(selected int) *list.Model {
	m := &list.Model{}
	m.SetSelected(selected, 1000)
	return m
}

func TestScrollViewport_SnapsSelectionDown(t *testing.T) {
	// Setup: 20 items, visible height 5, selection at 2, offset at 0
	// Scroll down by 5 → offset becomes 5, selection at 2 < 5, snaps to 5
	m := &list.Model{}
	m.SetSelected(2, 20)

	m.ScrollViewport(5, 20, 5)

	if m.Offset() != 5 {
		t.Errorf("expected offset=5, got %d", m.Offset())
	}
	if m.Selected() != 5 {
		t.Errorf("expected selected=5 (snapped to first visible), got %d", m.Selected())
	}
}

func TestScrollViewport_SnapsSelectionUp(t *testing.T) {
	// Setup: 20 items, visible height 5
	// Start at offset=10, selected=14 (last visible)
	// Scroll up by 5 → offset becomes 5, selected=14 >= 5+5=10, snaps to 9
	m := &list.Model{}
	m.ScrollViewport(10, 20, 5) // offset=10, selected snaps to 10
	m.SetSelected(14, 20)       // selection at bottom of viewport

	m.ScrollViewport(-5, 20, 5)

	if m.Offset() != 5 {
		t.Errorf("expected offset=5, got %d", m.Offset())
	}
	if m.Selected() != 9 {
		t.Errorf("expected selected=9 (snapped to last visible), got %d", m.Selected())
	}
}

func TestScrollViewport_LargeDeltaNoPanic(t *testing.T) {
	// Delta larger than total should not panic, just clamp
	m := &list.Model{}
	m.SetSelected(0, 10)

	// Should not panic
	m.ScrollViewport(1000, 10, 5)

	offset := m.Offset()
	selected := m.Selected()
	if offset < 0 || offset > 5 {
		t.Errorf("offset out of bounds: %d", offset)
	}
	if selected < 0 || selected > 9 {
		t.Errorf("selected out of bounds: %d", selected)
	}
}

func TestNavigate_MovesSelectionAndAdjustsOffset(t *testing.T) {
	// 20 items, visible height 5
	// Start at selection=0, offset=0
	// Navigate by 7 → selection=7, offset adjusts to show it (7 >= 0+5, so offset=7-5+1=3)
	m := &list.Model{}

	m.Navigate(7, 20, 5)

	if m.Selected() != 7 {
		t.Errorf("expected selected=7, got %d", m.Selected())
	}
	if m.Offset() != 3 {
		t.Errorf("expected offset=3, got %d", m.Offset())
	}
}

func TestNavigate_ClampsToBounds(t *testing.T) {
	m := &list.Model{}
	m.SetSelected(18, 20)

	// Navigate past end
	m.Navigate(5, 20, 5)

	if m.Selected() != 19 {
		t.Errorf("expected selected=19 (clamped), got %d", m.Selected())
	}
}

func TestEnsureSelectionVisible_AdjustsOffsetMinimally(t *testing.T) {
	// 20 items, visible height 5
	// offset=5, selection=3 (above viewport) → offset should move to 3
	m := &list.Model{}
	m.ScrollViewport(5, 20, 5) // offset=5
	m.SetSelected(3, 20)       // selection above viewport

	m.EnsureSelectionVisible(20, 5)

	if m.Offset() != 3 {
		t.Errorf("expected offset=3 (minimal adjustment), got %d", m.Offset())
	}
}

func TestEnsureSelectionVisible_NoCentering(t *testing.T) {
	// 20 items, visible height 5
	// offset=0, selection=6 (below viewport: 0+5=5) → offset = 6-5+1 = 2
	m := &list.Model{}
	m.SetSelected(6, 20)

	m.EnsureSelectionVisible(20, 5)

	if m.Offset() != 2 {
		t.Errorf("expected offset=2 (not centered), got %d", m.Offset())
	}
}

func TestEdgeCase_TotalZero(t *testing.T) {
	m := &list.Model{}

	// SetSelected with total=0 is a no-op
	m.SetSelected(5, 0)
	if m.Selected() != 0 {
		t.Errorf("expected selected=0 for total=0, got %d", m.Selected())
	}

	// Navigate with total=0 should not panic
	m.Navigate(1, 0, 5)

	// ScrollViewport with total=0 should not panic
	m.ScrollViewport(1, 0, 5)

	// EnsureSelectionVisible with total=0 should not panic
	m.EnsureSelectionVisible(0, 5)

	// VisibleRange with total=0
	start, end := m.VisibleRange(0, 5)
	if start != 0 || end != 0 {
		t.Errorf("expected (0,0) for total=0, got (%d,%d)", start, end)
	}
}

func TestEdgeCase_VisibleHZero(t *testing.T) {
	m := &list.Model{}
	m.SetSelected(5, 20)

	// EnsureSelectionVisible with visibleH=0 should not adjust offset into negative
	m.EnsureSelectionVisible(20, 0)

	// ScrollViewport with visibleH=0 should not panic
	m.ScrollViewport(1, 20, 0)

	// Navigate with visibleH=0 should not panic
	m.Navigate(1, 20, 0)
}

func TestEdgeCase_SingleItem(t *testing.T) {
	m := &list.Model{}

	m.SetSelected(0, 1)
	if m.Selected() != 0 {
		t.Errorf("expected selected=0, got %d", m.Selected())
	}

	m.Navigate(1, 1, 5)
	if m.Selected() != 0 {
		t.Errorf("expected selected=0 (clamped), got %d", m.Selected())
	}

	m.ScrollViewport(5, 1, 5)
	if m.Offset() != 0 {
		t.Errorf("expected offset=0 for single item, got %d", m.Offset())
	}

	start, end := m.VisibleRange(1, 5)
	if start != 0 || end != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", start, end)
	}
}

func TestVisibleRange(t *testing.T) {
	m := &list.Model{}
	m.ScrollViewport(3, 20, 5) // offset=3

	start, end := m.VisibleRange(20, 5)
	if start != 3 {
		t.Errorf("expected start=3, got %d", start)
	}
	if end != 8 {
		t.Errorf("expected end=8, got %d", end)
	}
}

func TestVisibleRange_ClipsAtTotal(t *testing.T) {
	m := &list.Model{}
	m.ScrollViewport(18, 20, 5) // offset clamped to 15

	start, end := m.VisibleRange(20, 5)
	if start != 15 {
		t.Errorf("expected start=15, got %d", start)
	}
	if end != 20 {
		t.Errorf("expected end=20, got %d", end)
	}
}

func TestScrollPage_MovesSelectionAndOffsetTogether(t *testing.T) {
	// 20 items, visible height 5, start at selected=5, offset=5
	m := &list.Model{}
	m.ScrollViewport(5, 20, 5) // offset=5, selection snaps to 5
	m.SetSelected(5, 20)

	m.ScrollPage(7, 20, 5)

	if m.Selected() != 12 {
		t.Errorf("expected selected=12, got %d", m.Selected())
	}
	if m.Offset() != 12 {
		t.Errorf("expected offset=12, got %d", m.Offset())
	}
}

func TestScrollPage_NoOpAtBottomBoundary(t *testing.T) {
	m := &list.Model{}
	m.SetSelected(19, 20)
	m.ScrollViewport(15, 20, 5) // offset=15

	m.ScrollPage(7, 20, 5)

	if m.Selected() != 19 {
		t.Errorf("expected selected=19 (no-op), got %d", m.Selected())
	}
	if m.Offset() != 15 {
		t.Errorf("expected offset=15 (no-op), got %d", m.Offset())
	}
}

func TestScrollPage_NoOpAtTopBoundary(t *testing.T) {
	m := &list.Model{}
	// selected=0, offset=0

	m.ScrollPage(-7, 20, 5)

	if m.Selected() != 0 {
		t.Errorf("expected selected=0 (no-op), got %d", m.Selected())
	}
	if m.Offset() != 0 {
		t.Errorf("expected offset=0 (no-op), got %d", m.Offset())
	}
}

func TestScrollPage_ClampsNearBoundary(t *testing.T) {
	// 20 items, selected=16, offset=12, visible=5 → max offset=15
	// ScrollPage(7): newSelected=23→19, newOffset=19→15
	m := &list.Model{}
	m.ScrollViewport(12, 20, 5)
	m.SetSelected(16, 20)

	m.ScrollPage(7, 20, 5)

	if m.Selected() != 19 {
		t.Errorf("expected selected=19 (clamped), got %d", m.Selected())
	}
	if m.Offset() != 15 {
		t.Errorf("expected offset=15 (clamped), got %d", m.Offset())
	}
}

func TestScrollPage_NegativeDelta(t *testing.T) {
	// 20 items, selected=10, offset=10, visible=5
	// ScrollPage(-7): newSelected=3, newOffset=3
	m := &list.Model{}
	m.ScrollViewport(10, 20, 5)
	m.SetSelected(10, 20)

	m.ScrollPage(-7, 20, 5)

	if m.Selected() != 3 {
		t.Errorf("expected selected=3, got %d", m.Selected())
	}
	if m.Offset() != 3 {
		t.Errorf("expected offset=3, got %d", m.Offset())
	}
}

// uniformHeight is the "opt-in default": every existing consumer's call
// sites never build a real LineHeight, so the *Lines methods must reduce to
// exactly today's index-space math when fed this.
func uniformHeight(int) int { return 1 }

// --- Regression suite: *Lines methods with a uniform 1-line height must be
// byte-for-byte identical to their non-Lines siblings. ---

func TestVisibleRangeLines_UniformHeight_MatchesVisibleRange(t *testing.T) {
	m1 := &list.Model{}
	m1.ScrollViewport(3, 20, 5)
	wantStart, wantEnd := m1.VisibleRange(20, 5)

	m2 := &list.Model{}
	m2.ScrollViewport(3, 20, 5)
	gotStart, gotEnd := m2.VisibleRangeLines(20, uniformHeight, 5)

	if gotStart != wantStart || gotEnd != wantEnd {
		t.Errorf("VisibleRangeLines(uniform) = (%d,%d), want (%d,%d)", gotStart, gotEnd, wantStart, wantEnd)
	}
}

func TestEnsureSelectionVisibleLines_UniformHeight_MatchesEnsureSelectionVisible(t *testing.T) {
	m1 := &list.Model{}
	m1.ScrollViewport(5, 20, 5)
	m1.SetSelected(3, 20)
	m1.EnsureSelectionVisible(20, 5)

	m2 := &list.Model{}
	m2.ScrollViewport(5, 20, 5)
	m2.SetSelected(3, 20)
	m2.EnsureSelectionVisibleLines(20, uniformHeight, 5)

	if m1.Offset() != m2.Offset() {
		t.Errorf("EnsureSelectionVisibleLines(uniform) offset = %d, want %d", m2.Offset(), m1.Offset())
	}
}

func TestNavigateLines_UniformHeight_MatchesNavigate(t *testing.T) {
	m1 := &list.Model{}
	m1.Navigate(7, 20, 5)

	m2 := &list.Model{}
	m2.NavigateLines(7, 20, uniformHeight, 5)

	if m1.Selected() != m2.Selected() || m1.Offset() != m2.Offset() {
		t.Errorf("NavigateLines(uniform) = (sel=%d,off=%d), want (sel=%d,off=%d)",
			m2.Selected(), m2.Offset(), m1.Selected(), m1.Offset())
	}
}

func TestScrollPageLines_UniformHeight_MatchesScrollPage(t *testing.T) {
	m1 := &list.Model{}
	m1.ScrollViewport(5, 20, 5)
	m1.SetSelected(5, 20)
	m1.ScrollPage(7, 20, 5)

	m2 := &list.Model{}
	m2.ScrollViewport(5, 20, 5)
	m2.SetSelected(5, 20)
	m2.ScrollPageLines(1, 20, uniformHeight, 7)

	if m1.Selected() != m2.Selected() || m1.Offset() != m2.Offset() {
		t.Errorf("ScrollPageLines(uniform) = (sel=%d,off=%d), want (sel=%d,off=%d)",
			m2.Selected(), m2.Offset(), m1.Selected(), m1.Offset())
	}
}

func TestScrollPageLines_UniformHeight_MatchesScrollPage_Negative(t *testing.T) {
	m1 := &list.Model{}
	m1.ScrollViewport(10, 20, 5)
	m1.SetSelected(10, 20)
	m1.ScrollPage(-7, 20, 5)

	m2 := &list.Model{}
	m2.ScrollViewport(10, 20, 5)
	m2.SetSelected(10, 20)
	m2.ScrollPageLines(-1, 20, uniformHeight, 7)

	if m1.Selected() != m2.Selected() || m1.Offset() != m2.Offset() {
		t.Errorf("ScrollPageLines(uniform) = (sel=%d,off=%d), want (sel=%d,off=%d)",
			m2.Selected(), m2.Offset(), m1.Selected(), m1.Offset())
	}
}

func TestScrollOffsetOnlyLines_UniformHeight_MatchesScrollOffsetOnly(t *testing.T) {
	m1 := &list.Model{}
	m1.ScrollOffsetOnly(5, 20, 5)

	m2 := &list.Model{}
	m2.ScrollOffsetOnlyLines(5, 20, uniformHeight, 5)

	if m1.Offset() != m2.Offset() {
		t.Errorf("ScrollOffsetOnlyLines(uniform) offset = %d, want %d", m2.Offset(), m1.Offset())
	}
}

func TestScrollViewportLines_UniformHeight_MatchesScrollViewport(t *testing.T) {
	m1 := &list.Model{}
	m1.SetSelected(14, 20)
	m1.ScrollViewport(10, 20, 5)
	m1.ScrollViewport(-5, 20, 5)

	m2 := &list.Model{}
	m2.SetSelected(14, 20)
	m2.ScrollViewportLines(10, 20, uniformHeight, 5)
	m2.ScrollViewportLines(-5, 20, uniformHeight, 5)

	if m1.Selected() != m2.Selected() || m1.Offset() != m2.Offset() {
		t.Errorf("ScrollViewportLines(uniform) = (sel=%d,off=%d), want (sel=%d,off=%d)",
			m2.Selected(), m2.Offset(), m1.Selected(), m1.Offset())
	}
}

func TestItemAtLine_UniformHeight_MatchesOffsetPlusBodyLine(t *testing.T) {
	for _, bodyLine := range []int{0, 1, 4, 5} {
		got := list.ItemAtLine(3, 20, uniformHeight, bodyLine)
		want := 3 + bodyLine
		if bodyLine >= 20-3 {
			want = -1
		}
		if got != want {
			t.Errorf("ItemAtLine(offset=3, bodyLine=%d) = %d, want %d", bodyLine, got, want)
		}
	}
}

// --- Line-aware behavior with variable per-item line counts. ---

// twoLineEven gives even-indexed items 2 lines and odd-indexed items 1 line.
func twoLineEven(i int) int {
	if i%2 == 0 {
		return 2
	}
	return 1
}

func TestVisibleRangeLines_VariableHeight(t *testing.T) {
	// Items: 0:2, 1:1, 2:2, 3:1, 4:2, ... budget=5 lines from offset 0
	// 0(2)+1(1)+2(2)=5 lines exactly -> items [0,3)
	m := &list.Model{}
	start, end := m.VisibleRangeLines(20, twoLineEven, 5)
	if start != 0 || end != 3 {
		t.Errorf("expected (0,3), got (%d,%d)", start, end)
	}
}

func TestEnsureSelectionVisibleLines_KeepsMultiLineItemFullyVisible(t *testing.T) {
	// offset=0, budget=3 lines. Selecting item 2 (2 lines) after item 0(2)+1(1)
	// would need lines [0,5) which exceeds budget 3, so offset must move so
	// that item 2's full 2-line span plus whatever fits before it stays <= 3.
	m := &list.Model{}
	m.SetSelected(2, 20)

	m.EnsureSelectionVisibleLines(20, twoLineEven, 3)

	// Walking back from item 2 (span=2): item 1 (1 line) -> span=3, fits.
	// item 0 (2 lines) -> span=5, doesn't fit. So offset lands on 1.
	if m.Offset() != 1 {
		t.Errorf("expected offset=1, got %d", m.Offset())
	}
	start, end := m.VisibleRangeLines(20, twoLineEven, 3)
	if !(m.Selected() >= start && m.Selected() < end) {
		t.Errorf("selected item %d not within visible range [%d,%d)", m.Selected(), start, end)
	}
}

func TestScrollPageLines_PagesByLineBudgetNotItemCount(t *testing.T) {
	// All items 2 lines, budget=6 lines -> should page by 3 items, not 6.
	twoLine := func(int) int { return 2 }
	m := &list.Model{}
	m.SetSelected(0, 20)

	m.ScrollPageLines(1, 20, twoLine, 6)

	if m.Selected() != 3 {
		t.Errorf("expected selected=3 (3 items x 2 lines = 6 line budget), got %d", m.Selected())
	}
}

func TestScrollPageLines_SingleOverTallItemStillPagesByOne(t *testing.T) {
	// Item 0 is 100 lines tall (exceeds the whole budget) -> paging still
	// advances by 1 item rather than getting stuck.
	overTall := func(i int) int {
		if i == 0 {
			return 100
		}
		return 1
	}
	m := &list.Model{}
	m.SetSelected(0, 20)

	m.ScrollPageLines(1, 20, overTall, 5)

	if m.Selected() != 1 {
		t.Errorf("expected selected=1 (progress past over-tall item), got %d", m.Selected())
	}
}

func TestScrollViewportLines_WheelStepConvertsLinesToItems(t *testing.T) {
	// All items 2 lines, wheel delta of 6 lines should move offset by 3 items,
	// not 6.
	twoLine := func(int) int { return 2 }
	m := &list.Model{}

	m.ScrollViewportLines(6, 20, twoLine, 6)

	if m.Offset() != 3 {
		t.Errorf("expected offset=3 (6 lines / 2-line items), got %d", m.Offset())
	}
}

func TestScrollViewportLines_SelectionSnapClampLandsOnFullyVisibleItem(t *testing.T) {
	// Budget=5 lines, items alternate 2/1 lines starting at offset. After
	// scrolling, selection above the new offset must snap to a fully visible
	// item, not a clipped one.
	m := &list.Model{}
	m.SetSelected(0, 20)

	m.ScrollViewportLines(6, 20, twoLineEven, 5)

	start, end := m.VisibleRangeLines(20, twoLineEven, 5)
	if m.Selected() < start || m.Selected() >= end {
		t.Errorf("selected %d snapped outside visible range [%d,%d)", m.Selected(), start, end)
	}
}

func TestOverTallItem_NoInfiniteLoopEitherDirection(t *testing.T) {
	overTall := func(i int) int { return 50 }
	m := &list.Model{}
	m.SetSelected(5, 10)

	// Should terminate promptly in both directions with an over-tall budget
	// smaller than every item.
	m.EnsureSelectionVisibleLines(10, overTall, 3)
	m.ScrollPageLines(1, 10, overTall, 3)
	m.ScrollPageLines(-1, 10, overTall, 3)
	m.ScrollViewportLines(3, 10, overTall, 3)
	m.ScrollViewportLines(-3, 10, overTall, 3)

	if m.Selected() < 0 || m.Selected() > 9 {
		t.Errorf("selected out of bounds: %d", m.Selected())
	}
	if m.Offset() < 0 || m.Offset() > 9 {
		t.Errorf("offset out of bounds: %d", m.Offset())
	}
}

func TestItemAtLine_VariableHeight(t *testing.T) {
	// offset=0: item0 lines[0,2), item1 line[2,3), item2 lines[3,5)
	cases := map[int]int{0: 0, 1: 0, 2: 1, 3: 2, 4: 2}
	for bodyLine, want := range cases {
		got := list.ItemAtLine(0, 20, twoLineEven, bodyLine)
		if got != want {
			t.Errorf("ItemAtLine(bodyLine=%d) = %d, want %d", bodyLine, got, want)
		}
	}
}

func TestItemAtLine_BelowLastItemReturnsNegativeOne(t *testing.T) {
	got := list.ItemAtLine(0, 2, twoLineEven, 100)
	if got != -1 {
		t.Errorf("expected -1, got %d", got)
	}
}
