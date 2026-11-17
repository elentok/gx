package server

import (
	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/wire"
)

// newChatDrafts builds the chat draft routes' state. It is a free function so
// Server gains no method.
func newChatDrafts(s *Server) *chatdrafts.ChatDrafts {
	return chatdrafts.NewChatDrafts(chatdrafts.Deps{
		Log: s.log, StateDir: s.cfg.StateDir, TicketStore: s.cfg.TicketStore, Events: s.events,
		Projects: s.projects, Workspaces: s.workspaces,
		Sessions: func() ([]wire.SessionInfo, error) { return s.sessions.ListSessions() },
	})
}
