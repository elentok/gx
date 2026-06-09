package help

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func sectionN(title string, n int) KeySection {
	bs := make([]keys.Binding, n)
	for i := range bs {
		bs[i] = keys.Binding{Seq: []string{string(rune('a' + i))}, Title: title + "-row"}
	}
	return KeySection{Title: title, Bindings: bs}
}

func TestColumnCount_RespondsToWidth(t *testing.T) {
	cases := []struct {
		width, want int
	}{
		{0, 1},
		{27, 1},
		{40, 1},
		{56, 2},
		{84, 3},
		{112, 4},
		{400, 4}, // capped at maxColumns
	}
	for _, c := range cases {
		if got := columnCount(c.width); got != c.want {
			t.Errorf("columnCount(%d)=%d want %d", c.width, got, c.want)
		}
	}
}

func TestPackColumns_KeepsSectionsWhole(t *testing.T) {
	sections := []KeySection{
		sectionN("A", 3),
		sectionN("B", 3),
		sectionN("C", 3),
	}
	cols := packColumns(sections, 3)

	// Every original section appears exactly once, intact.
	seen := map[string]int{}
	for _, col := range cols {
		for _, s := range col {
			seen[s.Title]++
			orig := sections[s.Title[0]-'A']
			if len(s.Bindings) != len(orig.Bindings) {
				t.Errorf("section %s was split: %d bindings want %d", s.Title, len(s.Bindings), len(orig.Bindings))
			}
		}
	}
	for _, title := range []string{"A", "B", "C"} {
		if seen[title] != 1 {
			t.Errorf("section %s appeared %d times, want 1", title, seen[title])
		}
	}
}

func TestPackColumns_ColumnMajorOrder(t *testing.T) {
	// Six equal sections into 3 columns → 2 per column, in registration order
	// down column 1, then column 2, then column 3.
	sections := []KeySection{
		sectionN("A", 2), sectionN("B", 2), sectionN("C", 2),
		sectionN("D", 2), sectionN("E", 2), sectionN("F", 2),
	}
	cols := packColumns(sections, 3)
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	want := [][]string{{"A", "B"}, {"C", "D"}, {"E", "F"}}
	for ci, col := range cols {
		for si, s := range col {
			if s.Title != want[ci][si] {
				t.Errorf("col %d row %d = %s want %s", ci, si, s.Title, want[ci][si])
			}
		}
	}
}

func TestPackColumns_FillsAllColumns(t *testing.T) {
	// One tall section among several must not swallow a column's budget and leave
	// the last column empty — break-before-overshoot fills all `cols` columns when
	// there are at least `cols` sections.
	sections := []KeySection{
		sectionN("A", 5),
		sectionN("Tall", 11),
		sectionN("C", 5),
		sectionN("D", 5),
		sectionN("E", 1),
		sectionN("F", 3),
	}
	cols := packColumns(sections, 4)
	if len(cols) != 4 {
		t.Fatalf("expected all 4 columns filled, got %d: %v", len(cols), cols)
	}
	for i, col := range cols {
		if len(col) == 0 {
			t.Errorf("column %d is empty", i)
		}
	}
}

func TestPackColumns_SingleColumnKeepsAll(t *testing.T) {
	sections := []KeySection{sectionN("A", 2), sectionN("B", 2)}
	cols := packColumns(sections, 1)
	if len(cols) != 1 {
		t.Fatalf("expected 1 column, got %d", len(cols))
	}
	if len(cols[0]) != 2 {
		t.Errorf("expected 2 sections in the single column, got %d", len(cols[0]))
	}
}

func TestRenderColumns_NarrowIsSingleColumn(t *testing.T) {
	sections := []KeySection{sectionN("App", 2), sectionN("Nav", 2)}
	// Narrow width → 1 column → headings stack vertically (App appears before Nav
	// on earlier lines, never side-by-side).
	out := ansi.Strip(RenderColumns(sections, 40, ""))
	lines := strings.Split(out, "\n")
	appLine, navLine := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "App") {
			appLine = i
		}
		if strings.Contains(l, "Nav") {
			navLine = i
		}
	}
	if appLine == -1 || navLine == -1 {
		t.Fatalf("missing headings in output: %q", out)
	}
	if appLine == navLine {
		t.Errorf("expected App and Nav on separate lines at narrow width, both on line %d", appLine)
	}
}

func TestHighlightMatch_WrapsMatchOnly(t *testing.T) {
	base := ui.StyleHint
	out := highlightMatch("robot", "bo", base)
	// The plain text is preserved.
	if got := ansi.Strip(out); got != "robot" {
		t.Errorf("stripped = %q, want 'robot'", got)
	}
	// The matched substring is rendered with the match style, the rest with base.
	wantMatch := ui.StyleFilterMatch.Render("bo")
	if !strings.Contains(out, wantMatch) {
		t.Errorf("expected matched 'bo' to be highlighted; out=%q", out)
	}
	if !strings.Contains(out, base.Render("ro")) || !strings.Contains(out, base.Render("t")) {
		t.Errorf("expected non-matched parts in base style; out=%q", out)
	}
}

func TestHighlightMatch_CaseInsensitiveAllOccurrences(t *testing.T) {
	out := highlightMatch("Go to bottom", "o", ui.StyleHint)
	if n := strings.Count(out, ui.StyleFilterMatch.Render("o")); n < 3 {
		t.Errorf("expected every 'o'/'O' highlighted, got %d highlights in %q", n, out)
	}
}

func TestHighlightMatch_EmptyQueryIsPlainBase(t *testing.T) {
	base := ui.StyleHint
	if got, want := highlightMatch("delete", "", base), base.Render("delete"); got != want {
		t.Errorf("empty query: got %q want %q", got, want)
	}
}

func TestRenderColumns_WideIsMultiColumn(t *testing.T) {
	sections := []KeySection{sectionN("App", 2), sectionN("Nav", 2), sectionN("Yank", 2)}
	// Wide width → 3 columns → the three headings share the first line.
	out := ansi.Strip(RenderColumns(sections, 120, ""))
	firstLine := strings.Split(out, "\n")[0]
	for _, h := range []string{"App", "Nav", "Yank"} {
		if !strings.Contains(firstLine, h) {
			t.Errorf("expected heading %q on first line of wide layout, got %q", h, firstLine)
		}
	}
}
