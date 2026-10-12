package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const (
	// EventTrunkMissing streams once when a project with queued roots enters
	// Missing trunk. Address is the project; Message is the reason.
	EventTrunkMissing = "trunk-missing"
	// EventTrunkFound streams once when that project's trunk resolves again.
	EventTrunkFound = "trunk-found"

	NoticeTrunkMissing = "trunk-missing"
	NoticeTrunkFound   = "trunk-found"

	// VerdictTrunkMissing is the explain verdict for a queued ticket of a
	// project in Missing trunk.
	VerdictTrunkMissing = "trunk missing"
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

func (m *missingTrunks) get(project string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reason[project]
	return r, ok
}

// checkTrunks resolves trunk for every project with a queued root, on every
// pass, so claimRoot can refuse a Missing trunk project before it derives any
// base. A project with nothing queued keeps whatever state it had: nothing of
// it can be claimed, and explain only asks about queued tickets.
func (s *Server) checkTrunks() {
	seen := map[string]bool{}
	for _, item := range s.queued.list() {
		addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
		if err != nil || seen[addr.Project] {
			continue
		}
		seen[addr.Project] = true
		reason, missing := s.trunkProblem(addr.Project)
		if !missing {
			if s.trunkMissing.clear(addr.Project) {
				s.log.Info("trunk found", "project", addr.Project)
				s.events.publishEvent(Event{Type: EventTrunkFound, Address: addr.Project, Level: EventLevelSuccess, Message: "trunk found, claiming resumes"})
				s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeTrunkFound, Emoji: "🌳", Title: "project " + addr.Project + ": trunk found",
					Detail: "trunk found, claiming resumes"})
			}
			continue
		}
		if s.trunkMissing.set(addr.Project, reason) {
			s.log.Warn("trunk missing", "project", addr.Project, "reason", reason)
			s.events.publishEvent(Event{Type: EventTrunkMissing, Address: addr.Project, Level: EventLevelWarning, Message: reason})
			s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeTrunkMissing, Emoji: "🪵", Title: "project " + addr.Project + ": trunk missing",
				Detail: reason + "; its tickets will not be claimed until it is fixed"})
		}
	}
}

// trunkProblem reports why project's trunk can't be used, if it can't. A
// project that is not found, has no repo or no VCS, or whose path is gone is
// not Missing trunk: it has no trunk, or some other state covers it.
func (s *Server) trunkProblem(project string) (string, bool) {
	dir, err := s.projectDir(project)
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
	if _, gone := missingPath(dir); gone {
		return "", false
	}
	if _, err := tickets.Trunk(dir, *pf.Repo); err != nil {
		return err.Error(), true
	}
	return "", false
}
