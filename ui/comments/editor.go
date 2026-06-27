package comments

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/terminalrun"
)

// CmdOpenEditor writes a comment file from path/loc/body and opens it in
// $EDITOR. Returns the tea.Cmd to run (nil on failure) and a status message
// (error description on failure, empty on success — the editor takeover is its
// own feedback, so no toast is shown).
func CmdOpenEditor(
	path, loc string,
	body []string,
	worktreeRoot string,
	terminal ui.Terminal,
	makeMsg func(err error, splitApp string) tea.Msg,
) (tea.Cmd, string) {
	commentPath, err := Write(path, loc, body)
	if err != nil {
		return nil, "comment write failed: " + err.Error()
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return nil, "$EDITOR is not set (comment file: " + commentPath + ")"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return nil, "$EDITOR is empty"
	}
	args := append(parts[1:], commentPath)
	cmd := terminalrun.Command(worktreeRoot, terminal, parts[0], args, makeMsg)
	return cmd, ""
}
