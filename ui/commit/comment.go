package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/comments"
)

func (m *Model) commentLocationAndBody() (string, []string, bool) {
	loc, body, yankErr := m.diffModel.FocusedLocationAndBody()
	if yankErr == "" {
		return loc, body, true
	}
	if len(m.diffModel.Data().RawLines) > 0 {
		return "", m.diffModel.Data().RawLines, true
	}
	m.setStatus(string(yankErr))
	return "", nil, false
}

func (m *Model) cmdCreateCommentFromDiff() tea.Cmd {
	path, ok := m.selectedFile()
	if !ok {
		m.setStatus("no file context for comment")
		return nil
	}
	loc, body, ok := m.commentLocationAndBody()
	if !ok {
		return nil
	}
	cmd, msg := comments.CmdOpenEditor(path, loc, body, m.worktreeRoot, ui.DetectTerminal(), func(err error, splitApp string) tea.Msg {
		return editCommentFinishedMsg{err: err, splitApp: splitApp}
	})
	m.setStatus(msg)
	return cmd
}

func (m Model) handleEditCommentFinished(msg editCommentFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatus("comment edit failed: " + msg.err.Error())
		return m, nil
	}
	if msg.splitApp != "" {
		m.setStatus("opened " + msg.splitApp + " split: comment editor")
		return m, nil
	}
	m.setStatus(ui.MessageClosed("comment editor"))
	m.reload()
	return m, nil
}
