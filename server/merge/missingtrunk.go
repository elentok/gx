package merge

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

const (
	NoticeTrunkMissing = "trunk-missing"
	NoticeTrunkFound   = "trunk-found"
)

// missingTrunks holds the projects in Missing trunk and why. Memory only: a
// restart with the trunk still missing announces it once more.
type missingTrunks struct {
	mu     sync.Mutex
	reason map[string]string
}

// set records project as missing and reports whether it just entered the state.
func (m *missingTrunks) set(project, reason string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, was := m.reason[project]
	if m.reason == nil {
		m.reason = map[string]string{}
	}
	m.reason[project] = reason
	return !was
}

// clear reports whether project was missing.
func (m *missingTrunks) clear(project string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, was := m.reason[project]
	delete(m.reason, project)
	return was
}

// keepOnly forgets every project not in projects, announcing nothing.
func (m *missingTrunks) keepOnly(projects map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for p := range m.reason {
		if !projects[p] {
			delete(m.reason, p)
		}
	}
}

func (m *missingTrunks) Get(project string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reason[project]
	return r, ok
}

// CheckTrunks resolves trunk for every project with a queued root, on every
// pass, so claimRoot can refuse a Missing trunk project before it derives any
// base. A project with nothing queued leaves the state silently: a re-queue
// after a fix must not announce a stale "trunk found".
func (s *Reconciler) CheckTrunks() {
	seen := map[string]bool{}
	defer s.TrunkMissing.keepOnly(seen)
	for _, item := range s.queue.List() {
		addr, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil || seen[addr.Project] {
			continue
		}
		seen[addr.Project] = true
		reason, missing := s.trunkProblem(addr.Project)
		if !missing {
			if s.TrunkMissing.clear(addr.Project) {
				s.log.Info("trunk found", "project", addr.Project)
				s.events.PublishEvent(wire.Event{Type: wire.EventTrunkFound, Address: addr.Project, Level: wire.EventLevelSuccess, Message: "trunk found, claiming resumes"})
				s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeTrunkFound, Emoji: "🌳", Title: "project " + addr.Project + ": trunk found",
					Detail: "trunk found, claiming resumes"})
			}
			continue
		}
		s.enterTrunkMissing(addr.Project, reason)
	}
}

// enterTrunkMissing puts project in Missing trunk, announcing only the entry.
func (s *Reconciler) enterTrunkMissing(project, reason string) {
	if !s.TrunkMissing.set(project, reason) {
		return
	}
	s.log.Warn("trunk missing", "project", project, "reason", reason)
	s.events.PublishEvent(wire.Event{Type: wire.EventTrunkMissing, Address: project, Level: wire.EventLevelWarning, Message: reason})
	s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeTrunkMissing, Emoji: "🪵", Title: "project " + project + ": trunk missing",
		Detail: reason + "; its tickets will not be claimed until it is fixed"})
}

// MigrationBlock reports a project's saved migration failure: its tickets are
// not claimed while the record stands. The record is read on every call so a
// clear by the CLI or a Fix agent ends the block with no restart.
func (s *Reconciler) MigrationBlock(project string) (string, bool) {
	failed, err := migratefail.Read(s.cfg.StateDir, []string{project})
	if err != nil || len(failed) == 0 {
		return "", false
	}
	return failed[0].Error, true
}

// trunkProblem reports why project's trunk can't be used, if it can't. A
// project that is not found, has no repo or no VCS, or whose path is gone is
// not Missing trunk: it has no trunk, or some other state covers it.
func (s *Reconciler) trunkProblem(project string) (string, bool) {
	dir, err := s.projects.Dir(project)
	if err != nil {
		// An unreadable project.json fails the whole store scan, so its project
		// can't be found by name; its directory is still named after it.
		dir = filepath.Join(s.cfg.TicketStore, project)
	}
	pf, err := config.ReadProjectFile(dir)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		return fmt.Sprintf("project %q: unreadable project.json: %v", project, err), true
	}
	if pf.Repo == nil || (pf.VCS != nil && *pf.VCS == config.VCSNone) {
		return "", false
	}
	if _, gone := projects.MissingPath(dir); gone {
		return "", false
	}
	if _, err := ticketgit.Trunk(dir, *pf.Repo); err != nil {
		return err.Error(), true
	}
	return "", false
}
