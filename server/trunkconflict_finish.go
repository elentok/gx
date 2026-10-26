package server

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// filesWithMarkers are the files, relative to dir, that still hold a line
// starting a conflict marker. A file that is gone or unreadable holds none.
func filesWithMarkers(dir string, files []string) []string {
	var left []string
	for _, f := range files {
		body, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		for _, line := range bytes.Split(body, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("<<<<<<< ")) || bytes.HasPrefix(line, []byte(">>>>>>> ")) {
				left = append(left, f)
				break
			}
		}
	}
	return left
}

// dropTrunkConflictBackup deletes the backup ref of a trunk-conflict ticket
// that just went done: the resolution replaced what the ref kept safe.
func (s *Server) dropTrunkConflictBackup(addr tickets.Address) {
	e, ok := s.freshEpic(addr.Project, addr.Epic)
	if !ok || e.TrunkConflictBackup == "" {
		return
	}
	_, repo, err := s.projectOf(addr.Project)
	if err != nil {
		s.log.Warn("delete trunk-conflict backup", "ticket", addr, "err", err)
		return
	}
	if err := git.DeleteRef(repo, e.TrunkConflictBackup); err != nil {
		s.log.Warn("delete trunk-conflict backup", "ticket", addr, "ref", e.TrunkConflictBackup, "err", err)
	}
}

// backupInUse reports whether an open trunk-conflict ticket names ref as its
// backup; the prune keeps such a ref whatever its age.
func (s *Server) backupInUse(ref string) bool {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return true // cannot tell: keep it
	}
	for _, dir := range dirs {
		epics, err := tickets.LoadResolved(tickets.ProjectName(dir), dir)
		if err != nil {
			return true
		}
		for _, e := range epics {
			if e.TrunkConflictBackup != ref {
				continue
			}
			for _, t := range e.Tickets {
				if t.Type == string(schema.TypeTrunkConflict) && !e.RenderedStatus(t).Terminal() {
					return true
				}
			}
		}
	}
	return false
}
