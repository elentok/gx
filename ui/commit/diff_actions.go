package commit

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/clip"
	"github.com/elentok/gx/ui/comments"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/yankfmt"
)

var commitClipboardWrite = clip.Write

type editFileFinishedMsg struct {
	err      error
	splitApp string
}

type commentLocation struct {
	loc    string
	body   []string
	errMsg string
	ok     bool
}

// diffActions holds the yank, edit and comment actions on the current
// selection. It keeps no state: everything it reads arrives in a diffActionCtx.
type diffActions struct{}

// diffActionCtx carries the Model state an action reads.
type diffActionCtx struct {
	worktreeRoot string
	home         string
	terminal     ui.Terminal // for editor splits
	commentTerm  ui.Terminal // detected from the environment, for the comment editor
	focusDiff    bool
	filePath     string
	hasFile      bool
	diff         *diffPane
}

func (diffActions) yankFilename(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	if err := commitClipboardWrite(c.filePath); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked filename")
}

func (a diffActions) yankLocationOnly(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	if !c.focusDiff {
		if err := commitClipboardWrite(yankfmt.FormatYankLocation(c.filePath, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked location")
	}
	loc, _, cmd, ok := a.focusedLocationAndBody(c)
	if !ok {
		return cmd
	}
	if err := commitClipboardWrite(yankfmt.FormatYankLocation(c.filePath, loc)); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked location")
}

func (a diffActions) yankAllContext(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	if !c.focusDiff {
		if err := commitClipboardWrite(yankfmt.FormatYankLocation(c.filePath, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked for AI agent")
	}
	loc, body, cmd, ok := a.focusedLocationAndBody(c)
	if !ok {
		return cmd
	}
	text := yankfmt.FormatForAgent(c.filePath, loc, body)
	if err := commitClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked for AI agent")
}

func (diffActions) yankContentOnly(c diffActionCtx) tea.Cmd {
	if !c.focusDiff {
		return notify.Warning("no diff selection to yank")
	}
	_, body, yankErr := c.diff.diffModel.FocusedLocationAndBody()
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

func (diffActions) focusedLocationAndBody(c diffActionCtx) (string, []string, tea.Cmd, bool) {
	loc, body, yankErr := c.diff.diffModel.FocusedLocationAndBody()
	if yankErr != "" {
		return "", nil, notify.Warning(string(yankErr)), false
	}
	return loc, body, nil, true
}

func (diffActions) commentLocationAndBody(c diffActionCtx) commentLocation {
	loc, body, yankErr := c.diff.diffModel.FocusedLocationAndBody()
	if yankErr == "" {
		return commentLocation{loc: loc, body: body, ok: true}
	}
	if len(c.diff.diffModel.Data().RawLines) > 0 {
		return commentLocation{body: c.diff.diffModel.Data().RawLines, ok: true}
	}
	return commentLocation{errMsg: string(yankErr)}
}

func (a diffActions) cmdCreateCommentFromDiff(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file context for comment")
	}
	cl := a.commentLocationAndBody(c)
	if !cl.ok {
		return notify.Warning(cl.errMsg)
	}
	cmd, msg := comments.CmdOpenEditor(c.home, c.filePath, cl.loc, cl.body, c.worktreeRoot, c.commentTerm, func(err error, splitApp string) tea.Msg {
		return editCommentFinishedMsg{err: err, splitApp: splitApp}
	})
	if cmd == nil {
		return notify.Warning(msg)
	}
	return cmd
}

func (diffActions) cmdEditSelectedFile(c diffActionCtx, splitType terminalrun.SplitType) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return notify.Warning("$EDITOR is not set")
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return notify.Warning("$EDITOR is empty")
	}
	target := filepath.Join(c.worktreeRoot, c.filePath)
	line := c.diff.editorLineForCurrentSelection(c.focusDiff)
	args := ui.EditorLaunchArgs(parts[0], parts[1:], target, line)
	return terminalrun.CommandWithSplit(c.worktreeRoot, c.terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return editFileFinishedMsg{err: err, splitApp: splitApp}
	})
}

// handleEditFileFinished reports the editor's outcome; reload runs when the
// editor ran in place.
func (diffActions) handleEditFileFinished(msg editFileFinishedMsg, reload func()) tea.Cmd {
	if msg.err != nil {
		return notify.Error("edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return notify.Info("opened " + msg.splitApp + " split: editor")
	}
	reload()
	return nil
}

func (diffActions) handleEditCommentFinished(msg editCommentFinishedMsg, reload func()) tea.Cmd {
	if msg.err != nil {
		return notify.Error("comment edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return notify.Info("opened " + msg.splitApp + " split: comment editor")
	}
	reload()
	return nil
}
