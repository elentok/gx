package stage

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
)

var stageClipboardWrite = clipboard.WriteAll

func (m *Model) yankFilename() {
	file, ok := m.selectedFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	text := file.Path
	if err := stageClipboardWrite(text); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked filename")
}

func (m *Model) yankContextForAI() {
	file, ok := m.selectedFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if m.focus == focusStatus {
		if err := stageClipboardWrite("@" + file.Path); err != nil {
			m.setStatus("clipboard copy failed: " + err.Error())
			return
		}
		m.setStatus("yanked context")
		return
	}

	sec := m.currentSection()
	hunkIdx := m.activeHunkIndexForYank(*sec)
	if hunkIdx < 0 || hunkIdx >= len(sec.parsed.Hunks) {
		m.setStatus("no hunk selected")
		return
	}
	h := sec.parsed.Hunks[hunkIdx]

	startLine, endLine := m.activeLineSpanForYank(*sec, hunkIdx)
	if startLine <= 0 {
		startLine = h.NewStart
		count := h.NewCount
		if count < 1 {
			count = 1
		}
		endLine = startLine + count - 1
	}

	body := make([]string, 0, maxInt(1, h.EndLine-h.StartLine))
	for i := h.StartLine + 1; i <= h.EndLine && i < len(sec.parsed.Lines); i++ {
		line := sec.parsed.Lines[i]
		if line == "" {
			continue
		}
		body = append(body, line)
	}
	if len(body) == 0 {
		m.setStatus("no hunk lines to yank")
		return
	}

	loc := fmt.Sprintf("L%d", startLine)
	if endLine > startLine {
		loc = fmt.Sprintf("L%d-%d", startLine, endLine)
	}
	text := fmt.Sprintf("@%s %s\n\n%s", file.Path, loc, strings.Join(body, "\n"))
	if err := stageClipboardWrite(text); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked context")
}

func (m Model) activeHunkIndexForYank(sec sectionState) int {
	if m.navMode == navHunk {
		return sec.activeHunk
	}
	if sec.activeLine >= 0 && sec.activeLine < len(sec.parsed.Changed) {
		return sec.parsed.Changed[sec.activeLine].HunkIndex
	}
	return sec.activeHunk
}

func (m Model) activeLineSpanForYank(sec sectionState, hunkIdx int) (int, int) {
	h := sec.parsed.Hunks[hunkIdx]
	if m.navMode == navHunk {
		count := h.NewCount
		if count < 1 {
			count = 1
		}
		return h.NewStart, h.NewStart + count - 1
	}
	if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
		return 0, 0
	}
	startIdx, endIdx := sec.activeLine, sec.activeLine
	if sec.visualActive {
		startIdx, endIdx = visualLineBounds(sec)
	}
	lineStart := 0
	lineEnd := 0
	for i := startIdx; i <= endIdx && i < len(sec.parsed.Changed); i++ {
		cl := sec.parsed.Changed[i]
		if cl.HunkIndex != hunkIdx {
			continue
		}
		ln := changedLineNumberForYank(cl)
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
		ln := changedLineNumberForYank(sec.parsed.Changed[sec.activeLine])
		if ln > 0 {
			return ln, ln
		}
	}
	return lineStart, lineEnd
}

func changedLineNumberForYank(cl changedLine) int {
	if cl.Prefix == '-' && cl.OldLine > 0 {
		return cl.OldLine
	}
	if cl.NewLine > 0 {
		return cl.NewLine
	}
	return cl.OldLine
}
