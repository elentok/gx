package commit

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/yankfmt"

	tea "charm.land/bubbletea/v2"
)

var commitClipboardWrite = clipboard.WriteAll

func (m *Model) selectedFile() (path string, ok bool) {
	file, ok := m.selectedCommitFile()
	if !ok {
		return "", false
	}
	return file.Path, true
}

func (m *Model) yankFilename() tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	if err := commitClipboardWrite(path); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked filename")
}

func (m *Model) yankLocationOnly() tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	if !m.focusDiff {
		if err := commitClipboardWrite(yankfmt.FormatYankLocation(path, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked location")
	}
	loc, _, cmd, ok := m.focusedLocationAndBody()
	if !ok {
		return cmd
	}
	if err := commitClipboardWrite(yankfmt.FormatYankLocation(path, loc)); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked location")
}

func (m *Model) yankAllContext() tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
		return notify.Warning("no file selected")
	}
	if !m.focusDiff {
		if err := commitClipboardWrite(yankfmt.FormatYankLocation(path, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked all context")
	}
	loc, body, cmd, ok := m.focusedLocationAndBody()
	if !ok {
		return cmd
	}
	text := yankfmt.FormatYankAllContext(path, loc, body)
	if err := commitClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked all context")
}

func (m *Model) yankContentOnly() tea.Cmd {
	if !m.focusDiff {
		return notify.Warning("no diff selection to yank")
	}
	_, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr == diffview.FocusedYankErrNoHunk {
		return notify.Warning(string(yankErr))
	}
	if yankErr == diffview.FocusedYankErrNoLines {
		return notify.Warning(string(yankErr))
	}
	if err := commitClipboardWrite(strings.Join(body, "\n")); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked content")
}

func (m *Model) yankCommitBody() tea.Cmd {
	body := m.commitMessageBody()
	if body == "" {
		return notify.Warning("no commit body to yank")
	}
	if err := commitClipboardWrite(body); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked commit body")
}

func (m *Model) focusedLocationAndBody() (string, []string, tea.Cmd, bool) {
	loc, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr != "" {
		return "", nil, notify.Warning(string(yankErr)), false
	}
	return loc, body, nil, true
}
