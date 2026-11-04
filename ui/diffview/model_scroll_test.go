package diffview

import (
	"testing"

	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/search"
)

func TestModelMoveActive(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n@@ -3 +3 @@\n-a\n+b\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	m.SetNavMode(NavModeHunk)

	// Move to next hunk
	moved := m.moveActive(1, false)
	if !moved {
		t.Error("expected MoveActive to return true when moving to next hunk")
	}

	// Move to line mode
	m.SetNavMode(NavModeLine)
	moved = m.moveActive(1, false)
	_ = moved
}

func TestModelScrollPage(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 3)
	m.scrollPage(1)
	m.scrollPage(-1)
	m.scrollPage(0) // no-op
}

func TestModelJumpTopAndBottom(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n@@ -5 +5 @@\n-x\n+y\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 3)

	m.jumpBottom()
	m.jumpTop()
	if m.Data().ActiveHunk != 0 {
		t.Errorf("JumpTop: expected ActiveHunk=0, got %d", m.Data().ActiveHunk)
	}

	m.SetNavMode(NavModeLine)
	m.jumpTop()
	m.jumpBottom()
}

func TestModelJumpTopEmptyHunks(t *testing.T) {
	m := NewModel(false)
	got := m.jumpTop()
	if got {
		t.Error("JumpTop on empty model should return false")
	}
	got = m.jumpBottom()
	if got {
		t.Error("JumpBottom on empty model should return false")
	}
}

func TestModelRestoreViewportYOffset(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 3)
	m.RestoreViewportYOffset(0)
}

func TestModelCurrentSearchMatchIndex(t *testing.T) {
	m := NewModel(false)
	// Empty model — no changed lines
	idx := m.CurrentSearchMatchIndex(nil)
	if idx != -1 {
		t.Errorf("expected -1 for empty model, got %d", idx)
	}
}

func TestModelFocusedLocationAndBody_Empty(t *testing.T) {
	m := NewModel(false)
	_, _, err := m.FocusedLocationAndBody()
	if err == "" {
		t.Error("expected error for empty model with no hunk")
	}
}

func TestModelApplyAndFocusSearchMatch(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	matches := m.ComputeSearchMatches("old")
	if len(matches) > 0 {
		sm := search.Match{DataIndex: matches[0].RawIndex, ViewportRow: matches[0].DisplayIndex}
		m.ApplySearchMatch(sm)
		m.FocusSearchMatch(sm)
	}
}

func TestCoScrollActive_HunkMode(t *testing.T) {
	m := NewModel(false)
	m.navMode = NavModeHunk
	// hunks start at display rows 0, 10, 20
	m.data.HunkDisplayRange = [][2]int{{0, 3}, {10, 13}, {20, 23}}
	m.data.ActiveHunk = 0

	// scroll by 12: target=0+12=12 → nearest is index 1 (display 10, dist=2) vs index 2 (display 20, dist=8)
	m.coScrollActive(12)
	if m.data.ActiveHunk != 1 {
		t.Errorf("expected ActiveHunk=1, got %d", m.data.ActiveHunk)
	}
}

func TestCoScrollActive_LineMode(t *testing.T) {
	m := NewModel(false)
	m.navMode = NavModeLine
	// changed lines at display rows 2, 8, 14
	m.data.ChangedDisplay = []int{2, 8, 14}
	m.data.ActiveLine = 0 // display=2

	// scroll by 7: target=2+7=9 → nearest is index 1 (display 8, dist=1) vs index 2 (display 14, dist=5)
	m.coScrollActive(7)
	if m.data.ActiveLine != 1 {
		t.Errorf("expected ActiveLine=1, got %d", m.data.ActiveLine)
	}
}

func TestCoScrollActive_EmptyHunkRange_NoOp(t *testing.T) {
	m := NewModel(false)
	m.navMode = NavModeHunk
	m.data.HunkDisplayRange = nil
	m.data.ActiveHunk = 0

	m.coScrollActive(7)
	if m.data.ActiveHunk != 0 {
		t.Errorf("expected ActiveHunk=0 (no-op), got %d", m.data.ActiveHunk)
	}
}

// buildScrollTestModel creates a Model with synthetic side-by-side-like data
// for testing ScrollViewport snapping. Lines 0..N-1, viewport height=3.
func buildScrollTestModel(hunkRanges [][2]int, changedDisplay []int, numHunks, numChanged int, navMode NavMode) Model {
	parsed := diffcore.ParsedDiff{}
	for i := 0; i < numHunks; i++ {
		parsed.Hunks = append(parsed.Hunks, diffcore.ParsedHunk{StartLine: i * 4, EndLine: i*4 + 3})
	}
	for i := 0; i < numChanged; i++ {
		parsed.Changed = append(parsed.Changed, diffcore.ChangedLine{LineIndex: i})
	}

	totalLines := 12
	viewLines := make([]string, totalLines)
	for i := range viewLines {
		viewLines[i] = "line"
	}

	m := NewModel(false)
	m.navMode = navMode
	m.data = DiffData{
		Parsed:           parsed,
		ActiveHunk:       0,
		ActiveLine:       0,
		HunkDisplayRange: hunkRanges,
		ChangedDisplay:   changedDisplay,
		ViewLines:        viewLines,
	}
	m.SyncViewport(80, 3) // height=3: shows rows [0,1,2]
	return m
}

func TestScrollViewport_SnapHunkDown(t *testing.T) {
	// Hunk 0 at display rows [0,2], Hunk 1 at display rows [4,6].
	// Active is hunk 0. Scroll down 4 → viewport top becomes 4.
	// Hunk 0 display row 0 < 4 → should snap to hunk 1 (first hunk with display >= 4).
	m := buildScrollTestModel(
		[][2]int{{0, 2}, {4, 6}},
		nil, 2, 0, NavModeHunk,
	)
	m.data.ActiveHunk = 0
	m.ScrollViewport(4)

	if m.data.ActiveHunk != 1 {
		t.Fatalf("ActiveHunk = %d, want 1 (snapped to first visible hunk)", m.data.ActiveHunk)
	}
}

func TestScrollViewport_SnapHunkUp(t *testing.T) {
	// Hunk 0 at [0,2], Hunk 1 at [4,6]. Active is hunk 1.
	// Start with viewport offset=4, scroll up 4 → offset=0.
	// Hunk 1 display row 4 >= bottom (0+3=3) → snap to last hunk visible (hunk 0).
	m := buildScrollTestModel(
		[][2]int{{0, 2}, {4, 6}},
		nil, 2, 0, NavModeHunk,
	)
	m.data.ActiveHunk = 1
	m.viewport.SetYOffset(4)
	m.ScrollViewport(-4)

	if m.data.ActiveHunk != 0 {
		t.Fatalf("ActiveHunk = %d, want 0 (snapped to last visible hunk)", m.data.ActiveHunk)
	}
}

func TestScrollViewport_SnapLineDown(t *testing.T) {
	// Changed lines at display rows 1 and 5.
	// Active is line 0 (display 1). Scroll down 3 → viewport top=3.
	// Display row 1 < 3 → snap to first changed line with display >= 3 (line 1 at display 5).
	m := buildScrollTestModel(
		nil,
		[]int{1, 5}, 0, 2, NavModeLine,
	)
	m.data.ActiveLine = 0
	m.ScrollViewport(3)

	if m.data.ActiveLine != 1 {
		t.Fatalf("ActiveLine = %d, want 1 (snapped to first visible changed line)", m.data.ActiveLine)
	}
}

func TestScrollViewport_SnapLineUp(t *testing.T) {
	// Changed lines at display rows 1 and 5.
	// Active is line 1 (display 5). Start with viewport at offset=5, scroll up 5 → offset=0.
	// Display row 5 >= bottom (0+3=3) → snap to last visible changed line (line 0 at display 1).
	m := buildScrollTestModel(
		nil,
		[]int{1, 5}, 0, 2, NavModeLine,
	)
	m.data.ActiveLine = 1
	m.viewport.SetYOffset(5)
	m.ScrollViewport(-5)

	if m.data.ActiveLine != 0 {
		t.Fatalf("ActiveLine = %d, want 0 (snapped to last visible changed line)", m.data.ActiveLine)
	}
}

func TestScrollViewport_NoSnapWhenStillVisible(t *testing.T) {
	// Hunk 0 at [0,2]. Active is hunk 0. Scroll down 1 → viewport top=1.
	// Hunk 0 display row 0 is now < 1 so it triggers snap... but actually
	// use a scenario where active stays visible: hunk 0 at [2,4], scroll down 1.
	// After scroll viewport top=1, bottom=4. Hunk display row 2 is in [1,4). Stays unchanged.
	m := buildScrollTestModel(
		[][2]int{{2, 4}},
		nil, 1, 0, NavModeHunk,
	)
	m.data.ActiveHunk = 0
	m.ScrollViewport(1)

	if m.data.ActiveHunk != 0 {
		t.Fatalf("ActiveHunk = %d, want 0 (active still visible, no snap)", m.data.ActiveHunk)
	}
}
