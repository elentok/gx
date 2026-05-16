package status

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/yankfmt"
)

var stageClipboardWrite = clipboard.WriteAll

func (m *Model) yankFilename() tea.Cmd {
	file, ok := m.selectedStatusFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	text := file.Path
	if err := stageClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked filename")
}

func (m *Model) yankLocationOnly() tea.Cmd {
	file, ok := m.selectedStatusFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	if m.focus == focusFiletree {
		if err := stageClipboardWrite(yankfmt.FormatYankLocation(file.Path, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked location")
	}
	loc, _, cmd, ok := m.focusedLocationAndBody()
	if !ok {
		return cmd
	}
	if err := stageClipboardWrite(yankfmt.FormatYankLocation(file.Path, loc)); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked location")
}

func (m *Model) yankAllContext() tea.Cmd {
	file, ok := m.selectedStatusFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	if m.focus == focusFiletree {
		if err := stageClipboardWrite(yankfmt.FormatYankLocation(file.Path, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked for AI agent")
	}

	loc, body, cmd, ok := m.focusedLocationAndBody()
	if !ok {
		return cmd
	}
	text := yankfmt.FormatForAgent(file.Path, loc, body)
	if err := stageClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked for AI agent")
}

func (m *Model) yankContentOnly() tea.Cmd {
	if m.focus == focusFiletree {
		return notify.Warning("no diff selection to yank")
	}
	diffviewModel := m.diffarea.ActiveSectionModel()
	_, body, yankErr := diffviewModel.FocusedLocationAndBody()
	if yankErr == diffview.FocusedYankErrNoHunk {
		return notify.Warning(string(yankErr))
	}
	if yankErr == diffview.FocusedYankErrNoLines {
		return notify.Warning(string(yankErr))
	}
	if err := stageClipboardWrite(strings.Join(body, "\n")); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked content")
}

func (m *Model) focusedLocationAndBody() (string, []string, tea.Cmd, bool) {
	diffviewModel := m.diffarea.ActiveSectionModel()
	loc, body, yankErr := diffviewModel.FocusedLocationAndBody()
	if yankErr != "" {
		return "", nil, notify.Warning(string(yankErr)), false
	}
	return loc, body, nil, true
}
