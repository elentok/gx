package filetree

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func TestModelUpdate_NavigationAndOpen(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
		{Kind: EntryFile, Path: "dir/b.txt", ParentPath: "dir", DisplayName: "b.txt", Value: 2},
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
		t.Fatal("expected OpenSelected result on file")
	}
}

func TestModelUpdate_DirExpandCollapse(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
	})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected RebuildRequested result for collapse")
	}
	if !next.CollapsedDirs()["dir"] {
		t.Fatal("expected dir to be collapsed")
	}

	next.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: false},
	})
	next, _, result = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Handled {
		t.Fatal("expected enter to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected RebuildRequested result for enter toggle")
	}
	if next.CollapsedDirs()["dir"] {
		t.Fatal("expected dir to be expanded")
	}
}

func TestModelUpdate_LeftOnFileMovesToParentDir(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true, Depth: 0},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1, Depth: 1},
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

func TestModelUpdate_RightOnExpandedDirMovesToFirstChild(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true, Depth: 0},
		{Kind: EntryDir, Path: "dir/nested", ParentPath: "dir", DisplayName: "nested", Expanded: true, Depth: 1},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1, Depth: 1},
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

func TestModelUpdate_LeftOnNestedExpandedDirCollapsesBeforeParent(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "top", DisplayName: "top", Expanded: true},
		{Kind: EntryDir, Path: "top/nested", ParentPath: "top", DisplayName: "nested", Expanded: true},
		{Kind: EntryFile, Path: "top/nested/a.txt", ParentPath: "top/nested", DisplayName: "a.txt", Value: 1},
	})
	m.SetSelectedIndex(1) // nested dir

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !result.Handled {
		t.Fatal("expected h to be handled")
	}
	if !result.RebuildRequested {
		t.Fatal("expected rebuild request (collapse) before parent focus")
	}
	if next.SelectedIndex() != 1 {
		t.Fatalf("expected selection to stay on nested dir for collapse, got %d", next.SelectedIndex())
	}
	if !next.CollapsedDirs()["top/nested"] {
		t.Fatal("expected nested dir to be marked collapsed")
	}
}

func TestMoveToAdjacentFile(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
		{Kind: EntryDir, Path: "other", DisplayName: "other", Expanded: true},
		{Kind: EntryFile, Path: "other/b.txt", ParentPath: "other", DisplayName: "b.txt", Value: 2},
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

func TestModelAccessors(t *testing.T) {
	m := NewModel[int]()
	entries := []Entry[int]{
		{Kind: EntryDir, Path: "dir", DisplayName: "dir", Expanded: true},
		{Kind: EntryFile, Path: "dir/a.txt", ParentPath: "dir", DisplayName: "a.txt", Value: 1},
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

	m.SetCollapsedDirs(map[string]bool{"dir": true})
	if !m.CollapsedDirs()["dir"] {
		t.Error("expected collapsed dir after SetCollapsedDirs")
	}
	if m.Keys() == nil {
		t.Error("Keys() should not be nil")
	}
}

func TestModelDirOperations(t *testing.T) {
	entries := []Entry[int]{
		{Kind: EntryDir, Path: "top", DisplayName: "top", Expanded: true, Depth: 0},
		{Kind: EntryFile, Path: "top/a.txt", ParentPath: "top", DisplayName: "a.txt", Value: 1, Depth: 1},
	}

	t.Run("CollapseSelectedDir", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries(entries)
		m.SetSelectedIndex(0)
		if !m.CollapseSelectedDir() {
			t.Error("expected CollapseSelectedDir=true on expanded dir")
		}
	})

	t.Run("ExpandSelectedDir", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries([]Entry[int]{
			{Kind: EntryDir, Path: "top", DisplayName: "top", Expanded: false, Depth: 0},
		})
		m.SetCollapsedDirs(map[string]bool{"top": true})
		m.SetSelectedIndex(0)
		if !m.ExpandSelectedDir() {
			t.Error("expected ExpandSelectedDir=true on collapsed dir")
		}
	})

	t.Run("ToggleSelectedDir_collapsed", func(t *testing.T) {
		m := NewModel[int]()
		m.SetEntries(entries)
		m.SetSelectedIndex(0)
		if !m.ToggleSelectedDir() {
			t.Error("expected ToggleSelectedDir=true on expanded dir")
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
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "alpha.go"}, {Kind: EntryFile, DisplayName: "beta.go"}})

	m.ApplyPassiveSearch("beta", func(entry Entry[int]) string { return entry.DisplayName })
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
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "a"}, {Kind: EntryFile, DisplayName: "b"}, {Kind: EntryFile, DisplayName: "c"}})
	m.SetVisibleHeight(1)
	m.ScrollViewport(1)

	lines := m.RenderLines(3, RenderOpts[int]{EmptyLine: "(empty)", AccentColor: color.White})
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

func TestRenderLines_PadsToBodyHeight(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "file.go"}})
	lines := m.RenderLines(5, RenderOpts[int]{AccentColor: color.White})
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestRenderLines_SelectedRowActiveHighlight(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "selected.go"}})
	lines := m.RenderLines(3, RenderOpts[int]{AccentColor: ui.ColorBlue, Active: true})
	if len(lines) == 0 || lines[0] == ansi.Strip(lines[0]) {
		t.Fatal("expected ANSI styling on selected active row")
	}
	if got := ansi.Strip(lines[0]); got != "▌selected.go  " {
		t.Fatalf("stripped line = %q, want %q", got, "▌selected.go  ")
	}
}

func TestRenderLines_SearchHighlightsCurrentMatch(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "alpha.go"}})
	m.ApplyPassiveSearch("pha", func(entry Entry[int]) string { return entry.DisplayName })
	lines := m.RenderLines(3, RenderOpts[int]{
		AccentColor: color.White,
	})
	if len(lines) == 0 || lines[0] == ansi.Strip(lines[0]) {
		t.Fatal("expected search highlight styling")
	}
}

func TestRenderLines_SearchHighlightUsesVisibleLabelText(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "model.go"}})
	m.ApplyPassiveSearch("m", func(entry Entry[int]) string { return entry.DisplayName })

	line := m.RenderLines(3, RenderOpts[int]{AccentColor: color.White})[0]
	got := ansi.Strip(line)
	if got != "▌model.go  " {
		t.Fatalf("stripped line = %q, want %q", got, "▌model.go  ")
	}
	if line == got {
		t.Fatal("expected ANSI styling for highlighted match")
	}
}

func TestRequiredWidth_UsesRenderedLines(t *testing.T) {
	m := NewModel[int]()
	m.SetEntries([]Entry[int]{{Kind: EntryFile, DisplayName: "wide-name.go"}})
	width := m.RequiredWidth(3, RenderOpts[int]{AccentColor: color.White})
	if width < len(" wide-name.go") {
		t.Fatalf("required width too small: %d", width)
	}
}
