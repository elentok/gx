package rowactions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/clip"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// EditFileFinishedMsg reports the outcome of an edit-chord launch, mirroring
// every other tab's edit-chord result message (e.g. ui/status).
type EditFileFinishedMsg struct {
	Err      error
	SplitApp string
}

// LaunchTicketEditor and ClipboardWrite are seams so tests can see what the
// tabs edit and yank.
var (
	LaunchTicketEditor = editTicketFile
	ClipboardWrite     = clip.Write
)

// Editor is the "edit the selected file, yank its text" behaviour shared by
// the Tickets and Queue tabs. The owning model resolves the selection and
// passes it in, so Editor reads no model state.
type Editor struct {
	WorktreeRoot string
	Settings     ui.Settings
}

// Edit launches the editor on path. An archived row opens read-only.
func (e Editor) Edit(path string, splitType terminalrun.SplitType, readOnly bool) tea.Cmd {
	return LaunchTicketEditor(e.WorktreeRoot, e.Settings, path, splitType, readOnly)
}

// FormatTicketSummary renders a ticket for the "yy" chord: "{epic} - {id}
// {title}".
func FormatTicketSummary(epicName string, t tickets.Ticket) string {
	return fmt.Sprintf("%s - %s %s", epicName, t.DisplayNumber(), t.Title)
}

// Yank copies text to the clipboard and reports "yanked <what>".
func (Editor) Yank(text, what string) tea.Cmd {
	if err := ClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked " + what)
}

// Finished reacts to the edit-chord result for both tabs.
func (Editor) Finished(msg EditFileFinishedMsg) tea.Cmd {
	if msg.Err != nil {
		return notify.Error("edit failed: " + msg.Err.Error())
	}
	if msg.SplitApp != "" {
		return notify.Info("opened " + msg.SplitApp + " split: editor")
	}
	return nil
}

// editTicketFile launches $EDITOR on path via the given split mode, shared by
// every tab with an edit-file chord (Tickets, Queue) so the launched command
// is identical regardless of which tab's model calls it. readOnly only takes
// effect for the vi family; other editors have no common flag for it.
func editTicketFile(worktreeRoot string, settings ui.Settings, path string, splitType terminalrun.SplitType, readOnly bool) tea.Cmd {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return notify.Warning("$EDITOR is not set")
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return notify.Warning("$EDITOR is empty")
	}

	args := ui.EditorLaunchArgs(parts[0], parts[1:], path, 0)
	if readOnly && isViEditor(parts[0]) {
		args = append([]string{"-R"}, args...)
	}
	return terminalrun.CommandWithSplit(worktreeRoot, settings.Terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return EditFileFinishedMsg{Err: err, SplitApp: splitApp}
	})
}

func isViEditor(bin string) bool {
	switch filepath.Base(bin) {
	case "vi", "vim", "nvim":
		return true
	}
	return false
}
