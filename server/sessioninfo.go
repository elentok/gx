package server

import (
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// chatSource is the chat's view of the server's sessions.
type chatSource struct{ s *Server }

func (c chatSource) SessionDir(id string) (string, chat.Meta, wire.SessionResult, error) {
	dir, m, res, err := c.s.sessions.SessionDir(id)
	return dir, m.ChatMeta(), res, err
}

func (c chatSource) ReadableSession(id string) (string, chat.Meta, wire.SessionResult, error) {
	dir, m, res, err := c.s.sessions.ReadableSession(id)
	return dir, m.ChatMeta(), res, err
}

func (c chatSource) ReadMeta(dir string) (chat.Meta, error) {
	m, err := sessions.ReadMeta(dir)
	return m.ChatMeta(), err
}

func (c chatSource) CheckProjectDir(project, dir string) (*wire.SessionResult, error) {
	return c.s.sessions.CheckProjectDir(project, dir)
}

func (c chatSource) DesignFinal(ticket string) bool { return c.s.prompt.DesignFinal(ticket) }
func (c chatSource) NoteFailure(id string)          { c.s.sessions.NoteFailure(id) }
func (c chatSource) Stopped() <-chan struct{}       { return c.s.sessions.Watches.Stop }
func (c chatSource) CodexHome() (string, error)     { return c.s.projects.CodexHome() }
func (c chatSource) Publish(event, id string)       { c.s.events.Publish(event, id) }
func (c chatSource) HasChatReader(h string) bool    { return c.s.settings.Load().HasChatReader(h) }
