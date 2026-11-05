package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// rowEditor is the "edit the selected file, yank its text" behaviour shared by
// the Tickets and Queue tabs. The owning model resolves the selection and
// passes it in, so rowEditor reads no model state.
type rowEditor struct {
	worktreeRoot string
	settings     ui.Settings
}

// edit launches the editor on path. An archived row opens read-only.
func (e rowEditor) edit(path string, splitType terminalrun.SplitType, readOnly bool) tea.Cmd {
	return launchTicketEditor(e.worktreeRoot, e.settings, path, splitType, readOnly)
}

// yank copies text to the clipboard and reports "yanked <what>".
func (rowEditor) yank(text, what string) tea.Cmd {
	if err := ticketsClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked " + what)
}

// finished reacts to the edit-chord result for both tabs.
func (rowEditor) finished(msg editFileFinishedMsg) tea.Cmd {
	if msg.err != nil {
		return notify.Error("edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return notify.Info("opened " + msg.splitApp + " split: editor")
	}
	return nil
}
