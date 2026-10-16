package status

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/status/diffarea"
)

func newDiffHeaderTestModel(t *testing.T) Model {
	t.Helper()
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.width = 120
	m.height = 30
	m.focus = focusDiff
	return m
}

func sectionHeader(m Model, section diffarea.Section) string {
	pane := m.renderSectionPane(80, 20, section)
	return ansi.Strip(strings.SplitN(pane, "\n", 2)[0])
}

func TestDiffHeaderShowsVisualWrapNavModeAndContextInOrder(t *testing.T) {
	m := newDiffHeaderTestModel(t)
	m.diffarea.SetNavMode(diffview.NavModeLine)
	m.diffarea.SetWrap(true)
	m.diffarea.Unstaged.DataRef().VisualActive = true

	header := sectionHeader(m, diffarea.SectionUnstaged)

	order := []string{"VISUAL", ui.Icons(true).Wrap + " wrap", "line", "±1"}
	pos := -1
	for _, want := range order {
		idx := strings.Index(header, want)
		if idx <= pos {
			t.Fatalf("expected %q after position %d in header %q", want, pos, header)
		}
		pos = idx
	}
	for _, unwanted := range []string{"unified", "render", "Context:", "focus"} {
		if strings.Contains(header, unwanted) {
			t.Fatalf("expected no %q in header %q", unwanted, header)
		}
	}
}

func TestDiffHeaderHidesVisualAndWrapWhenOff(t *testing.T) {
	m := newDiffHeaderTestModel(t)
	m.diffarea.SetWrap(false)

	header := sectionHeader(m, diffarea.SectionUnstaged)

	if strings.Contains(header, "VISUAL") || strings.Contains(header, "wrap") {
		t.Fatalf("expected no VISUAL/wrap in header %q", header)
	}
	if !strings.Contains(header, "hunk · ±1") {
		t.Fatalf("expected nav mode and context in header %q", header)
	}
}

func TestDiffHeaderWrapWithoutNerdFontsShowsOnlyTheWord(t *testing.T) {
	m := newDiffHeaderTestModel(t)
	m.settings.UseNerdFontIcons = false
	m.diffarea.SetWrap(true)

	header := sectionHeader(m, diffarea.SectionUnstaged)

	if !strings.Contains(header, "wrap · hunk") {
		t.Fatalf("expected bare wrap word in header %q", header)
	}
	if strings.Contains(header, ui.Icons(true).Wrap) {
		t.Fatalf("expected no wrap icon without nerd fonts in header %q", header)
	}
}

func TestDiffHeaderOptionsOnlyOnFocusedPanel(t *testing.T) {
	m := newDiffHeaderTestModel(t)
	m.diffarea.SetWrap(true)

	for _, header := range []string{
		sectionHeader(m, diffarea.SectionStaged),
		func() string { m.focus = focusFiletree; return sectionHeader(m, diffarea.SectionUnstaged) }(),
	} {
		for _, unwanted := range []string{"wrap", "hunk", "±"} {
			if strings.Contains(header, unwanted) {
				t.Fatalf("expected no %q on unfocused panel header %q", unwanted, header)
			}
		}
	}
}

func TestViewHasNoFooterAndFillsFullHeight(t *testing.T) {
	for _, focus := range []focusPane{focusFiletree, focusDiff} {
		m := newDiffHeaderTestModel(t)
		m.focus = focus

		plain := ansi.Strip(m.View().Content)

		if got := strings.Count(plain, "\n") + 1; got != m.height {
			t.Fatalf("focus %v: expected %d rows, got %d", focus, m.height, got)
		}
		if strings.Contains(plain, "? help") || strings.Contains(plain, "diff: mode:") {
			t.Fatalf("focus %v: expected no footer, got %q", focus, plain)
		}
	}
}

func TestHelpExplainsContextIndicator(t *testing.T) {
	m := newDiffHeaderTestModel(t)
	if !strings.Contains(ansi.Strip(help.RenderView(m.help.KeySections)), "±N") {
		t.Fatalf("expected help to explain ±N")
	}
}
