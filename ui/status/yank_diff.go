package status

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/yankfmt"
)

var stageClipboardWrite = clipboard.WriteAll

func (m *Model) yankFilename() {
	file, ok := m.selectedStatusFile()
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
	file, ok := m.selectedStatusFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if m.focus == focusFiletree {
		if err := stageClipboardWrite(yankfmt.FormatYankLocation(file.Path, "")); err != nil {
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
	if err := stageClipboardWrite(yankfmt.FormatYankLocation(file.Path, loc)); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked location")
}

func (m *Model) yankAllContext() {
	file, ok := m.selectedStatusFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if m.focus == focusFiletree {
		if err := stageClipboardWrite(yankfmt.FormatYankLocation(file.Path, "")); err != nil {
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
	text := yankfmt.FormatYankAllContext(file.Path, loc, body)
	if err := stageClipboardWrite(text); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked all context")
}

func (m *Model) yankContentOnly() {
	if m.focus == focusFiletree {
		m.setStatus("no diff selection to yank")
		return
	}
	diffviewModel := m.diffarea.ActiveSectionModel()
	_, body, yankErr := diffviewModel.FocusedLocationAndBody()
	if yankErr == diffview.FocusedYankErrNoHunk {
		m.setStatus(string(yankErr))
		return
	}
	if yankErr == diffview.FocusedYankErrNoLines {
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
	diffviewModel := m.diffarea.ActiveSectionModel()
	loc, body, yankErr := diffviewModel.FocusedLocationAndBody()
	if yankErr != "" {
		m.setStatus(string(yankErr))
		return "", nil, false
	}
	return loc, body, true
}
