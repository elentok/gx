package diffview

import (
	"strings"

	diffcore "github.com/elentok/gx/ui/diffview/diffcore"
	diffrender "github.com/elentok/gx/ui/diffview/diffrender"
)

type DiffBuffer struct {
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

func NewDiffBuffer() DiffBuffer {
	return DiffBuffer{
		ActiveHunk:   -1,
		ActiveLine:   -1,
		VisualAnchor: -1,
	}
}

func BuildDiffBuffer(raw, color string, prev DiffBuffer, sideBySide bool) DiffBuffer {
	state := DiffBuffer{
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
		initSideBySideDiffBuffer(&state, color)
		return state
	}

	colorLines := SplitLines(color)
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

	clampDiffBufferSelection(&state)
	return state
}

func ReflowDiffBuffer(state *DiffBuffer, wrapWidth int, wrapSoft bool) {
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

func initSideBySideDiffBuffer(state *DiffBuffer, color string) {
	state.ViewLines = SplitLines(color)
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

	mapping := BuildSideBySideMapping(state.Parsed, state.ViewLines)
	state.DisplayToRaw = mapping.DisplayToRaw
	state.RawToDisplay = mapping.RawToDisplay
	state.ChangedDisplay = mapping.ChangedDisplay
	state.HunkDisplayRange = mapping.HunkDisplayRange

	clampDiffBufferSelection(state)
}

func clampDiffBufferSelection(state *DiffBuffer) {
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
