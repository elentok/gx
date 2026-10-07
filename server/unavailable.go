package server

import (
	"os"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// NoticeProjectUnavailable is the notify kind for a project whose path is gone.
const NoticeProjectUnavailable = "project-unavailable"

// VerdictProjectUnavailable is the explain verdict for a ticket of a project
// whose path is missing.
const VerdictProjectUnavailable = "project unavailable"

// unavailableNotes latches the projects already announced as unavailable, so
// one that stays missing notifies once and not on every poll. A project that
// comes back leaves the set, so a second disappearance notifies again.
type unavailableNotes struct {
	mu   sync.Mutex
	seen map[string]bool
}

// mark records project as unavailable and reports whether that is new.
func (u *unavailableNotes) mark(project string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.seen[project] {
		return false
	}
	if u.seen == nil {
		u.seen = map[string]bool{}
	}
	u.seen[project] = true
	return true
}

func (u *unavailableNotes) clear(project string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.seen, project)
}

// missingPath returns the registered path of the project in dir when it no
// longer exists. A project with no path (the built-in scratch one) is never missing.
func missingPath(dir string) (string, bool) {
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.Repo == nil {
		return "", false
	}
	_, err = os.Stat(*pf.Repo)
	return *pf.Repo, os.IsNotExist(err)
}

// unavailablePath reports whether project's path is missing, and what it was.
func (s *Server) unavailablePath(project string) (string, bool) {
	dir, err := s.projectDir(project)
	if err != nil {
		return "", false
	}
	return missingPath(dir)
}

// checkProjects looks at every registered project's path: at start and on every
// poll. The first poll that finds one missing sends the one notification; the
// claim and explain paths read the path themselves, so they never lag it.
func (s *Server) checkProjects() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		name := tickets.ProjectName(dir)
		path, missing := missingPath(dir)
		if !missing {
			s.unavailable.clear(name)
			continue
		}
		if s.unavailable.mark(name) {
			s.log.Warn("project unavailable", "project", name, "path", path)
			s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeProjectUnavailable, Emoji: "📁", Title: "project " + name + " unavailable",
				Detail: path + " does not exist; its tickets will not run. Fix it with gx project set-path " + name + " <path>"})
		}
	}
}
