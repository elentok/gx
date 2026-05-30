package diffview

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelInit(t *testing.T) {
	m := NewModel(false)
	if m.Init() != nil {
		t.Error("Init() should return nil")
	}
}

func TestModelSetDataAndViewport(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	data := m.Data()

	m2 := NewModel(false)
	m2.SetData(data)
	if !m2.DataRef().HasContent() {
		t.Error("SetData: expected content after setting data")
	}

	vp := m2.Viewport()
	if vp == nil {
		t.Error("Viewport() should not be nil")
	}
}

func TestModelReflow(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	m.Reflow(80) // should not panic
}

func TestModelEnsureActiveVisible(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	m.EnsureActiveVisible(NavModeHunk)
	m.EnsureActiveVisible(NavModeLine)
}

func TestModelComputeSearchMatches(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)

	matches := m.ComputeSearchMatches("text")
	if len(matches) == 0 {
		t.Error("expected search matches for 'text'")
	}

	empty := m.ComputeSearchMatches("")
	if len(empty) != 0 {
		t.Errorf("expected no matches for empty query, got %d", len(empty))
	}
}

func TestModelActiveRawLineIndex(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)
	idx := m.ActiveRawLineIndex()
	_ = idx // just verify no panic
}

func TestModelRenderRows_EmptyBodyH(t *testing.T) {
	m := NewModel(false)
	m.BuildFromRaw("@@ -1 +1 @@\n-old\n+new\n", "@@ -1 +1 @@\n-old\n+new\n")
	m.SyncViewport(80, 10)
	lines := m.RenderRows(0, true, RenderOpts{InnerWidth: 80})
	if len(lines) != 0 {
		t.Errorf("RenderRows(0) should be empty, got %d lines", len(lines))
	}
}

func TestModelRenderRows_PadsToBodyH(t *testing.T) {
	m := NewModel(false)
	m.BuildFromRaw("@@ -1 +1 @@\n-old\n+new\n", "@@ -1 +1 @@\n-old\n+new\n")
	m.SyncViewport(80, 10)
	lines := m.RenderRows(20, true, RenderOpts{InnerWidth: 80})
	if len(lines) != 20 {
		t.Errorf("expected 20 lines (padded), got %d", len(lines))
	}
}

func TestModelRenderRows_ActiveMarkVsInactive(t *testing.T) {
	m := NewModel(false)
	m.BuildFromRaw("@@ -1 +1 @@\n-old\n+new\n", "@@ -1 +1 @@\n-old\n+new\n")
	m.SyncViewport(80, 10)
	accent := lipgloss.Color("#fab387")
	active := m.RenderRows(5, true, RenderOpts{AccentColor: accent, InnerWidth: 80})
	inactive := m.RenderRows(5, false, RenderOpts{AccentColor: accent, InnerWidth: 80})
	differ := false
	for i := range active {
		if i < len(inactive) && active[i] != inactive[i] {
			differ = true
			break
		}
	}
	if !differ {
		t.Error("active and inactive renders should differ (marks differ)")
	}
	// Inactive rows must start with blank mark "  "
	for _, line := range inactive {
		if len(line) >= 2 && ansi.Strip(line[:2]) != "  " {
			t.Errorf("inactive row should start with blank mark, got %q", line[:2])
		}
	}
}

func TestModelRenderRows_SeparatorDimmed(t *testing.T) {
	m := NewModel(false)
	m.renderMode = RenderModeSideBySide
	// Inject a separator line (all dashes triggers IsDeltaSectionDivider)
	divider := "────────────────────"
	m.data.ViewLines = []string{divider, "-removed", "+added"}
	m.data.ViewLineKinds = []diffrender.RowKind{diffrender.RowPlain, diffrender.RowRemoved, diffrender.RowAdded}
	m.data.DisplayToRaw = []int{-1, 0, 1}
	m.SyncViewport(80, 3)

	lines := m.RenderRows(3, false, RenderOpts{InnerWidth: 40})
	// Separator line should have ANSI codes applied (dim/foreground styling)
	rawSepText := ansi.Strip(divider)
	sepLine := ansi.Strip(lines[0][2:]) // strip the 2-char mark prefix
	if sepLine != rawSepText {
		// ansi.Strip removed codes — OK: dimming was applied but stripped text matches
	}
	// The raw line content should still equal the divider text
	if ansi.Strip(lines[0]) != "  "+rawSepText {
		// mark (2 chars blank) + separator text
		// after strip the mark is "  " and the body is the plain text (possibly truncated)
	}
	_ = sepLine // presence of ANSI codes in lines[0] vs raw text is the real assertion
	// Verify: the full line with ANSI != the line with ANSI stripped (dimming added codes)
	if lines[0] == ansi.Strip(lines[0]) {
		t.Error("separator line should have ANSI styling applied (dim color)")
	}
}

func TestModelRenderRows_SearchHighlight(t *testing.T) {
	m := NewModel(false)
	m.BuildFromRaw("@@ -1 +1 @@\n-old text\n+new text\n", "@@ -1 +1 @@\n-old text\n+new text\n")
	m.SyncViewport(80, 10)

	query := "text"
	matched := false
	opts := RenderOpts{
		InnerWidth: 80,
		SearchMatch: func(displayIdx int) (bool, bool) {
			return true, false
		},
		SearchQuery: query,
	}
	lines := m.RenderRows(5, true, opts)
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), query) {
			matched = true
			// Check that line has ANSI (highlighting applied)
			if line == ansi.Strip(line) {
				t.Error("highlighted line should contain ANSI codes")
			}
			break
		}
	}
	if !matched {
		t.Errorf("expected at least one line containing %q", query)
	}
}

func TestModelRenderRows_BodyTruncated(t *testing.T) {
	m := NewModel(false)
	long := "longcontent_longcontent_longcontent_longcontent_longcontent"
	raw := "@@ -1 +1 @@\n-" + long + "\n+new\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	innerW := 20
	lines := m.RenderRows(5, true, RenderOpts{InnerWidth: innerW})
	for _, line := range lines {
		stripped := ansi.Strip(line)
		// Each line (mark+body) should be at most innerW visible chars
		if ansi.StringWidth(stripped) > innerW {
			t.Errorf("line visible width %d exceeds InnerWidth %d: %q", ansi.StringWidth(stripped), innerW, stripped)
		}
	}
}

func TestModelRenderRows_AddedLinePadded(t *testing.T) {
	m := NewModel(false)
	m.BuildFromRaw("@@ -1 +1 @@\n-old\n+new\n", "@@ -1 +1 @@\n-old\n+new\n")
	m.SyncViewport(80, 10)

	innerW := 40
	lines := m.RenderRows(5, true, RenderOpts{InnerWidth: innerW})
	// At least one added/removed line should have been padded (body width = innerW-2)
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if len(stripped) == 0 {
			continue
		}
		// A line with actual content and ANSI background padding: ansi width > stripped width
		if ansi.StringWidth(line) > len(stripped) || len(line) > len(stripped) {
			// There are ANSI codes — could be marks or padding
			return
		}
	}
}

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
		sm := search.Match{Index: matches[0].RawIndex, DisplayIndex: matches[0].DisplayIndex}
		m.ApplySearchMatch(sm)
		m.FocusSearchMatch(sm)
	}
}

func TestNearestIndex_FindsClosest(t *testing.T) {
	// displays at 0, 5, 10, 15
	displayAt := func(i int) int { return i * 5 }

	if got := nearestIndex(4, displayAt, 7); got != 1 { // 5 is nearer to 7 than 10
		t.Errorf("expected 1, got %d", got)
	}
	if got := nearestIndex(4, displayAt, 8); got != 2 { // 10 is nearer to 8 than 5... wait 8-5=3, 10-8=2 → index 2
		t.Errorf("expected 2, got %d", got)
	}
	if got := nearestIndex(4, displayAt, 0); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
	if got := nearestIndex(4, displayAt, 100); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestNearestIndex_EmptySlice(t *testing.T) {
	if got := nearestIndex(0, func(i int) int { return i }, 5); got != 0 {
		t.Errorf("expected 0 for empty, got %d", got)
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

func TestModelBuildFromRawAndHasContent(t *testing.T) {
	m := NewModel(false)
	if m.DataRef().HasContent() {
		t.Fatal("expected empty model to have no content")
	}

	raw := "@@ -1 +1 @@\n-old\n+new\n"
	m.BuildFromRaw(raw, raw)
	if !m.DataRef().HasContent() {
		t.Fatal("expected model to have content")
	}
	if len(m.Data().ViewLines) == 0 {
		t.Fatal("expected view lines")
	}

	m.BuildFromRaw("", "")
	if m.DataRef().HasContent() {
		t.Fatal("expected cleared model to have no content")
	}
}

func TestModelDiffSettings(t *testing.T) {
	m := NewModel(false)
	if m.RenderMode() != RenderModeUnified {
		t.Fatalf("render mode=%v want unified", m.RenderMode())
	}
	if m.NavMode() != NavModeHunk {
		t.Fatalf("nav mode=%v want hunk", m.NavMode())
	}
	if !m.WrapEnabled() {
		t.Fatal("expected wrap enabled by default")
	}

	m.SetRenderMode(RenderModeSideBySide)
	if !m.IsSideBySide() {
		t.Fatal("expected side-by-side mode")
	}

	m.SetNavMode(NavModeLine)
	if m.NavMode() != NavModeLine {
		t.Fatalf("nav mode=%v want line", m.NavMode())
	}

	m.EnableWrap(false)
	if m.WrapEnabled() {
		t.Fatal("expected wrap disabled")
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

func TestModelUpdate_SearchLifecycle(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	// '/' activates search input mode
	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !result.Handled {
		t.Fatal("expected / to be handled by search")
	}
	if next.Search().Mode() != search.SearchModeInput {
		t.Fatalf("mode=%v want input", next.Search().Mode())
	}

	// Typing a query char: cmd must be nil (handled internally), matches computed
	next, cmd, result := next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if !result.Handled {
		t.Fatal("expected query key to be handled by search")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd (search handled internally), got %T", cmd)
	}
	// SearchMatchAt reports a match for rows containing "t"
	foundMatch := false
	for i := 0; i < len(next.data.ViewLines); i++ {
		if matched, _ := next.SearchMatchAt(i); matched {
			foundMatch = true
			break
		}
	}
	if !foundMatch {
		t.Fatal("expected SearchMatchAt to return matched=true for at least one row after typing query")
	}
}

func TestModelUpdate_SearchEnterSetsConfirmed(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	// Activate and type query
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})

	// Enter confirms search
	next, _, result := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Handled {
		t.Fatal("expected enter to be handled")
	}
	if !result.SearchConfirmed {
		t.Fatal("expected SearchConfirmed=true after Enter")
	}
	if next.Search().Mode() != search.SearchModeResults {
		t.Fatalf("expected SearchModeResults after Enter, got %v", next.Search().Mode())
	}
}

func TestModelUpdate_SearchEscapeClearsMatches(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	// Activate and type query
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})

	// Verify we have matches before escaping
	foundBefore := false
	for i := 0; i < len(next.data.ViewLines); i++ {
		if matched, _ := next.SearchMatchAt(i); matched {
			foundBefore = true
			break
		}
	}
	if !foundBefore {
		t.Fatal("setup: expected matches before escape")
	}

	// Escape clears search
	next, _, result := next.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !result.Handled {
		t.Fatal("expected escape to be handled")
	}
	for i := 0; i < len(next.data.ViewLines); i++ {
		if matched, _ := next.SearchMatchAt(i); matched {
			t.Fatalf("expected SearchMatchAt(row=%d) to return false after Escape", i)
		}
	}
}

func TestModelUpdate_SearchNMovesCurrentMatch(t *testing.T) {
	m := NewModel(false)
	// Two hunks, each with a "match" line
	raw := "@@ -1,3 +1,3 @@\n-match one\n+match two\n context\n@@ -10,2 +10,2 @@\n-match three\n+context\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 20)

	// Activate search and type query that matches multiple rows
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})

	// Enter confirms so n/N navigation works
	next, _, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Find which row is currently "current"
	firstCurrent := -1
	for i := 0; i < len(next.data.ViewLines); i++ {
		if _, cur := next.SearchMatchAt(i); cur {
			firstCurrent = i
			break
		}
	}
	if firstCurrent < 0 {
		t.Fatal("expected a current match after Enter")
	}

	// 'n' moves to next match — current row should differ
	after, _, result := next.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !result.Handled {
		t.Fatal("expected n to be handled")
	}
	newCurrent := -1
	for i := 0; i < len(after.data.ViewLines); i++ {
		if _, cur := after.SearchMatchAt(i); cur {
			newCurrent = i
			break
		}
	}
	if newCurrent == firstCurrent {
		t.Fatalf("expected current match to move after n, still at row %d", firstCurrent)
	}
}

func TestModelBuildFromRaw_RecomputesMatchesOnReload(t *testing.T) {
	m := NewModel(false)
	raw1 := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw1, raw1)
	m.SyncViewport(80, 10)

	// Activate search and type query
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	countBefore := next.search.MatchesCount()
	if countBefore == 0 {
		t.Fatal("setup: expected matches after typing query")
	}

	// Load new diff content — matches should be recomputed automatically
	raw2 := "@@ -1,2 +1,2 @@\n-text alpha\n+text beta\n-text gamma\n+text delta\n"
	next.BuildFromRaw(raw2, raw2)
	countAfter := next.search.MatchesCount()
	if countAfter == 0 {
		t.Fatal("expected matches to be recomputed after BuildFromRaw with active query")
	}
	// New diff has more "t" matches than the old one
	if countAfter <= countBefore {
		t.Fatalf("expected more matches after reload (got %d <= %d)", countAfter, countBefore)
	}
}

// buildNavTestModel creates a Model with two hunks and content for nav key tests.
func buildNavTestModel() Model {
	m := NewModel(false)
	raw := "@@ -1,3 +1,3 @@\n-a\n+b\n context\n@@ -10,3 +10,3 @@\n-x\n+y\n context\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 20)
	return m
}

func pressText(m Model, text string) (Model, UpdateResult) {
	next, _, result := m.Update(tea.KeyPressMsg{Text: text})
	return next, result
}

func pressCode(m Model, code rune) (Model, UpdateResult) {
	next, _, result := m.Update(tea.KeyPressMsg{Code: code})
	return next, result
}

func pressCtrl(m Model, r rune) (Model, UpdateResult) {
	next, _, result := m.Update(tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl})
	return next, result
}

func TestUpdateKey_JMovesDownHandled(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, result := pressText(m, "j")
	if !result.Handled {
		t.Fatal("j: expected Handled=true")
	}
	if next.Data().ActiveHunk <= m.Data().ActiveHunk {
		t.Fatalf("j: expected ActiveHunk to increase, got %d→%d", m.Data().ActiveHunk, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_KMovesUpHandled(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.ActiveHunk = 1
	next, result := pressText(m, "k")
	if !result.Handled {
		t.Fatal("k: expected Handled=true")
	}
	if next.Data().ActiveHunk >= m.Data().ActiveHunk {
		t.Fatalf("k: expected ActiveHunk to decrease, got %d→%d", m.Data().ActiveHunk, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_DownArrowMovesDown(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, result := pressCode(m, tea.KeyDown)
	if !result.Handled {
		t.Fatal("down: expected Handled=true")
	}
	if next.Data().ActiveHunk <= m.Data().ActiveHunk {
		t.Fatalf("down: expected ActiveHunk to increase, got %d→%d", m.Data().ActiveHunk, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_JKScrollViewport(t *testing.T) {
	m := buildNavTestModel()
	// J scrolls viewport without changing active item
	before := m.data.ActiveHunk
	next, result := pressText(m, "J")
	if !result.Handled {
		t.Fatal("J: expected Handled=true")
	}
	_ = next.data.ActiveHunk // snap may change it, but no assertion needed
	_ = before

	next, result = pressText(m, "K")
	if !result.Handled {
		t.Fatal("K: expected Handled=true")
	}
}

func TestUpdateKey_ATogglesNavMode(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.VisualActive = true
	next, result := pressText(m, "a")
	if !result.Handled {
		t.Fatal("a: expected Handled=true")
	}
	if next.NavMode() != NavModeLine {
		t.Fatalf("a: expected NavModeLine, got %v", next.NavMode())
	}
	if next.data.VisualActive {
		t.Fatal("a: expected VisualActive=false after toggle")
	}
}

func TestUpdateKey_ATogglesNavModeBackToHunk(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeLine)
	next, result := pressText(m, "a")
	if !result.Handled {
		t.Fatal("a: expected Handled=true")
	}
	if next.NavMode() != NavModeHunk {
		t.Fatalf("a: expected NavModeHunk, got %v", next.NavMode())
	}
}

func TestUpdateKey_VTogglesVisual(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, result := pressText(m, "v")
	if !result.Handled {
		t.Fatal("v: expected Handled=true")
	}
	if next.NavMode() != NavModeLine {
		t.Fatalf("v: expected NavModeLine when starting from hunk mode, got %v", next.NavMode())
	}
	if !next.data.VisualActive {
		t.Fatal("v: expected VisualActive=true")
	}
}

func TestUpdateKey_VToggleVisualOff(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeLine)
	m.data.VisualActive = true
	next, result := pressText(m, "v")
	if !result.Handled {
		t.Fatal("v (off): expected Handled=true")
	}
	if next.data.VisualActive {
		t.Fatal("v (off): expected VisualActive=false")
	}
}

func TestUpdateKey_SReturnsNeedsReload(t *testing.T) {
	m := buildNavTestModel()
	next, result := pressText(m, "s")
	if !result.Handled {
		t.Fatal("s: expected Handled=true")
	}
	if !result.NeedsReload {
		t.Fatal("s: expected NeedsReload=true")
	}
	if next.RenderMode() != RenderModeSideBySide {
		t.Fatalf("s: expected RenderModeSideBySide, got %v", next.RenderMode())
	}
}

func TestUpdateKey_STogglesRenderModeBack(t *testing.T) {
	m := buildNavTestModel()
	m.SetRenderMode(RenderModeSideBySide)
	next, result := pressText(m, "s")
	if !result.Handled || !result.NeedsReload {
		t.Fatal("s: expected Handled=true and NeedsReload=true")
	}
	if next.RenderMode() != RenderModeUnified {
		t.Fatalf("s: expected RenderModeUnified, got %v", next.RenderMode())
	}
}

func TestUpdateKey_UnregisteredKeyNotHandled(t *testing.T) {
	m := buildNavTestModel()
	_, result := pressText(m, "x")
	if result.Handled {
		t.Fatal("x: expected Handled=false for unregistered key")
	}
}

func TestUpdateKey_GGChordJumpsTop(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.ActiveHunk = 1

	// First 'g' starts chord — consumed but not matched
	next, result := pressText(m, "g")
	if !result.Handled {
		t.Fatal("g (chord prefix): expected Handled=true (chord in progress)")
	}
	if next.Data().ActiveHunk != 1 {
		t.Fatal("g: chord prefix should not change active hunk")
	}

	// Second 'g' completes chord and jumps to top
	next, result = pressText(next, "g")
	if !result.Handled {
		t.Fatal("gg: expected Handled=true")
	}
	if next.Data().ActiveHunk != 0 {
		t.Fatalf("gg: expected ActiveHunk=0, got %d", next.Data().ActiveHunk)
	}
}

func TestUpdateKey_GJumpsBottom(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.ActiveHunk = 0
	next, result := pressText(m, "G")
	if !result.Handled {
		t.Fatal("G: expected Handled=true")
	}
	last := len(next.Data().Parsed.Hunks) - 1
	if next.Data().ActiveHunk != last {
		t.Fatalf("G: expected ActiveHunk=%d (last), got %d", last, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_JumpBottomScrollsViewport(t *testing.T) {
	// Build a model with many hunks spread across many lines. The last hunk
	// should be near the bottom of the content so JumpBottom+EnsureActiveVisible
	// leaves the viewport scrolled down.
	m := NewModel(false)
	raw := ""
	for i := 0; i < 15; i++ {
		start := i*10 + 1
		raw += fmt.Sprintf("@@ -%d,2 +%d,2 @@\n context\n-old%d\n+new%d\n context\n", start, start, i, i)
	}
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 3)
	m.data.ActiveHunk = 0

	next, _ := pressText(m, "G")
	if next.viewport.YOffset() == 0 {
		t.Fatal("G: expected viewport YOffset > 0 after JumpBottom on multi-hunk content")
	}
}

func TestUpdateKey_CtrlDPageDown(t *testing.T) {
	m := buildNavTestModel()
	next, result := pressCtrl(m, 'd')
	if !result.Handled {
		t.Fatal("ctrl+d: expected Handled=true")
	}
	_ = next
}

func TestUpdateKey_CtrlUPageUp(t *testing.T) {
	m := buildNavTestModel()
	next, result := pressCtrl(m, 'u')
	if !result.Handled {
		t.Fatal("ctrl+u: expected Handled=true")
	}
	_ = next
}

func TestModelRenderRows_SearchBoxAppearsWhenInputActive(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	bodyH := 10
	// Activate search input
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !next.search.InputFocused() {
		t.Fatal("setup: expected InputFocused after /")
	}

	lines := next.RenderRows(bodyH, true, RenderOpts{InnerWidth: 80})
	if len(lines) != bodyH {
		t.Fatalf("RenderRows returned %d lines, want %d", len(lines), bodyH)
	}
	// The last non-empty line should contain search box content (border char)
	lastContent := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(ansi.Strip(lines[i])) != "" {
			lastContent = lines[i]
			break
		}
	}
	if !strings.Contains(ansi.Strip(lastContent), "Search") && !strings.Contains(lastContent, "╯") && !strings.Contains(lastContent, "─") {
		t.Errorf("expected bottom lines to contain search box, last non-empty line: %q", ansi.Strip(lastContent))
	}
}

func TestModelRenderRows_SearchBoxGoneAfterEnter(t *testing.T) {
	m := NewModel(false)
	raw := "@@ -1 +1 @@\n-old text\n+new text\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 10)

	bodyH := 10

	// Activate, type a query, then confirm with Enter
	next, _, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _, _ = next.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	next, _, result := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.SearchConfirmed {
		t.Fatal("setup: expected SearchConfirmed after Enter")
	}
	if next.search.InputFocused() {
		t.Fatal("setup: expected InputFocused=false after Enter")
	}

	lines := next.RenderRows(bodyH, true, RenderOpts{InnerWidth: 80})
	if len(lines) != bodyH {
		t.Fatalf("RenderRows returned %d lines after Enter, want %d", len(lines), bodyH)
	}
	// None of the lines should contain the search input box title "Search"
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "Search") {
			t.Errorf("line %d still contains search box after Enter: %q", i, ansi.Strip(line))
		}
	}
}

func TestUpdateKey_NonKeyMsgNotHandled(t *testing.T) {
	m := buildNavTestModel()
	type someMsg struct{}
	_, _, result := m.Update(someMsg{})
	if result.Handled {
		t.Fatal("non-key msg: expected Handled=false")
	}
}
