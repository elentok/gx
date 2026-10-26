package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the designs open route, beside the session ones.
const (
	ReasonDesignNotMapEpic  = "not-map-epic"
	ReasonDesignUnknownEpic = "unknown-epic"
	ReasonDesignEpicExists  = "epic-exists"
	// ReasonDesignStartFailed: a New Design wrote its map but could not start
	// the session.
	ReasonDesignStartFailed = "design-start-failed"
)

// DesignOpenRequest is the body of POST /v1/designs/open. Epic is a map
// epic's address ("project:epic") or one of its tickets ("project:epic/NN").
// New stops the Design's live session and starts a fresh one; State is the
// token of the live session the web saw, required on the web mux for New.
type DesignOpenRequest struct {
	Epic  string `json:"epic"`
	New   bool   `json:"new,omitempty"`
	State string `json:"state,omitempty"`
	// NewDesign starts a Design for Idea that has no map yet: Epic is then the
	// slug the map will take in Project, and no epic is created.
	NewDesign bool   `json:"new_design,omitempty"`
	Idea      string `json:"idea,omitempty"`
	Project   string `json:"project,omitempty"`
}

var (
	designTicketRe = regexp.MustCompile(`^\d+[[:alpha:]]?\d*$`)
	designSlugRe   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func (s *Server) designOpenWrite(w http.ResponseWriter, r *http.Request) {
	s.designOpenHandler(false)(w, r)
}

// designOpenWeb is designOpenWrite for the web mux: New needs the state token.
func (s *Server) designOpenWeb() http.HandlerFunc { return s.designOpenHandler(true) }

func (s *Server) designOpenHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DesignOpenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := s.openDesign(req, web)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// openDesign returns the Design's newest live session, or creates one. Calls
// for one epic run one at a time, so a double click starts one session.
func (s *Server) openDesign(req DesignOpenRequest, web bool) (SessionResult, error) {
	if req.NewDesign {
		return s.openNewDesign(req)
	}
	epicAddr, ticket, _ := strings.Cut(req.Epic, "/")
	project, epic, ok := strings.Cut(epicAddr, ":")
	if !ok || project == "" || epic == "" || (strings.Contains(req.Epic, "/") && !designTicketRe.MatchString(ticket)) {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic or project:epic/NN)", req.Epic)), nil
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return sessionRefusal(ReasonDesignUnknownEpic, err.Error()), nil
	}
	epicDir := filepath.Join(dir, epic)
	if fi, err := os.Stat(epicDir); err != nil || !fi.IsDir() {
		return sessionRefusal(ReasonDesignUnknownEpic, fmt.Sprintf("no epic %s", epicAddr)), nil
	}
	if !tickets.IsMapEpic(epicDir) {
		return sessionRefusal(ReasonDesignNotMapEpic, fmt.Sprintf("%s is not a map epic", epicAddr)), nil
	}

	mu, _ := s.designOpens.LoadOrStore(epicAddr, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	live, err := s.liveDesignSession(epicAddr)
	if err != nil {
		return SessionResult{}, err
	}
	if live != nil && !req.New {
		return SessionResult{Session: live}, nil
	}
	if live != nil {
		if web && req.State != SessionStateToken(*live) {
			return sessionRefusal(ReasonStale, "the session changed; review its current state"), nil
		}
		if res, err := s.stopSession(SessionRequest{ID: live.ID}); err != nil || (res.Refused && res.Reason != ReasonSessionNotLive) {
			return res, err
		}
	}

	checkout := tickets.TrunkCheckout(s.cfg.TicketStore, project)
	if checkout == "" {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", project)), nil
	}
	prompt := "/gx-design " + epicAddr
	if ticket != "" {
		prompt += "/" + ticket
	}
	return s.createSession(CreateSessionRequest{
		Project: project,
		Dir:     checkout,
		Name:    epic,
		Ticket:  epicAddr,
		Prompt:  prompt,
		Harness: s.settings.Load().chatHarness(),
	})
}

// openNewDesign starts a design session for an idea with no map yet: Epic is
// the slug the map will take in Project. It creates no epic; Chart writes it.
func (s *Server) openNewDesign(req DesignOpenRequest) (SessionResult, error) {
	slug := req.Epic
	if !designSlugRe.MatchString(slug) {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not a slug (want lowercase words joined by -)", slug)), nil
	}
	idea := strings.TrimSpace(normalizeAnswer(req.Idea))
	switch {
	case idea == "":
		return sessionRefusal(ReasonEmptyPrompt, "an idea is required"), nil
	case len(idea) > maxSessionPromptBytes:
		return sessionRefusal(ReasonPromptTextTooLong, fmt.Sprintf("idea is over %d bytes", maxSessionPromptBytes)), nil
	}
	dir, err := s.projectDir(req.Project)
	if err != nil {
		return sessionRefusal(ReasonSessionBadRequest, err.Error()), nil
	}
	addr := req.Project + ":" + slug

	mu, _ := s.designOpens.LoadOrStore(addr, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	if _, err := os.Stat(filepath.Join(dir, slug)); err == nil {
		return sessionRefusal(ReasonDesignEpicExists, fmt.Sprintf("%s already has an epic %s", req.Project, slug)), nil
	}
	checkout := tickets.TrunkCheckout(s.cfg.TicketStore, req.Project)
	if checkout == "" {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", req.Project)), nil
	}

	// From here the map exists, so every failure carries EpicCreated.
	createdFailure := func(res SessionResult, err error) (SessionResult, error) {
		if err != nil {
			res = sessionRefusal(ReasonDesignStartFailed, err.Error())
		}
		if res.Refused {
			res.EpicCreated = true
		}
		return res, nil
	}
	epicPath, err := tickets.CreateEpic(dir, slug, tickets.EpicFields{Kind: tickets.KindMap, Status: string(schema.StatusOpen)}, "## Initial prompt\n\n"+idea+"\n")
	if err != nil {
		if epicPath == "" {
			return SessionResult{}, err
		}
		return createdFailure(SessionResult{}, err)
	}
	step := string(tickets.StepChart)
	if err := tickets.UpdateEpicFields(filepath.Join(epicPath, "ticket.md"), tickets.EpicUpdate{DesignStep: &step}); err != nil {
		return createdFailure(SessionResult{}, err)
	}
	return createdFailure(s.createSession(CreateSessionRequest{
		Project: req.Project,
		Dir:     checkout,
		Name:    slug,
		Ticket:  addr,
		Prompt:  "/gx-design " + addr,
		Harness: s.settings.Load().chatHarness(),
	}))
}

// Refusal reason of the designs delete route, beside the unknown and not-map ones.
const ReasonDesignNotUncharted = "not-uncharted"

// DesignDeleteRequest is the body of POST /v1/designs/delete. Epic is the map
// epic's address ("project:epic").
type DesignDeleteRequest struct {
	Epic string `json:"epic"`
}

// designUncharted is the one rule behind EpicDetail.Deletable and the delete
// route: a map with no Destination, no tickets of any status and no live
// design session.
func (s *Server) designUncharted(epicAddr string, e tickets.Epic) (bool, error) {
	if !e.IsMap || schema.Section(e.Body, "Destination") != "" || len(e.Tickets) > 0 {
		return false, nil
	}
	live, err := s.liveDesignSession(epicAddr)
	return live == nil, err
}

func (s *Server) designDeleteWrite(w http.ResponseWriter, r *http.Request) {
	var req DesignDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.deleteDesign(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// deleteDesign removes an uncharted design's epic directory. It re-checks the
// rule from disk under the lock designs open takes, so a session or a chart
// that lands first wins. Exited session records stay.
func (s *Server) deleteDesign(req DesignDeleteRequest) (SessionResult, error) {
	project, epic, ok := strings.Cut(req.Epic, ":")
	if !ok || project == "" || epic == "" || strings.Contains(epic, "/") {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic)", req.Epic)), nil
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return sessionRefusal(ReasonDesignUnknownEpic, err.Error()), nil
	}

	mu, _ := s.designOpens.LoadOrStore(req.Epic, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		return SessionResult{}, err
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epic })
	if i < 0 {
		return sessionRefusal(ReasonDesignUnknownEpic, fmt.Sprintf("no epic %s", req.Epic)), nil
	}
	if !epics[i].IsMap {
		return sessionRefusal(ReasonDesignNotMapEpic, fmt.Sprintf("%s is not a map epic", req.Epic)), nil
	}
	uncharted, err := s.designUncharted(req.Epic, epics[i])
	if err != nil {
		return SessionResult{}, err
	}
	if !uncharted {
		return sessionRefusal(ReasonDesignNotUncharted, fmt.Sprintf("%s is charted or has a live session", req.Epic)), nil
	}
	if err := os.RemoveAll(epics[i].Path); err != nil {
		return SessionResult{}, err
	}
	s.events.publish(EventTicketChanged, req.Epic)
	return SessionResult{}, nil
}

// liveDesignSession is the session of the Design at epicAddr that has not
// exited and has the latest RunStartedAt, or nil. The Design page picks by the
// same key (webui/src/design/view.ts).
func (s *Server) liveDesignSession(epicAddr string) (*SessionInfo, error) {
	list, err := s.listSessions()
	if err != nil {
		return nil, err
	}
	var live *SessionInfo
	for i := range list {
		in := &list[i]
		if in.Design && in.Status.State != SessionExited && epicAddress(in.Ticket) == epicAddr &&
			(live == nil || !in.RunStartedAt.Before(live.RunStartedAt)) {
			live = in
		}
	}
	return live, nil
}
