package status

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/elentok/gx/ui/explorer"
)

var stageClipboardWrite = clipboard.WriteAll

func (m *Model) yankFilename() {
	file, ok := m.selectedExplorerFile()
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

func (m *Model) yankLocationOnly() {
	file, ok := m.selectedExplorerFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if m.focus == focusStatus {
		if err := stageClipboardWrite(explorer.FormatYankLocation(file.Path, "")); err != nil {
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
	if err := stageClipboardWrite(explorer.FormatYankLocation(file.Path, loc)); err != nil {
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
		if err := stageClipboardWrite(explorer.FormatYankLocation(file.Path, "")); err != nil {
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
	text := explorer.FormatYankAllContext(file.Path, loc, body)
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
	_, body, yankErr := explorer.FocusedLocationAndBody(sec.data, m.navMode)
	if yankErr == explorer.FocusedYankErrNoHunk {
		m.setStatus(string(yankErr))
		return
	}
	if yankErr == explorer.FocusedYankErrNoLines {
		m.setStatus(string(yankErr))
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
	loc, body, yankErr := explorer.FocusedLocationAndBody(sec.data, m.navMode)
	if yankErr != "" {
		m.setStatus(string(yankErr))
		return "", nil, false
	}
	return loc, body, true
}
