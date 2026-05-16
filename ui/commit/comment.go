package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/comments"
	"github.com/elentok/gx/ui/notify"
)

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
