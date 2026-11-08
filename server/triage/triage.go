// Package triage tells the web when a project's Triage record changes. The
// record is a JSON file the CLI verbs write; the store watcher only reacts to
// .md files, so a ping and a rescan backstop feed the event.
package triage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	triagerec "github.com/elentok/gx/triage"
)

// Deps is what Triage copies when it is built.
type Deps struct {
	TicketStore string
	Projects    *projects.Registry
	Events      *broker.Broker
	// Sessions lists the sessions, to tell whether a Triage's session stopped.
	Sessions func() ([]wire.SessionInfo, error)
}

// Triage publishes triage-changed and serves the Triage view.
type Triage struct {
	ticketStore string
	projects    *projects.Registry
	events      *broker.Broker
	sessions    func() ([]wire.SessionInfo, error)

	// seen maps a project name to the live record's id and revision at the
	// last look, "" when it had none.
	mu   sync.Mutex
	seen map[string]string
}

// New builds Triage from d.
func New(d Deps) *Triage {
	return &Triage{ticketStore: d.TicketStore, projects: d.Projects, events: d.Events, sessions: d.Sessions, seen: map[string]string{}}
}

// Routes lists the ping and read routes. The getter is called per request.
func Routes(get func() *Triage) []wire.Route {
	return []wire.Route{
		{
			Pattern: "GET /v1/triage",
			Web:     wire.WebRead,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().readHandler(w, r) },
		},
		{
			Pattern: "POST /v1/triage/changed",
			Handler: func(w http.ResponseWriter, r *http.Request) { get().changedHandler(w, r) },
		},
	}
}

func (t *Triage) changedHandler(w http.ResponseWriter, r *http.Request) {
	var req wire.TriageChangedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := t.Changed(req.Project); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	wire.WriteJSON(w, wire.TriageChangedResult{Project: req.Project})
}

// Changed publishes triage-changed for project and records the revision, so
// the next rescan does not publish it again.
func (t *Triage) Changed(project string) error {
	dir, err := t.projects.Dir(project)
	if err != nil {
		return err
	}
	t.note(project, dir)
	t.events.Publish(wire.EventTriageChanged, project)
	return nil
}

// ScanRevisions is the rescan backstop: it publishes triage-changed for each
// project whose live record's revision differs from the last look.
func (t *Triage) ScanRevisions() {
	dirs, err := tickets.ProjectDirs(t.ticketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		if t.note(project, dir) {
			t.events.Publish(wire.EventTriageChanged, project)
		}
	}
}

// note stores the project's current key and reports whether it differs from
// the last one. A project first seen with no record is no change.
func (t *Triage) note(project, dir string) bool {
	key := ""
	if rec, err := triagerec.NewStore(dir).LoadLive(); err == nil && rec != nil {
		key = fmt.Sprintf("%s#%d", rec.ID, rec.Revision)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	prev := t.seen[project]
	t.seen[project] = key
	return prev != key
}
