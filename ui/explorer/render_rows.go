package explorer

import "github.com/elentok/gx/ui/diff"

type VisibleDiffRow struct {
	DisplayIndex       int
	RawIndex           int
	Text               string
	Kind               diff.RowKind
	InActiveHunk       bool
	IsActiveRaw        bool
	IsActiveChangedRaw bool
	OverflowTop        bool
	OverflowBottom     bool
}

type VisibleDiffRowsOptions struct {
	Section    SectionData
	ViewportY  int
	Visible    int
	BodyHeight int
	NavMode    NavMode
	Active     bool
	ActiveRaw  int
}

func BuildVisibleDiffRows(opts VisibleDiffRowsOptions) []VisibleDiffRow {
	rows := make([]VisibleDiffRow, 0, maxInt(0, opts.BodyHeight))
	if opts.BodyHeight <= 0 {
		return rows
	}

	hunkStart, hunkEnd := -1, -1
	if opts.NavMode == NavHunk && opts.Section.ActiveHunk >= 0 && opts.Section.ActiveHunk < len(opts.Section.Parsed.Hunks) {
		hunkStart = opts.Section.Parsed.Hunks[opts.Section.ActiveHunk].StartLine
		hunkEnd = opts.Section.Parsed.Hunks[opts.Section.ActiveHunk].EndLine
	}

	overflowTopDisplay := -1
	overflowBottomDisplay := -1
	if opts.NavMode == NavHunk && opts.Active && opts.Section.ActiveHunk >= 0 {
		if start, end, ok := HunkDisplayBounds(opts.Section.HunkDisplayRange, opts.Section.Parsed, opts.Section.DisplayToRaw, opts.Section.ActiveHunk); ok && opts.Visible > 0 {
			vpBottom := opts.ViewportY + opts.Visible - 1
			if start < opts.ViewportY {
				overflowTopDisplay = opts.ViewportY
			}
			if end > vpBottom {
				overflowBottomDisplay = vpBottom
			}
		}
	}

	for i := 0; i < opts.BodyHeight; i++ {
		displayIdx := opts.ViewportY + i
		if displayIdx >= len(opts.Section.ViewLines) {
			rows = append(rows, VisibleDiffRow{DisplayIndex: displayIdx, RawIndex: -1})
			continue
		}
		rawIdx := -1
		if displayIdx >= 0 && displayIdx < len(opts.Section.DisplayToRaw) {
			rawIdx = opts.Section.DisplayToRaw[displayIdx]
		}
		rowKind := diff.RowPlain
		if displayIdx >= 0 && displayIdx < len(opts.Section.ViewLineKinds) {
			rowKind = opts.Section.ViewLineKinds[displayIdx]
		}

		inActiveHunk := false
		if opts.NavMode == NavHunk {
			if len(opts.Section.HunkDisplayRange) > 0 && opts.Section.ActiveHunk >= 0 && opts.Section.ActiveHunk < len(opts.Section.HunkDisplayRange) {
				r := opts.Section.HunkDisplayRange[opts.Section.ActiveHunk]
				inActiveHunk = displayIdx >= r[0] && displayIdx <= r[1]
			} else {
				inActiveHunk = rawIdx >= 0 && rawIdx >= hunkStart && rawIdx <= hunkEnd
			}
		}

		isChanged := rawIdx < 0 && opts.NavMode == NavLine && opts.Active && opts.Section.ActiveLine >= 0 && opts.Section.ActiveLine < len(opts.Section.ChangedDisplay) && opts.Section.ChangedDisplay[opts.Section.ActiveLine] == displayIdx

		rows = append(rows, VisibleDiffRow{
			DisplayIndex:       displayIdx,
			RawIndex:           rawIdx,
			Text:               opts.Section.ViewLines[displayIdx],
			Kind:               rowKind,
			InActiveHunk:       inActiveHunk,
			IsActiveRaw:        rawIdx >= 0 && rawIdx == opts.ActiveRaw && opts.Active,
			IsActiveChangedRaw: isChanged,
			OverflowTop:        displayIdx == overflowTopDisplay && inActiveHunk,
			OverflowBottom:     displayIdx == overflowBottomDisplay && inActiveHunk,
		})
	}

	return rows
}
