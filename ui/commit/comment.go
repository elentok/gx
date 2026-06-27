package commit

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/comments"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

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

func (m *Model) commentLocationAndBody() commentLocation {
	loc, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr == "" {
		return commentLocation{loc: loc, body: body, ok: true}
	}
	if len(m.diffModel.Data().RawLines) > 0 {
		return commentLocation{body: m.diffModel.Data().RawLines, ok: true}
	}
	return commentLocation{errMsg: string(yankErr)}
}

func (m *Model) cmdCreateCommentFromDiff() tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
		return notify.Warning("no file context for comment")
	}
	cl := m.commentLocationAndBody()
	if !cl.ok {
		return notify.Warning(cl.errMsg)
	}
	cmd, msg := comments.CmdOpenEditor(path, cl.loc, cl.body, m.worktreeRoot, ui.DetectTerminal(), func(err error, splitApp string) tea.Msg {
		return editCommentFinishedMsg{err: err, splitApp: splitApp}
	})
	if cmd == nil {
		return notify.Warning(msg)
	}
	return cmd
}

func (m *Model) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
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
	target := filepath.Join(m.worktreeRoot, path)
	line := m.editorLineForCurrentSelection()
	args := ui.EditorLaunchArgs(parts[0], parts[1:], target, line)
	cmd := terminalrun.CommandWithSplit(m.worktreeRoot, m.settings.Terminal, splitType, parts[0], args, func(err error, splitApp string) tea.Msg {
		return editFileFinishedMsg{err: err, splitApp: splitApp}
	})
	return cmd
}

func (m Model) handleEditFileFinished(msg editFileFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return m, notify.Info("opened " + msg.splitApp + " split: editor")
	}
	m.reload()
	return m, nil
}

func (m Model) handleEditCommentFinished(msg editCommentFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("comment edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return m, notify.Info("opened " + msg.splitApp + " split: comment editor")
	}
	m.reload()
	return m, nil
}
