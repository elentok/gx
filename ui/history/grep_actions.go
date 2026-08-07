package history

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui/notify"
)

// selectedGrepResult returns the currently highlighted row in the grep
// results list, or false if the list is empty (nothing to act on).
func (m Model) selectedGrepResult() (claudehistory.GrepResult, bool) {
	sel := m.grepList.Selected()
	if len(m.grepResults) == 0 || sel < 0 || sel >= len(m.grepResults) {
		return claudehistory.GrepResult{}, false
	}
	return m.grepResults[sel], true
}

// cmdExportAndEditGrep exports the selected grep result's transcript to a
// temp markdown file and opens it in the resolved editor, the same as
// ticket 11's cmdExportAndEdit but targeting the grep result's file.
func (m Model) cmdExportAndEditGrep() tea.Cmd {
	result, ok := m.selectedGrepResult()
	if !ok {
		return notify.Warning("no grep result selected")
	}
	return cmdExportAndEditPath(result.FilePath)
}

// cmdResumeGrepResult opens `claude --resume <session-id>` for the selected
// grep result in a new terminal split, the same as ticket 11's
// cmdResumeConversation but targeting the grep result's session, with cwd
// resolved from the project owning the result's file.
func (m Model) cmdResumeGrepResult() tea.Cmd {
	result, ok := m.selectedGrepResult()
	if !ok {
		return notify.Warning("no grep result selected")
	}
	if result.SessionID == "" {
		return notify.Warning("grep result has no session id to resume")
	}
	cwd, ok := m.projectCwdForFile(result.FilePath)
	if !ok {
		return notify.Warning("could not find project for grep result")
	}
	return m.cmdResumeSession(result.SessionID, cwd)
}

// cmdYankGrepSessionID copies the selected grep result's session id to the
// clipboard, following ui/log/model_yank.go's clipboard-seam pattern (same as
// ticket 11's cmdYankSessionID on the conversations page).
func (m Model) cmdYankGrepSessionID() tea.Cmd {
	result, ok := m.selectedGrepResult()
	if !ok {
		return notify.Warning("no grep result selected")
	}
	if result.SessionID == "" {
		return notify.Warning("grep result has no session id to copy")
	}
	if err := historyClipboardWrite(result.SessionID); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("copied session id: " + result.SessionID)
}

// projectCwdForFile returns the Cwd of the project containing filePath, by
// matching filePath's parent directory against m.projects.
func (m Model) projectCwdForFile(filePath string) (string, bool) {
	dir := filepath.Dir(filePath)
	for _, p := range m.projects {
		if p.Dir == dir {
			return p.Cwd, true
		}
	}
	return "", false
}
