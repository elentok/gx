package server

import (
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// chatSource is the chat's view of the server's sessions.
type chatSource struct{ s *Server }

// chatMeta is the part of a session's meta the chat reads.
func chatMeta(m sessionMeta) chat.Meta {
	return chat.Meta{
		ID: m.ID, Harness: m.Harness, Project: m.Project, Dir: m.Dir, Ticket: m.Ticket,
		ConversationID: m.ConversationID, Model: m.Model, Effort: m.Effort,
		Status: m.Status, Exit: m.Exit, RestoreCrashed: m.RestoreCrashed,
	}
}

func (c chatSource) SessionDir(id string) (string, chat.Meta, wire.SessionResult, error) {
	dir, m, res, err := c.s.SessionDir(id)
	return dir, chatMeta(m), res, err
}

func (c chatSource) ReadableSession(id string) (string, chat.Meta, wire.SessionResult, error) {
	dir, m, res, err := c.s.readableSession(id)
	return dir, chatMeta(m), res, err
}

func (c chatSource) ReadMeta(dir string) (chat.Meta, error) {
	m, err := readSessionMeta(dir)
	return chatMeta(m), err
}

func (c chatSource) CheckProjectDir(project, dir string) (*wire.SessionResult, error) {
	return c.s.checkProjectDir(project, dir)
}

func (c chatSource) DesignFinal(ticket string) bool { return c.s.designFinal(ticket) }
func (c chatSource) NoteFailure(id string)          { c.s.noteFailure(id) }
func (c chatSource) Stopped() <-chan struct{}       { return c.s.sessionWatches.stop }
func (c chatSource) CodexHome() (string, error)     { return c.s.projects.CodexHome() }
func (c chatSource) Publish(event, id string)       { c.s.events.Publish(event, id) }
func (c chatSource) HasChatReader(h string) bool    { return c.s.settings.Load().HasChatReader(h) }

// sessionInfo is m.info() plus the failure that needs a transcript read, the
// reported model and the running countdown.
func (s *Server) sessionInfo(m sessionMeta) wire.SessionInfo {
	cm := chatMeta(m)
	info := m.info()
	info.Model = s.sessionChat.ReportedModel(cm)
	info.ModelSwitched = chat.ModelSwitched(m.Model, info.Model)
	if m.Harness == string(harness.Codex) {
		info.Effort = s.sessionChat.ReportedCodexTurn(cm).Effort
	}
	if at := s.autoAdvances.deadline(m.ID); !at.IsZero() {
		info.AutoAdvanceAt = &at
	}
	info.Failure = s.sessionChat.Failure(cm)
	return info
}
