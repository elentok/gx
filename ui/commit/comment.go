package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/comments"
	"github.com/elentok/gx/ui/notify"
)

func (m *Model) commentLocationAndBody() (string, []string, string, bool) {
	loc, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr == "" {
		return loc, body, "", true
	}
	if len(m.diffModel.Data().RawLines) > 0 {
		return "", m.diffModel.Data().RawLines, "", true
	}
	return "", nil, string(yankErr), false
}

func (m *Model) cmdCreateCommentFromDiff() tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
		return notify.Warning("no file context for comment")
	}
	loc, body, errMsg, ok := m.commentLocationAndBody()
	if !ok {
		return notify.Warning(errMsg)
	}
	cmd, msg := comments.CmdOpenEditor(path, loc, body, m.worktreeRoot, ui.DetectTerminal(), func(err error, splitApp string) tea.Msg {
		return editCommentFinishedMsg{err: err, splitApp: splitApp}
	})
	return tea.Batch(notify.Info(msg), cmd)
}

func (m Model) handleEditCommentFinished(msg editCommentFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("comment edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return m, notify.Info("opened " + msg.splitApp + " split: comment editor")
	}
	m.reload()
	return m, notify.Info(ui.MessageClosed("comment editor"))
}
