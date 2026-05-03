package status

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
)

var stageClipboardWrite = clipboard.WriteAll

func (m *Model) yankFilename() {
	file, ok := m.selectedExplorerFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	text := file.path
	if err := stageClipboardWrite(text); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked filename")
}

func (m *Model) yankLocationOnly() {
	file, ok := m.selectedExplorerFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if m.focus == focusStatus {
		if err := stageClipboardWrite("@" + file.path); err != nil {
			m.setStatus("clipboard copy failed: " + err.Error())
			return
		}
		m.setStatus("yanked location")
		return
	}
	loc, _, ok := m.focusedLocationAndBody()
	if !ok {
		return
	}
	if err := stageClipboardWrite("@" + file.path + " " + loc); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked location")
}

func (m *Model) yankAllContext() {
	file, ok := m.selectedExplorerFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if m.focus == focusStatus {
		if err := stageClipboardWrite("@" + file.path); err != nil {
			m.setStatus("clipboard copy failed: " + err.Error())
			return
		}
		m.setStatus("yanked all context")
		return
	}

	loc, body, ok := m.focusedLocationAndBody()
	if !ok {
		return
	}
	text := fmt.Sprintf("@%s %s\n\n%s", file.path, loc, strings.Join(body, "\n"))
	if err := stageClipboardWrite(text); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked all context")
}

func (m *Model) yankContentOnly() {
	if m.focus == focusStatus {
		m.setStatus("no diff selection to yank")
		return
	}
	sec := m.currentSection()
	hunkIdx := m.activeHunkIndexForYank(*sec)
	if hunkIdx < 0 || hunkIdx >= len(sec.parsed.Hunks) {
		m.setStatus("no hunk selected")
		return
	}
	body := m.focusedYankBody(*sec, hunkIdx)
	if len(body) == 0 {
		m.setStatus("no lines to yank")
		return
	}
	if err := stageClipboardWrite(strings.Join(body, "\n")); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked content")
}

func (m *Model) focusedLocationAndBody() (string, []string, bool) {
	sec := m.currentSection()
	hunkIdx := m.activeHunkIndexForYank(*sec)
	if hunkIdx < 0 || hunkIdx >= len(sec.parsed.Hunks) {
		m.setStatus("no hunk selected")
		return "", nil, false
	}
	body := m.focusedYankBody(*sec, hunkIdx)
	if len(body) == 0 {
		m.setStatus("no lines to yank")
		return "", nil, false
	}
	h := sec.parsed.Hunks[hunkIdx]
	startLine, endLine := m.focusedLineSpanForYank(*sec, hunkIdx)
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
	return loc, body, true
}

func (m Model) focusedYankBody(sec sectionState, hunkIdx int) []string {
	if m.navMode == navLine {
		startIdx, endIdx := sec.activeLine, sec.activeLine
		if sec.visualActive {
			startIdx, endIdx = visualLineBounds(sec)
		}
		body := make([]string, 0, maxInt(1, endIdx-startIdx+1))
		for i := startIdx; i <= endIdx && i < len(sec.parsed.Changed); i++ {
			if i < 0 {
				continue
			}
			line := sec.parsed.Changed[i].Text
			if line == "" {
				continue
			}
			body = append(body, line)
		}
		return body
	}

	h := sec.parsed.Hunks[hunkIdx]
	body := make([]string, 0, maxInt(1, h.EndLine-h.StartLine))
	for i := h.StartLine + 1; i <= h.EndLine && i < len(sec.parsed.Lines); i++ {
		line := sec.parsed.Lines[i]
		if line == "" {
			continue
		}
		body = append(body, line)
	}
	return body
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

func (m Model) focusedLineSpanForYank(sec sectionState, hunkIdx int) (int, int) {
	if m.navMode != navLine {
		return m.activeLineSpanForYank(sec, hunkIdx)
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
		if i < 0 {
			continue
		}
		ln := changedLineNumberForYank(sec.parsed.Changed[i])
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
