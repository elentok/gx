package commit

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/yankfmt"
)

var commitClipboardWrite = clipboard.WriteAll

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
}

func (m *Model) clearStatus() {
	m.statusMsg = ""
}

func (m *Model) selectedFile() (path string, ok bool) {
	file, ok := m.selectedCommitFile()
	if !ok {
		return "", false
	}
	return file.Path, true
}

func (m *Model) yankFilename() {
	path, ok := m.selectedFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if err := commitClipboardWrite(path); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked filename")
}

func (m *Model) yankLocationOnly() {
	path, ok := m.selectedFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if !m.focusDiff {
		if err := commitClipboardWrite(yankfmt.FormatYankLocation(path, "")); err != nil {
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
	if err := commitClipboardWrite(yankfmt.FormatYankLocation(path, loc)); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked location")
}

func (m *Model) yankAllContext() {
	path, ok := m.selectedFile()
	if !ok {
		m.setStatus("no file selected")
		return
	}
	if !m.focusDiff {
		if err := commitClipboardWrite(yankfmt.FormatYankLocation(path, "")); err != nil {
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
	text := yankfmt.FormatYankAllContext(path, loc, body)
	if err := commitClipboardWrite(text); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked all context")
}

func (m *Model) yankContentOnly() {
	if !m.focusDiff {
		m.setStatus("no diff selection to yank")
		return
	}
	_, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr == diffview.FocusedYankErrNoHunk {
		m.setStatus(string(yankErr))
		return
	}
	if yankErr == diffview.FocusedYankErrNoLines {
		m.setStatus(string(yankErr))
		return
	}
	if err := commitClipboardWrite(strings.Join(body, "\n")); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked content")
}

func (m *Model) yankCommitBody() {
	body := m.commitMessageBody()
	if body == "" {
		m.setStatus("no commit body to yank")
		return
	}
	if err := commitClipboardWrite(body); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked commit body")
}

func (m *Model) focusedLocationAndBody() (string, []string, bool) {
	loc, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr != "" {
		m.setStatus(string(yankErr))
		return "", nil, false
	}
	return loc, body, true
}
