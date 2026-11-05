package diffarea

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/search"
)

const (
	minContextLines = 1
	maxContextLines = 20
)

// SetContextLines sets the number of context lines around each hunk.
func (d *Model) SetContextLines(n int) {
	d.contextLines = n
}

// ContextLines is the context line count, clamped to its allowed range.
func (d Model) ContextLines() int {
	return min(max(d.contextLines, minContextLines), maxContextLines)
}

// AdjustContextLines changes the context by delta and reports whether the
// clamped value moved.
func (d *Model) AdjustContextLines(delta int) bool {
	next := min(max(d.ContextLines()+delta, minContextLines), maxContextLines)
	if next == d.ContextLines() {
		return false
	}
	d.contextLines = next
	return true
}

// ToggleRenderMode flips between unified and side-by-side. It reports false
// when side-by-side is wanted but delta is not installed.
func (d *Model) ToggleRenderMode() bool {
	if d.renderMode == diffview.RenderModeUnified {
		if !stage.DeltaAvailable() {
			return false
		}
		d.SetRenderMode(diffview.RenderModeSideBySide)
		return true
	}
	d.SetRenderMode(diffview.RenderModeUnified)
	return true
}

// RenderWidth is the width delta renders a side-by-side diff at, given the
// width of the whole diff pane.
func RenderWidth(diffW int, nerdFont bool) int {
	innerW := max(1, diffW-2)

	markW := ansi.StringWidth("▌ ")
	indicator := "* "
	if nerdFont {
		indicator = "󰍉 "
	}
	indicatorW := ansi.StringWidth(indicator)

	return max(1, innerW-markW-indicatorW)
}

// SyncForSize reflows both sections and resizes their viewports for a diff
// pane of diffW x diffH.
func (d *Model) SyncForSize(diffW, diffH int) {
	vpW := max(1, diffW-4)
	wrapWidth := max(1, vpW-2)
	for _, section := range []Section{SectionUnstaged, SectionStaged} {
		sectionModel := d.SectionModel(section)
		sectionModel.EnableWrap(d.wrap)
		sectionModel.Reflow(wrapWidth)
	}

	expandedH, collapsedH := PaneHeights(diffH - 1)
	d.SyncViewports(vpW, expandedH, collapsedH)
}

// SectionAt is the section drawn at row relY of a diff pane diffH rows tall,
// or nil on the seam or outside the pane.
func (d *Model) SectionAt(relY, diffH int) *diffview.Model {
	if diffH <= 0 {
		return nil
	}
	expandedH, collapsedH := PaneHeights(diffH)
	if d.ActiveSection == SectionStaged {
		if relY < collapsedH {
			return d.SectionModel(SectionUnstaged)
		}
		if relY < collapsedH+expandedH {
			return d.SectionModel(SectionStaged)
		}
		return nil
	}
	if relY < expandedH {
		return d.SectionModel(SectionUnstaged)
	}
	if relY < expandedH+collapsedH {
		return d.SectionModel(SectionStaged)
	}
	return nil
}

// SyncSearchToInactive copies the active section's query to the other section
// and marks its matches as a dismissed search.
func (d *Model) SyncSearchToInactive() {
	active := d.ActiveSectionModel()
	inactive := d.InactiveSectionModel()
	query := active.Search().Query()
	diffMatches := inactive.ComputeSearchMatches(query)
	searchMatches := make([]search.Match, len(diffMatches))
	for i, dm := range diffMatches {
		searchMatches[i] = search.Match{DataIndex: dm.RawIndex, ViewportRow: dm.DisplayIndex}
	}
	inactive.Search().Start(query)
	inactive.Search().SetMatches(searchMatches)
	inactive.Search().DismissAndKeepResults()
}

// SearchCounter is the "⌕ n/total" label for a section, empty while the diff
// pane is unfocused or the section has no matches.
func (d *Model) SearchCounter(section Section, diffFocused, nerdFont bool) string {
	if !diffFocused {
		return ""
	}
	s := d.SectionModel(section).Search()
	if !s.HasQuery() || s.MatchesCount() == 0 {
		return ""
	}
	return SearchCounterText(nerdFont, s.Cursor(), s.MatchesCount())
}

// SearchCounterText formats a zero-based cursor and a match total.
func SearchCounterText(nerdFont bool, cursorZeroBased, total int) string {
	if total == 0 {
		return ""
	}
	cursor := min(max(cursorZeroBased+1, 1), total)

	icon := "⌕"
	if nerdFont {
		icon = ui.Icons(true).Search
	}
	return fmt.Sprintf("%s %d/%d", icon, cursor, total)
}
