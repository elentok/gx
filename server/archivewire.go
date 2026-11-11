package server

import (
	"fmt"

	"github.com/elentok/gx/server/archive"
	"github.com/elentok/gx/server/sessions"
)

// newArchive wires Archive to the workspace store, the index and the sessions.
func newArchive(s *Server) *archive.Archive {
	return archive.New(archive.Deps{
		Directory:     workspacesDirectory{s},
		TicketStore:   s.cfg.TicketStore,
		Events:        s.events,
		Workspace:     s.workspaces.Workspace,
		Unhold:        s.workspaces.Unhold,
		SetMember:     s.workspaces.SetMember,
		Remove:        s.workspaces.Remove,
		CleanupDesign: s.designs.CleanupDesign,
		Log:           s.log,
		RekeySession:  s.rekeySession,
	})
}

// rekeySession rewrites a Session's ticket and fix_of in its meta.
func (s *Server) rekeySession(id string, rekey func(ticket, fixOf string) (string, string)) error {
	dir, _, res, err := s.sessions.SessionDir(id)
	if err != nil {
		return err
	}
	if res.Refused {
		return fmt.Errorf("session %s: %s", id, res.Message)
	}
	return s.sessions.UpdateSessionMeta(dir, id, func(m *sessions.Meta) bool {
		ticket, fixOf := rekey(m.Ticket, m.FixOf)
		changed := ticket != m.Ticket || fixOf != m.FixOf
		m.Ticket, m.FixOf = ticket, fixOf
		return changed
	})
}
