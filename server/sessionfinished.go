package server

import (
	"path/filepath"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets/schema"
)

// sessionFinished reports whether the ticket behind s's iteration label has
// iteration_status finished. The label can't be parsed back to a ticket (long
// epic names are hashed), so it is matched against every indexed ticket, and
// the status is read from the file: the index may lag the agent's own report.
func (s *Server) sessionFinished(sess agentrunner.Session) bool {
	for _, epics := range s.idx.allEpics() {
		for _, e := range epics {
			epic := filepath.Base(e.Path)
			for _, t := range e.Tickets {
				if label, _, _ := ralphloop.IterationIdentity(epic, t.Identifier, ""); label != sess.Label {
					continue
				}
				parsed, err := schema.ParseTicket(t.Path)
				return err == nil && parsed.IterationStatus == schema.IterationStatusFinished
			}
		}
	}
	return false
}
