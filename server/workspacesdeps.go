package server

import (
	"fmt"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// workspacesDirectory answers the workspace store's lookups from the index and the sessions.
type workspacesDirectory struct{ s *Server }

// workspacesWithNeedsYou lists the workspaces with each one's Needs-you count,
// and the total of every Needs-you card (One-off cards included).
func (s *Server) workspacesWithNeedsYou() ([]wire.Workspace, int) {
	ws := s.workspaces.Listed()
	perWorkspace, total := s.epics.NeedsYouCounts()
	for i := range ws {
		ws[i].NeedsYou = perWorkspace[ws[i].Name]
	}
	return ws, total
}

// Epic finds the epic behind a `project:epic` key.
func (d workspacesDirectory) Epic(key string) (tickets.Epic, bool) {
	project, name, ok := tickets.ParseEpicAddress(key)
	if !ok {
		return tickets.Epic{}, false
	}
	_, epics, ok := d.s.idx.ProjectEpics(project)
	if !ok {
		return tickets.Epic{}, false
	}
	for _, e := range epics {
		if e.Name == name {
			return e, true
		}
	}
	return tickets.Epic{}, false
}

// DesignExists reports whether key is a map epic: a Design.
func (d workspacesDirectory) DesignExists(key string) bool {
	e, ok := d.Epic(key)
	return ok && e.IsMap
}

func (d workspacesDirectory) RootQueued(key string) bool {
	root, err := queue.ParseRootRef(key)
	return err == nil && d.s.queue.RootQueued(root)
}

func (d workspacesDirectory) StopSession(id string) error {
	res, err := d.s.sessions.StopSession(wire.SessionRequest{ID: id})
	if err != nil {
		return err
	}
	if res.Refused {
		return fmt.Errorf("stop session %s: %s", id, res.Message)
	}
	return nil
}

func (d workspacesDirectory) Sessions() []wire.SessionInfo {
	list, _ := d.s.queue.ListSessionsWithQueue()
	return list
}
