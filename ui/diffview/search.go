package diffview

import (
	"charm.land/bubbles/v2/viewport"
	"github.com/elentok/gx/ui/search"
)

type DiffSearchMatch struct {
	DisplayIndex int
	RawIndex     int
}

func applyDiffSearchMatch(section *DiffData, vp *viewport.Model, match search.Match) {
	if match.ViewportRow >= 0 {
		if match.ViewportRow < vp.YOffset() {
			vp.SetYOffset(match.ViewportRow)
		} else {
			last := vp.YOffset() + vp.VisibleLineCount() - 1
			if vp.VisibleLineCount() > 0 && match.ViewportRow > last {
				vp.SetYOffset(maxInt(0, match.ViewportRow-vp.VisibleLineCount()+1))
			}
		}
	}
	if match.DataIndex < 0 {
		return
	}
	for i, ch := range section.Parsed.Changed {
		if ch.LineIndex == match.DataIndex {
			section.ActiveLine = i
			break
		}
	}
	for i, h := range section.Parsed.Hunks {
		if match.DataIndex >= h.StartLine && match.DataIndex <= h.EndLine {
			section.ActiveHunk = i
			break
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
