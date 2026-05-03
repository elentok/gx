package commit

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/elentok/gx/ui/explorer"
)

var commitClipboardWrite = clipboard.WriteAll

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
}

func (m *Model) clearStatus() {
	m.statusMsg = ""
}

func (m *Model) selectedFile() (path string, ok bool) {
	if m.selected < 0 || m.selected >= len(m.files) {
		return "", false
	}
	return m.files[m.selected].Path, true
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
		if err := commitClipboardWrite("@" + path); err != nil {
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
	if err := commitClipboardWrite("@" + path + " " + loc); err != nil {
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
		if err := commitClipboardWrite("@" + path); err != nil {
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
	text := fmt.Sprintf("@%s %s\n\n%s", path, loc, strings.Join(body, "\n"))
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
	if explorer.ActiveHunkIndexForYank(m.section, m.diffNavMode) < 0 {
		m.setStatus("no hunk selected")
		return
	}
	body := explorer.FocusedYankBody(m.section, m.diffNavMode)
	if len(body) == 0 {
		m.setStatus("no lines to yank")
		return
	}
	if err := commitClipboardWrite(strings.Join(body, "\n")); err != nil {
		m.setStatus("clipboard copy failed: " + err.Error())
		return
	}
	m.setStatus("yanked content")
}

func (m *Model) focusedLocationAndBody() (string, []string, bool) {
	if explorer.ActiveHunkIndexForYank(m.section, m.diffNavMode) < 0 {
		m.setStatus("no hunk selected")
		return "", nil, false
	}
	body := explorer.FocusedYankBody(m.section, m.diffNavMode)
	if len(body) == 0 {
		m.setStatus("no lines to yank")
		return "", nil, false
	}
	loc := explorer.FocusedLocation(m.section, m.diffNavMode)
	return loc, body, true
}
