package status

import (
	"fmt"
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
		if err := stageClipboardWrite("@" + file.Path); err != nil {
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
	if err := stageClipboardWrite("@" + file.Path + " " + loc); err != nil {
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
		if err := stageClipboardWrite("@" + file.Path); err != nil {
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
	text := fmt.Sprintf("@%s %s\n\n%s", file.Path, loc, strings.Join(body, "\n"))
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
	if explorer.ActiveHunkIndexForYank(sec.data, m.navMode) < 0 {
		m.setStatus("no hunk selected")
		return
	}
	body := explorer.FocusedYankBody(sec.data, m.navMode)
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
	hunkIdx := explorer.ActiveHunkIndexForYank(sec.data, m.navMode)
	if hunkIdx < 0 || hunkIdx >= len(sec.data.Parsed.Hunks) {
		m.setStatus("no hunk selected")
		return "", nil, false
	}
	body := explorer.FocusedYankBody(sec.data, m.navMode)
	if len(body) == 0 {
		m.setStatus("no lines to yank")
		return "", nil, false
	}
	loc := explorer.FocusedLocation(sec.data, m.navMode)
	return loc, body, true
}
