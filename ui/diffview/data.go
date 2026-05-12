package diffview

import (
	"strings"

	diffcore "github.com/elentok/gx/ui/diffview/diffcore"
	diffrender "github.com/elentok/gx/ui/diffview/diffrender"
)

// DiffData contains the complete per-pane diff state:
// parsed diff, rendered lines/mappings, and active selection/cursor.
type DiffData struct {
	RawLines         []string
	BaseLines        []string
	BaseLineKinds    []diffrender.RowKind
	BaseDisplayToRaw []int
	ViewLines        []string
	ViewLineKinds    []diffrender.RowKind
	DisplayToRaw     []int
	RawToDisplay     []int
	HunkDisplayRange [][2]int
	ChangedDisplay   []int
	Parsed           diffcore.ParsedDiff
	ActiveHunk       int
	ActiveLine       int
	VisualActive     bool
	VisualAnchor     int
}

func (d *DiffData) HasContent() bool {
	if d == nil {
		return false
	}
	return len(d.ViewLines) > 0 || diffcore.HasBinaryDiff(d.Parsed)
}

func (d DiffData) ActiveRawLineIndex(navMode NavMode) int {
	if navMode == NavModeHunk {
		if d.ActiveHunk >= 0 && d.ActiveHunk < len(d.Parsed.Hunks) {
			return d.Parsed.Hunks[d.ActiveHunk].StartLine
		}
		return -1
	}
	if d.ActiveLine >= 0 && d.ActiveLine < len(d.Parsed.Changed) {
		return d.Parsed.Changed[d.ActiveLine].LineIndex
	}
	return -1
}

func (d DiffData) HunkDisplayBounds(hunkIdx int) (start int, end int, ok bool) {
	if hunkIdx >= 0 && hunkIdx < len(d.HunkDisplayRange) {
		r := d.HunkDisplayRange[hunkIdx]
		if r[0] >= 0 && r[1] >= r[0] {
			return r[0], r[1], true
		}
	}
	if hunkIdx < 0 || hunkIdx >= len(d.Parsed.Hunks) {
		return 0, 0, false
	}
	h := d.Parsed.Hunks[hunkIdx]
	start = -1
	end = -1
	for displayIdx, rawIdx := range d.DisplayToRaw {
		if rawIdx < h.StartLine || rawIdx > h.EndLine {
			continue
		}
		if start < 0 {
			start = displayIdx
		}
		end = displayIdx
	}
	if start < 0 || end < 0 {
		return 0, 0, false
	}
	return start, end, true
}

func (d DiffData) VisualLineBounds() (start, end int) {
	start = d.VisualAnchor
	end = d.ActiveLine
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	changedCount := len(d.Parsed.Changed)
	if end >= changedCount {
		end = changedCount - 1
	}
	if start >= changedCount {
		start = changedCount - 1
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

func NewDiffData() DiffData {
	return DiffData{
		ActiveHunk:   -1,
		ActiveLine:   -1,
		VisualAnchor: -1,
	}
}

func BuildDiffData(raw, color string, prev DiffData, sideBySide bool) DiffData {
	state := DiffData{
		ActiveHunk:   prev.ActiveHunk,
		ActiveLine:   prev.ActiveLine,
		VisualActive: prev.VisualActive,
		VisualAnchor: prev.VisualAnchor,
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		state.ActiveHunk = -1
		state.ActiveLine = -1
		state.VisualActive = false
		state.VisualAnchor = -1
		return state
	}

	state.Parsed = diffcore.ParseUnifiedDiff(raw)
	state.RawLines = append([]string{}, state.Parsed.Lines...)
	if sideBySide {
		initSideBySideDiffData(&state, color)
		return state
	}

	colorLines := splitLines(color)
	if len(colorLines) == 0 {
		colorLines = append([]string{}, state.RawLines...)
	} else if len(colorLines) < len(state.RawLines) {
		colorLines = append(colorLines, state.RawLines[len(colorLines):]...)
	} else if len(colorLines) > len(state.RawLines) {
		colorLines = colorLines[:len(state.RawLines)]
	}
	state.BaseLines, state.BaseLineKinds, state.BaseDisplayToRaw = diffrender.BuildDisplayBaseLines(state.Parsed, colorLines)
	state.ViewLines = append([]string{}, state.BaseLines...)
	state.ViewLineKinds = append([]diffrender.RowKind{}, state.BaseLineKinds...)
	state.DisplayToRaw = append([]int{}, state.BaseDisplayToRaw...)
	state.RawToDisplay = diffcore.BuildRawToDisplayMap(state.Parsed, state.DisplayToRaw)
	state.HunkDisplayRange = nil
	state.ChangedDisplay = nil

	clampDiffDataSelection(&state)
	return state
}

func reflowDiffData(state *DiffData, wrapWidth int, wrapSoft bool) {
	if len(state.BaseLines) == 0 {
		state.ViewLines = nil
		state.ViewLineKinds = nil
		state.DisplayToRaw = nil
		state.RawToDisplay = diffcore.BuildRawToDisplayMap(state.Parsed, nil)
		return
	}

	view := make([]string, 0, len(state.BaseLines))
	kinds := make([]diffrender.RowKind, 0, len(state.BaseLines))
	mapRaw := make([]int, 0, len(state.BaseDisplayToRaw))

	for i, line := range state.BaseLines {
		rawIdx := -1
		kind := diffrender.RowPlain
		if i < len(state.BaseDisplayToRaw) {
			rawIdx = state.BaseDisplayToRaw[i]
		}
		if i < len(state.BaseLineKinds) {
			kind = state.BaseLineKinds[i]
		}
		if !wrapSoft || rawIdx < 0 {
			view = append(view, line)
			kinds = append(kinds, kind)
			mapRaw = append(mapRaw, rawIdx)
			continue
		}
		parts := diffrender.WrapANSI(line, wrapWidth)
		for _, p := range parts {
			view = append(view, p)
			kinds = append(kinds, kind)
			mapRaw = append(mapRaw, rawIdx)
		}
	}

	state.ViewLines = view
	state.ViewLineKinds = kinds
	state.DisplayToRaw = mapRaw
	state.RawToDisplay = diffcore.BuildRawToDisplayMap(state.Parsed, state.DisplayToRaw)
}

func initSideBySideDiffData(state *DiffData, color string) {
	state.ViewLines = splitLines(color)
	if len(state.ViewLines) == 0 {
		state.ViewLines = append([]string{}, state.RawLines...)
	}
	state.BaseLines = append([]string{}, state.ViewLines...)
	state.BaseLineKinds = make([]diffrender.RowKind, len(state.BaseLines))
	state.BaseDisplayToRaw = make([]int, len(state.BaseLines))
	for i := range state.BaseDisplayToRaw {
		state.BaseDisplayToRaw[i] = -1
	}
	state.ViewLineKinds = append([]diffrender.RowKind{}, state.BaseLineKinds...)

	mapping := buildSideBySideMapping(state.Parsed, state.ViewLines)
	state.DisplayToRaw = mapping.DisplayToRaw
	state.RawToDisplay = mapping.RawToDisplay
	state.ChangedDisplay = mapping.ChangedDisplay
	state.HunkDisplayRange = mapping.HunkDisplayRange

	clampDiffDataSelection(state)
}

func clampDiffDataSelection(state *DiffData) {
	if len(state.Parsed.Hunks) == 0 {
		state.ActiveHunk = -1
	} else {
		if state.ActiveHunk < 0 {
			state.ActiveHunk = 0
		}
		if state.ActiveHunk >= len(state.Parsed.Hunks) {
			state.ActiveHunk = len(state.Parsed.Hunks) - 1
		}
	}

	if len(state.Parsed.Changed) == 0 {
		state.ActiveLine = -1
		state.VisualActive = false
		state.VisualAnchor = -1
	} else {
		if state.ActiveLine < 0 {
			state.ActiveLine = 0
		}
		if state.ActiveLine >= len(state.Parsed.Changed) {
			state.ActiveLine = len(state.Parsed.Changed) - 1
		}
		if state.VisualAnchor < 0 || state.VisualAnchor >= len(state.Parsed.Changed) {
			state.VisualAnchor = state.ActiveLine
		}
	}
}
