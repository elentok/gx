package diffview

import (
	"testing"

	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelBuildFromRawAndHasContent(t *testing.T) {
	m := NewModel()
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
	m := NewModel()
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

	m := NewModel()
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
	m := NewModel()

	next, _, handled := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !handled {
		t.Fatal("expected / to be handled by search")
	}
	if next.Search().Mode() != search.SearchModeInput {
		t.Fatalf("mode=%v want input", next.Search().Mode())
	}

	next, cmd, handled := next.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !handled {
		t.Fatal("expected query key to be handled by search")
	}
	if cmd == nil {
		t.Fatal("expected search batch cmd")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("unexpected cmd msg type %T", msg)
	}

	found := false
	for _, batchCmd := range batch {
		if queryMsg, ok := batchCmd().(search.SearchQueryUpdatedMsg); ok {
			if queryMsg.Query != "a" {
				t.Fatalf("query=%q want=a", queryMsg.Query)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected SearchQueryUpdatedMsg in batch")
	}
}
