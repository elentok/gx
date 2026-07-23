package tickets

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// editFileFinishedMsg reports the outcome of an edit-chord launch, mirroring
// every other tab's edit-chord result message (e.g. ui/status).
type editFileFinishedMsg struct {
	err      error
	splitApp string
}

// cmdEditSelectedFile opens the selected row's underlying file for editing:
// a ticket's own file, or an epic's map.md if it has one. A plain epic (no
// map.md) has nothing to edit, so it's a no-op with an inline message rather
// than a crash.
func (m Model) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	target, ok, warning := m.selectedEditTarget()
	if !ok {
		return notify.Warning(warning)
	}

	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return notify.Warning("$EDITOR is not set")
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return notify.Warning("$EDITOR is empty")
	}

	args := ui.EditorLaunchArgs(parts[0], parts[1:], target, 0)
	return terminalrun.CommandWithSplit(m.worktreeRoot, m.settings.Terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return editFileFinishedMsg{err: err, splitApp: splitApp}
	})
}

// selectedEditTarget resolves the file path to edit for the current
// selection. A ticket row always has one; an epic row only has one when it
// has a map.md. On failure it also returns a warning message describing why.
func (m Model) selectedEditTarget() (path string, ok bool, warning string) {
	r, ok := m.selectedRow()
	if !ok {
		return "", false, "nothing selected"
	}
	epic := m.epics[r.epicIdx]
	if r.isEpic() {
		if !epic.IsMap {
			return "", false, "epic has no map.md to edit"
		}
		return filepath.Join(epic.Path, "map.md"), true, ""
	}
	return epic.Tickets[r.ticketIdx].Path, true, ""
}

func (m Model) handleEditFileFinished(msg editFileFinishedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return m, notify.Info("opened " + msg.splitApp + " split: editor")
	}
	return m, nil
}
