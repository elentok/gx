package tree

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelUpdate_NavigationAndOpen(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: true},
		{ID: "a", ParentID: "parent", Depth: 1, Value: 1},
		{ID: "b", ParentID: "parent", Depth: 1, Value: 2},
	})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !result.Handled {
		t.Fatal("expected j to be handled")
	}
	if next.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1", next.SelectedIndex())
	}

	next, _, result = next.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if !result.Handled {
		t.Fatal("expected k to be handled")
	}
	if next.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0", next.SelectedIndex())
	}

	next.SetSelectedIndex(1)
	next, _, result = next.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !result.Handled {
		t.Fatal("expected l to be handled")
	}
	if !result.OpenSelected {
		t.Fatal("expected OpenSelected result on childless node")
	}
}

// TestModelUpdate_SkipsUnselectableRows exercises the shared selectability
// hook (SetIsSelectable/SkipUnselectable) that ui/tickets' sidebar migrated
// its bespoke skipUnselectableRow mechanism onto: a value of 0 stands in for
// a decorative row (e.g. a blank separator) that must never hold the cursor.
func TestModelUpdate_SkipsUnselectableRows(t *testing.T) {
	m := NewModel[int]()
	m.SetIsSelectable(func(v int) bool { return v != 0 })
	m.SetEntries([]Entry[int]{
		{ID: "a", Value: 1},
		{ID: "blank", Value: 0},
		{ID: "b", Value: 2},
	})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !result.Handled {
		t.Fatal("expected j to be handled")
	}
	if next.SelectedIndex() != 2 {
		t.Fatalf("selected=%d want=2 (blank row at 1 skipped)", next.SelectedIndex())
	}

	next, _, result = next.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if !result.Handled {
		t.Fatal("expected k to be handled")
	}
	if next.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0 (blank row at 1 skipped)", next.SelectedIndex())
	}
}

// TestSkipUnselectable_BoundaryRetriesOppositeDirection covers the case
// where the selection is already sitting on a non-selectable row and the
// requested direction runs off the end of the entries — SkipUnselectable
// must retry the opposite direction rather than leaving the selection on a
// non-selectable row.
func TestSkipUnselectable_BoundaryRetriesOppositeDirection(t *testing.T) {
	m := NewModel[int]()
	m.SetIsSelectable(func(v int) bool { return v != 0 })
	m.SetEntries([]Entry[int]{
		{ID: "blank", Value: 0},
		{ID: "a", Value: 1},
	})
	m.SetSelectedIndex(0)

	m.SkipUnselectable(-1)
	if m.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1 (retry opposite direction off the top boundary)", m.SelectedIndex())
	}
}

// TestSetSelectedIndex_IgnoresSelectabilityPolicy pins SetSelectedIndex as
// the direct-set path (used by search-jump and click resolution, ticket 17)
// that a selectability policy must never silently relocate — unlike
// SkipUnselectable, it is not skip-aware.
func TestSetSelectedIndex_IgnoresSelectabilityPolicy(t *testing.T) {
	m := NewModel[int]()
	m.SetIsSelectable(func(v int) bool { return v != 0 })
	m.SetEntries([]Entry[int]{
		{ID: "blank", Value: 0},
		{ID: "a", Value: 1},
	})

	m.SetSelectedIndex(0)
	if m.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0 (SetSelectedIndex must not skip the excluded row)", m.SelectedIndex())
	}
}

func TestModelUpdate_ExpandCollapse(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: true},
		{ID: "a", ParentID: "parent", Depth: 1, Value: 1},
	})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected RebuildRequested result for collapse")
	}
	if !next.CollapsedIDs()["parent"] {
		t.Fatal("expected parent to be collapsed")
	}

	next.SetEntries([]Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: false},
	})
	next, _, result = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Handled {
		t.Fatal("expected enter to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected RebuildRequested result for enter toggle")
	}
	if next.CollapsedIDs()["parent"] {
		t.Fatal("expected parent to be expanded")
	}
}

func TestModelUpdate_LeftOnLeafMovesToParent(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: true, Depth: 0},
		{ID: "a", ParentID: "parent", Value: 1, Depth: 1},
	})
	m.SetSelectedIndex(1)

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if next.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0", next.SelectedIndex())
	}
}

func TestModelUpdate_RightOnExpandedNodeMovesToFirstChild(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: true, Depth: 0},
		{ID: "nested", ParentID: "parent", HasChildren: true, Expanded: true, Depth: 1},
		{ID: "a", ParentID: "parent", Value: 1, Depth: 1},
	})
	m.SetSelectedIndex(0)

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !result.Handled {
		t.Fatal("expected l to be handled")
	}
	if !result.SelectionChanged {
		t.Fatal("expected selection change")
	}
	if next.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1", next.SelectedIndex())
	}
}

func TestModelUpdate_LeftOnNestedExpandedNodeCollapsesBeforeParent(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "top", HasChildren: true, Expanded: true},
		{ID: "nested", ParentID: "top", HasChildren: true, Expanded: true, Depth: 1},
		{ID: "a", ParentID: "nested", Value: 1, Depth: 2},
	})
	m.SetSelectedIndex(1) // nested

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected rebuild request (collapse) before parent focus")
	}
	if next.SelectedIndex() != 1 {
		t.Fatalf("expected selection to stay on nested node for collapse, got %d", next.SelectedIndex())
	}
	if !next.CollapsedIDs()["nested"] {
		t.Fatal("expected nested node to be marked collapsed")
	}
}

func TestModelAccessors(t *testing.T) {
	m := NewModel[int]()
	entries := []Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: true},
		{ID: "a", ParentID: "parent", Value: 1},
	}
	m.SetEntries(entries)

	if m.Init() != nil {
		t.Error("Init() should return nil")
	}
	if len(m.Entries()) != 2 {
		t.Errorf("Entries() len=%d want 2", len(m.Entries()))
	}
	if m.ScrollOffset() != 0 {
		t.Errorf("ScrollOffset()=%d want 0", m.ScrollOffset())
	}
	m.SetVisibleHeight(10)
	m.ScrollViewport(1)
	m.ScrollPage(1)

	m.SetCollapsedIDs(map[string]bool{"parent": true})
	if !m.CollapsedIDs()["parent"] {
		t.Error("expected collapsed id after SetCollapsedIDs")
	}
	if m.Keys() == nil {
		t.Error("Keys() should not be nil")
	}
}

func TestModelNodeOperations(t *testing.T) {
	entries := []Entry[int]{
		{ID: "top", HasChildren: true, Expanded: true, Depth: 0},
		{ID: "a", ParentID: "top", Value: 1, Depth: 1},
	}

	t.Run("CollapseSelected", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries(entries)
		m.SetSelectedIndex(0)
		if !m.CollapseSelected() {
			t.Error("expected CollapseSelected=true on expanded node")
		}
	})

	t.Run("ExpandSelected", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries([]Entry[int]{
			{ID: "top", HasChildren: true, Expanded: false, Depth: 0},
		})
		m.SetCollapsedIDs(map[string]bool{"top": true})
		m.SetSelectedIndex(0)
		if !m.ExpandSelected() {
			t.Error("expected ExpandSelected=true on collapsed node")
		}
	})

	t.Run("ToggleSelected_collapsed", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries(entries)
		m.SetSelectedIndex(0)
		if !m.ToggleSelected() {
			t.Error("expected ToggleSelected=true on expanded node")
		}
	})

	t.Run("FocusParent", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries(entries)
		m.SetSelectedIndex(1)
		if !m.FocusParent() {
			t.Error("expected FocusParent=true when on child")
		}
		if m.SelectedIndex() != 0 {
			t.Errorf("expected selection at parent (0), got %d", m.SelectedIndex())
		}
	})

	t.Run("FocusParent_at_root", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries(entries)
		m.SetSelectedIndex(0)
		if m.FocusParent() {
			t.Error("expected FocusParent=false when already at root")
		}
	})
}

func TestModelUpdate_SearchStartAndQueryMsg(t *testing.T) {
	m := NewModel[int]()

	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !result.Handled {
		t.Fatal("expected / to be handled")
	}
	if next.Search().Mode() != search.SearchModeInput {
		t.Fatalf("mode=%v want input", next.Search().Mode())
	}

	next, cmd, result := next.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !result.Handled {
		t.Fatal("expected a to be handled in search input")
	}
	if cmd == nil {
		t.Fatal("expected search query updated cmd")
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

func TestModelSearchHelpers(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "alpha"}, {ID: "beta"}})

	label := func(entry Entry[int]) string { return entry.ID }
	m.ApplyPassiveSearch("beta", label)
	if m.Search().Query() != "beta" {
		t.Fatalf("query=%q want beta", m.Search().Query())
	}
	if m.Search().MatchesCount() != 1 {
		t.Fatalf("matches=%d want 1", m.Search().MatchesCount())
	}
	if !m.FocusCurrentSearchMatch() {
		t.Fatal("expected current search match to move selection")
	}
	if m.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want 1", m.SelectedIndex())
	}
	matched, current := m.SearchMatch(1)
	if !matched || !current {
		t.Fatalf("SearchMatch(1) = (%v, %v), want (true, true)", matched, current)
	}
	matched, current = m.SearchMatch(0)
	if matched || current {
		t.Fatalf("SearchMatch(0) = (%v, %v), want (false, false)", matched, current)
	}
}

func TestRenderLines_VisibleRangeOffset(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	m.SetVisibleHeight(1)
	m.ScrollViewport(1)

	lines := m.RenderLines(3, RenderOpts[int]{EmptyLine: "(empty)", AccentColor: color.White, Label: func(e Entry[int]) string { return e.ID }})
	if len(lines) != 1 {
		t.Fatalf("expected 1 visible line, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "b") {
		t.Fatalf("visible line = %q, want visible entry containing %q", got, "b")
	}
}

func TestRenderLines_EmptyUsesEmptyLine(t *testing.T) {
	m := NewModel[int]()
	lines := m.RenderLines(4, RenderOpts[int]{EmptyLine: "(empty)", AccentColor: color.White})
	if len(lines) != 2 {
		t.Fatalf("expected body height 2, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); got != "(empty)  " {
		t.Fatalf("lines[0] = %q, want %q", got, "(empty)  ")
	}
}

func TestRenderLines_SelectedRowActiveHighlight(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "selected"}})
	lines := m.RenderLines(3, RenderOpts[int]{AccentColor: ui.ColorBlue, Active: true, Label: func(e Entry[int]) string { return e.ID }})
	if len(lines) == 0 || lines[0] == ansi.Strip(lines[0]) {
		t.Fatal("expected ANSI styling on selected active row")
	}
	if got := ansi.Strip(lines[0]); got != "▌selected  " {
		t.Fatalf("stripped line = %q, want %q", got, "▌selected  ")
	}
}

func TestRenderLines_SearchHighlightsCurrentMatch(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "alpha"}})
	label := func(e Entry[int]) string { return e.ID }
	m.ApplyPassiveSearch("pha", label)
	lines := m.RenderLines(3, RenderOpts[int]{AccentColor: color.White, Label: label})
	if len(lines) == 0 || lines[0] == ansi.Strip(lines[0]) {
		t.Fatal("expected search highlight styling")
	}
}

func TestRequiredWidth_UsesRenderedLines(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "wide-name"}})
	label := func(e Entry[int]) string { return e.ID }
	width := m.RequiredWidth(3, RenderOpts[int]{AccentColor: color.White, Label: label})
	if width < len(" wide-name") {
		t.Fatalf("required width too small: %d", width)
	}
}

func TestMoveToAdjacentFile(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "dir", HasChildren: true, Expanded: true},
		{ID: "dir/a.txt", ParentID: "dir", Value: 1},
		{ID: "other", HasChildren: true, Expanded: true},
		{ID: "other/b.txt", ParentID: "other", Value: 2},
	})

	m.SetSelectedIndex(1)
	if ok := m.MoveToAdjacentFile(1); !ok {
		t.Fatal("expected move down to adjacent file")
	}
	if m.SelectedIndex() != 3 {
		t.Fatalf("selected=%d want=3", m.SelectedIndex())
	}
	if ok := m.MoveToAdjacentFile(1); ok {
		t.Fatal("expected no move past last file")
	}
	if ok := m.MoveToAdjacentFile(-1); !ok {
		t.Fatal("expected move up to previous file")
	}
	if m.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1", m.SelectedIndex())
	}
}

func TestNewModel_ExtraBindings(t *testing.T) {
	const bindingToggleStage keys.BindingID = "toggle-stage"
	extra := keys.Binding{ID: bindingToggleStage, Seq: []string{"space"}, Categories: []string{"Status"}, Title: "toggle stage"}

	m := NewModel[int](extra)

	found := false
	for _, b := range m.Keys().Bindings() {
		if b.ID == bindingToggleStage {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected extra binding to appear in Keys().Bindings()")
	}

	match, consumed := m.Keys().Process(tea.KeyPressMsg{Code: ' ', Text: " "})
	if !consumed || match == nil || match.ID != bindingToggleStage {
		t.Fatalf("expected extra binding to be dispatched by Process, got match=%v consumed=%v", match, consumed)
	}
}

func TestNewModel_NoExtraBindingsUnaffected(t *testing.T) {
	m := NewModel[int]()
	if len(m.Keys().Bindings()) != len(treeBindings) {
		t.Fatalf("bindings=%d want=%d", len(m.Keys().Bindings()), len(treeBindings))
	}
}

func TestRenderLines_MultiLineBody_RendersContiguousLines(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "a", Body: []string{"reason line 1", "reason line 2"}},
		{ID: "b"},
	})
	label := func(e Entry[int]) string { return e.ID }
	lines := m.RenderLines(6, RenderOpts[int]{AccentColor: color.White, Label: label})

	if got := ansi.Strip(lines[0]); !strings.Contains(got, "a") {
		t.Fatalf("lines[0] = %q, want entry a's primary line", got)
	}
	if got := ansi.Strip(lines[1]); !strings.Contains(got, "reason line 1") {
		t.Fatalf("lines[1] = %q, want first body line", got)
	}
	if got := ansi.Strip(lines[2]); !strings.Contains(got, "reason line 2") {
		t.Fatalf("lines[2] = %q, want second body line", got)
	}
	if got := ansi.Strip(lines[3]); !strings.Contains(got, "b") {
		t.Fatalf("lines[3] = %q, want entry b right after a's body, got", got)
	}
}

func TestRenderLines_MultiLineBody_SelectionHighlightsAllLines(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "sel", Body: []string{"extra"}}})
	label := func(e Entry[int]) string { return e.ID }
	lines := m.RenderLines(4, RenderOpts[int]{AccentColor: ui.ColorBlue, Active: true, Label: label})

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 rendered lines, got %d", len(lines))
	}
	if lines[0] == ansi.Strip(lines[0]) {
		t.Fatal("expected ANSI selection styling on entry's primary line")
	}
	if lines[1] == ansi.Strip(lines[1]) {
		t.Fatal("expected ANSI selection styling on entry's body line too")
	}
}

// TestSelectAtBodyLine_MixedHeights_ResolvesCorrectEntry covers ticket 28's
// core seam: a click's tree-body-relative line resolves to the entry
// physically occupying it, including a click on a multi-line entry's second
// line, which must select that entry rather than its successor.
func TestSelectAtBodyLine_MixedHeights_ResolvesCorrectEntry(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "a", Body: []string{"a2", "a3"}}, // lines 0-2
		{ID: "b"},                             // line 3
		{ID: "c"},                             // line 4
	})

	m.SelectAtBodyLine(1) // a's second physical line
	if m.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0 (click on a's body line selects a)", m.SelectedIndex())
	}

	m.SelectAtBodyLine(3) // b's line
	if m.SelectedIndex() != 1 {
		t.Fatalf("selected=%d want=1 (click on b's line selects b)", m.SelectedIndex())
	}

	m.SelectAtBodyLine(4) // c's line
	if m.SelectedIndex() != 2 {
		t.Fatalf("selected=%d want=2 (click on c's line selects c)", m.SelectedIndex())
	}
}

// TestSelectAtBodyLine_PastLastRenderedLine_NoOps pins the out-of-content
// case as a designed no-op rather than the incidental clamp the pre-ticket-28
// hand-rolled math produced via list.SetSelected's clamping.
func TestSelectAtBodyLine_PastLastRenderedLine_NoOps(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "a"}, {ID: "b"}})
	m.SetSelectedIndex(0)

	m.SelectAtBodyLine(5) // past the last rendered line
	if m.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0 (click past last line must no-op, not clamp)", m.SelectedIndex())
	}
}

// TestSelectAtBodyLine_RespectsIsSelectable covers the SetIsSelectable
// integration: a click landing on a row the consumer marked unselectable
// must not change the selection.
func TestSelectAtBodyLine_RespectsIsSelectable(t *testing.T) {
	m := NewModel[int]()
	m.SetIsSelectable(func(v int) bool { return v != 0 })
	m.SetEntries([]Entry[int]{
		{ID: "a", Value: 1},
		{ID: "blank", Value: 0},
		{ID: "b", Value: 2},
	})
	m.SetSelectedIndex(0)

	m.SelectAtBodyLine(1) // the unselectable blank row
	if m.SelectedIndex() != 0 {
		t.Fatalf("selected=%d want=0 (click on unselectable row must not change selection)", m.SelectedIndex())
	}
}

// TestVisibleEntries_MixedHeights_ScrollMathUsesPhysicalLines covers the
// ticket's "scroll/paging math correctness for mixed single/multi-line
// trees" seam: a scrolled-past multi-line entry must free up exactly as many
// screen lines as it occupies, not 1.
func TestVisibleEntries_MixedHeights_ScrollMathUsesPhysicalLines(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "a", Body: []string{"a2", "a3"}}, // 3 physical lines
		{ID: "b"},
		{ID: "c"},
	})
	m.SetVisibleHeight(2)
	m.ScrollViewport(3) // scroll past all 3 of a's lines

	label := func(e Entry[int]) string { return e.ID }
	lines := m.RenderLines(4, RenderOpts[int]{AccentColor: color.White, Label: label})
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "b") {
		t.Fatalf("lines[0] = %q, want entry b (a's 3 lines scrolled past)", got)
	}
	if got := ansi.Strip(lines[1]); !strings.Contains(got, "c") {
		t.Fatalf("lines[1] = %q, want entry c", got)
	}
}

func TestAppendScrollbar_LineBasedFits_ShowsWhenBodyOverflowsViewport(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "a", Body: []string{"a2", "a3", "a4"}}, // 4 physical lines
		{ID: "b"},
	})
	label := func(e Entry[int]) string { return e.ID }
	// height=5 -> innerH=3; total physical lines=5 > innerH=3, so despite
	// only 2 entries (which would fit an entry-count-based check), the
	// line-based total must still trigger the scrollbar.
	lines := m.RenderLines(5, RenderOpts[int]{AccentColor: color.White, Width: 20, Label: label})

	found := false
	for _, line := range lines {
		if strings.ContainsAny(ansi.Strip(line), "┃│") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scrollbar glyph when physical lines overflow the viewport, got lines=%v", lines)
	}
}

func TestAppendScrollbar_LineBasedFits_HiddenWhenBodyFitsViewport(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{ID: "a", Body: []string{"a2"}}}) // 2 physical lines
	label := func(e Entry[int]) string { return e.ID }
	// height=4 -> innerH=2, exactly matching the entry's 2 physical lines.
	lines := m.RenderLines(4, RenderOpts[int]{AccentColor: color.White, Width: 20, Label: label})

	for _, line := range lines {
		if strings.ContainsAny(ansi.Strip(line), "┃│") {
			t.Fatalf("expected no scrollbar glyph when content fits, got lines=%v", lines)
		}
	}
}

func TestRenderLines_DepthIndentsRows(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{ID: "parent", HasChildren: true, Expanded: true, Depth: 0},
		{ID: "child", ParentID: "parent", Depth: 1},
	})
	label := func(e Entry[int]) string { return e.ID }
	lines := m.RenderLines(4, RenderOpts[int]{AccentColor: color.White, Label: label})
	if got := ansi.Strip(lines[1]); !strings.HasPrefix(got, "   child") {
		t.Fatalf("child line = %q, want indented prefix", got)
	}
}
