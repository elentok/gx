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
	if match.DisplayIndex >= 0 {
		if match.DisplayIndex < vp.YOffset() {
			vp.SetYOffset(match.DisplayIndex)
		} else {
			last := vp.YOffset() + vp.VisibleLineCount() - 1
			if vp.VisibleLineCount() > 0 && match.DisplayIndex > last {
				vp.SetYOffset(maxInt(0, match.DisplayIndex-vp.VisibleLineCount()+1))
			}
		}
	}
	if match.Index < 0 {
		return
	}
	for i, ch := range section.Parsed.Changed {
		if ch.LineIndex == match.Index {
			section.ActiveLine = i
			break
		}
	}
	for i, h := range section.Parsed.Hunks {
		if match.Index >= h.StartLine && match.Index <= h.EndLine {
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
