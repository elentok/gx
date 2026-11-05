package status

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
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/yankfmt"
)

var stageClipboardWrite = clip.Write

// diffActions holds the yank, edit and comment actions on the current
// selection. It keeps no state: everything it reads arrives in a diffActionCtx.
type diffActions struct{}

// diffActionCtx carries the Model state an action reads.
type diffActionCtx struct {
	worktreeRoot string
	home         string
	terminal     ui.Terminal
	focus        focusPane
	file         statusDiffFileSelection
	hasFile      bool
	diffarea     *diffarea.Model
}

func (diffActions) yankFilename(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	if err := stageClipboardWrite(c.file.Path); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked filename")
}

func (a diffActions) yankLocationOnly(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	if c.focus == focusFiletree {
		if err := stageClipboardWrite(yankfmt.FormatYankLocation(c.file.Path, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked location")
	}
	loc, _, cmd, ok := a.focusedLocationAndBody(c)
	if !ok {
		return cmd
	}
	if err := stageClipboardWrite(yankfmt.FormatYankLocation(c.file.Path, loc)); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked location")
}

func (a diffActions) yankAllContext(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file selected")
	}
	if c.focus == focusFiletree {
		if err := stageClipboardWrite(yankfmt.FormatYankLocation(c.file.Path, "")); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked for AI agent")
	}

	loc, body, cmd, ok := a.focusedLocationAndBody(c)
	if !ok {
		return cmd
	}
	text := yankfmt.FormatForAgent(c.file.Path, loc, body)
	if err := stageClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked for AI agent")
}

func (diffActions) yankContentOnly(c diffActionCtx) tea.Cmd {
	if c.focus == focusFiletree {
		return notify.Warning("no diff selection to yank")
	}
	diffviewModel := c.diffarea.ActiveSectionModel()
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

func (diffActions) focusedLocationAndBody(c diffActionCtx) (string, []string, tea.Cmd, bool) {
	diffviewModel := c.diffarea.ActiveSectionModel()
	loc, body, yankErr := diffviewModel.FocusedLocationAndBody()
	if yankErr != "" {
		return "", nil, notify.Warning(string(yankErr)), false
	}
	return loc, body, nil, true
}

func (diffActions) commentLocationAndBody(c diffActionCtx) (string, []string, string, bool) {
	diffModel := c.diffarea.ActiveSectionModel()
	loc, body, yankErr := diffModel.FocusedLocationAndBody()
	if yankErr == "" {
		return loc, body, "", true
	}
	if len(diffModel.DataRef().RawLines) > 0 {
		return "", diffModel.DataRef().RawLines, "", true
	}
	return "", nil, string(yankErr), false
}

func (a diffActions) cmdEditSelectedFile(c diffActionCtx, splitType terminalrun.SplitType) tea.Cmd {
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
	target := filepath.Join(c.worktreeRoot, c.file.Path)
	line := a.editorLineForCurrentSelection(c)
	args := ui.EditorLaunchArgs(parts[0], parts[1:], target, line)
	return terminalrun.CommandWithSplit(c.worktreeRoot, c.terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return editFileFinishedMsg{err: err, splitApp: splitApp}
	})
}

func (a diffActions) cmdCreateCommentFromDiff(c diffActionCtx) tea.Cmd {
	if !c.hasFile {
		return notify.Warning("no file context for comment")
	}
	loc, body, errMsg, ok := a.commentLocationAndBody(c)
	if !ok {
		return notify.Warning(errMsg)
	}
	cmd, msg := comments.CmdOpenEditor(c.home, c.file.Path, loc, body, c.worktreeRoot, c.terminal, func(err error, splitApp string) tea.Msg {
		return editCommentFinishedMsg{err: err, splitApp: splitApp}
	})
	if cmd == nil {
		return notify.Warning(msg)
	}
	return cmd
}

func (diffActions) editorLineForCurrentSelection(c diffActionCtx) int {
	if c.focus != focusDiff {
		return 0
	}
	diff := c.diffarea.ActiveSectionModel().DataRef()
	if c.diffarea.NavMode() == diffview.NavModeLine {
		if diff.ActiveLine < 0 || diff.ActiveLine >= len(diff.Parsed.Changed) {
			return 0
		}
		cl := diff.Parsed.Changed[diff.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine
		}
		return cl.OldLine
	}
	if diff.ActiveHunk < 0 || diff.ActiveHunk >= len(diff.Parsed.Hunks) {
		return 0
	}
	h := diff.Parsed.Hunks[diff.ActiveHunk]
	if h.NewStart > 0 {
		return h.NewStart
	}
	return h.OldStart
}

// handleEditFileFinished reports the editor's outcome; refresh runs when the
// editor ran in place.
func (diffActions) handleEditFileFinished(msg editFileFinishedMsg, refresh func() tea.Cmd) tea.Cmd {
	if msg.err != nil {
		return notify.Error("edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return notify.Info("opened " + msg.splitApp + " split: editor")
	}
	return refresh()
}

func (diffActions) handleEditCommentFinished(msg editCommentFinishedMsg, refresh func() tea.Cmd) tea.Cmd {
	if msg.err != nil {
		return notify.Error("comment edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return notify.Info("opened " + msg.splitApp + " split: comment editor")
	}
	return refresh()
}
