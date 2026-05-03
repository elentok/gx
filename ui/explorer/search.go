package explorer

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"
)

type DiffSearchMatch struct {
	DisplayIndex int
	RawIndex     int
}

func ComputeDiffSearchMatches(viewLines []string, displayToRaw []int, query string) []DiffSearchMatch {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	matches := make([]DiffSearchMatch, 0)
	for i := 0; i < len(viewLines) && i < len(displayToRaw); i++ {
		line := strings.ToLower(ansi.Strip(viewLines[i]))
		if strings.Contains(line, q) {
			matches = append(matches, DiffSearchMatch{
				DisplayIndex: i,
				RawIndex:     displayToRaw[i],
			})
		}
	}
	return matches
}

func ApplyDiffSearchMatch(section *SectionData, vp *viewport.Model, match DiffSearchMatch) {
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
	if match.RawIndex < 0 {
		return
	}
	for i, ch := range section.Parsed.Changed {
		if ch.LineIndex == match.RawIndex {
			section.ActiveLine = i
			break
		}
	}
	for i, h := range section.Parsed.Hunks {
		if match.RawIndex >= h.StartLine && match.RawIndex <= h.EndLine {
			section.ActiveHunk = i
			break
		}
	}
}

func CurrentDiffSearchMatchIndex(section SectionData, matches []DiffSearchMatch, navMode NavMode) int {
	if navMode != NavLine || section.ActiveLine < 0 || section.ActiveLine >= len(section.Parsed.Changed) {
		return -1
	}
	raw := section.Parsed.Changed[section.ActiveLine].LineIndex
	for i, match := range matches {
		if match.RawIndex == raw {
			return i
		}
	}
	return -1
}

func DiffSearchMatchIndex(matches []DiffSearchMatch, displayIdx int) int {
	for i, match := range matches {
		if match.DisplayIndex == displayIdx {
			return i
		}
	}
	return -1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
