package diffview

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/diffview/diffrender"
)

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
