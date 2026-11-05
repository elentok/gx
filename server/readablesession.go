package server

import (
	"cmp"
	"strings"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// readableSession resolves a session read's id: a sessions-dir session, else a
// live Queue agent. Only the chat and screen reads use it; the write routes
// call sessionDir and so refuse a Queue agent id as an unknown session.
func (s *Server) readableSession(id string) (string, sessions.Meta, wire.SessionResult, error) {
	dir, meta, res, err := s.sessions.SessionDir(id)
	if !res.Refused || res.Reason != wire.ReasonSessionUnknown || !strings.HasPrefix(id, queue.AgentIDPrefix) {
		return dir, meta, res, err
	}
	for _, row := range s.queue.AgentRows() {
		if row.ID != id {
			continue
		}
		d, err := nativerunner.OpenDir(row.Holder)
		if err != nil {
			break
		}
		// No Ticket: it would make the chat read the agent as a design step.
		return row.Holder, sessions.Meta{
			ID:             id,
			Harness:        row.Harness,
			Project:        row.Project,
			Dir:            cmp.Or(d.Meta.Cwd, row.Dir),
			ConversationID: d.Meta.SessionID,
			Status:         row.Status,
		}, wire.SessionResult{}, nil
	}
	return dir, meta, res, nil
}
