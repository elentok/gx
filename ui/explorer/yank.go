package explorer

import "fmt"

func ChangedLineNumberForYank(cl ChangedLineLike) int {
	if cl.Prefix() == '-' && cl.OldLineNumber() > 0 {
		return cl.OldLineNumber()
	}
	if cl.NewLineNumber() > 0 {
		return cl.NewLineNumber()
	}
	return cl.OldLineNumber()
}

type ChangedLineLike interface {
	Prefix() rune
	OldLineNumber() int
	NewLineNumber() int
	HunkIndexValue() int
	TextValue() string
}

type changedLineAdapter struct {
	prefix rune
	old    int
	new    int
	hunk   int
	text   string
}

func (c changedLineAdapter) Prefix() rune        { return c.prefix }
func (c changedLineAdapter) OldLineNumber() int  { return c.old }
func (c changedLineAdapter) NewLineNumber() int  { return c.new }
func (c changedLineAdapter) HunkIndexValue() int { return c.hunk }
func (c changedLineAdapter) TextValue() string   { return c.text }

func changedLineAt(section SectionData, idx int) (changedLineAdapter, bool) {
	if idx < 0 || idx >= len(section.Parsed.Changed) {
		return changedLineAdapter{}, false
	}
	cl := section.Parsed.Changed[idx]
	return changedLineAdapter{
		prefix: rune(cl.Prefix),
		old:    cl.OldLine,
		new:    cl.NewLine,
		hunk:   cl.HunkIndex,
		text:   cl.Text,
	}, true
}

func ActiveHunkIndexForYank(section SectionData, navMode NavMode) int {
	if navMode == NavHunk {
		return section.ActiveHunk
	}
	if cl, ok := changedLineAt(section, section.ActiveLine); ok {
		return cl.HunkIndexValue()
	}
	return section.ActiveHunk
}

func FocusedYankBody(section SectionData, navMode NavMode) []string {
	hunkIdx := ActiveHunkIndexForYank(section, navMode)
	if hunkIdx < 0 || hunkIdx >= len(section.Parsed.Hunks) {
		return nil
	}
	if navMode == NavLine {
		startIdx, endIdx := section.ActiveLine, section.ActiveLine
		if section.VisualActive {
			startIdx, endIdx = VisualLineBounds(section.VisualAnchor, section.ActiveLine, len(section.Parsed.Changed))
		}
		body := make([]string, 0, maxInt(1, endIdx-startIdx+1))
		for i := startIdx; i <= endIdx && i < len(section.Parsed.Changed); i++ {
			if i < 0 {
				continue
			}
			line := section.Parsed.Changed[i].Text
			if line == "" {
				continue
			}
			body = append(body, line)
		}
		return body
	}

	h := section.Parsed.Hunks[hunkIdx]
	body := make([]string, 0, maxInt(1, h.EndLine-h.StartLine))
	for i := h.StartLine + 1; i <= h.EndLine && i < len(section.Parsed.Lines); i++ {
		line := section.Parsed.Lines[i]
		if line == "" {
			continue
		}
		body = append(body, line)
	}
	return body
}

func FocusedLineSpanForYank(section SectionData, navMode NavMode) (int, int) {
	hunkIdx := ActiveHunkIndexForYank(section, navMode)
	if hunkIdx < 0 || hunkIdx >= len(section.Parsed.Hunks) {
		return 0, 0
	}
	h := section.Parsed.Hunks[hunkIdx]
	if navMode != NavLine {
		count := h.NewCount
		if count < 1 {
			count = 1
		}
		return h.NewStart, h.NewStart + count - 1
	}
	if section.ActiveLine < 0 || section.ActiveLine >= len(section.Parsed.Changed) {
		return 0, 0
	}
	startIdx, endIdx := section.ActiveLine, section.ActiveLine
	if section.VisualActive {
		startIdx, endIdx = VisualLineBounds(section.VisualAnchor, section.ActiveLine, len(section.Parsed.Changed))
	}
	lineStart := 0
	lineEnd := 0
	for i := startIdx; i <= endIdx && i < len(section.Parsed.Changed); i++ {
		cl, ok := changedLineAt(section, i)
		if !ok || cl.HunkIndexValue() != hunkIdx {
			continue
		}
		ln := ChangedLineNumberForYank(cl)
		if ln <= 0 {
			continue
		}
		if lineStart == 0 || ln < lineStart {
			lineStart = ln
		}
		if ln > lineEnd {
			lineEnd = ln
		}
	}
	if lineStart == 0 {
		if cl, ok := changedLineAt(section, section.ActiveLine); ok {
			ln := ChangedLineNumberForYank(cl)
			if ln > 0 {
				return ln, ln
			}
		}
	}
	return lineStart, lineEnd
}

func FocusedLocation(section SectionData, navMode NavMode) string {
	hunkIdx := ActiveHunkIndexForYank(section, navMode)
	if hunkIdx < 0 || hunkIdx >= len(section.Parsed.Hunks) {
		return ""
	}
	h := section.Parsed.Hunks[hunkIdx]
	startLine, endLine := FocusedLineSpanForYank(section, navMode)
	if startLine <= 0 {
		startLine = h.NewStart
		count := h.NewCount
		if count < 1 {
			count = 1
		}
		endLine = startLine + count - 1
	}
	loc := fmt.Sprintf("L%d", startLine)
	if endLine > startLine {
		loc = fmt.Sprintf("L%d-%d", startLine, endLine)
	}
	return loc
}
