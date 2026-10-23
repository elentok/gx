package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/elentok/gx/tickets"
)

// Refusal reasons of the designs open route, beside the session ones.
const (
	ReasonNotMapEpic  = "not-map-epic"
	ReasonUnknownEpic = "unknown-epic"
)

// DesignOpenRequest is the body of POST /v1/designs/open. Epic is a map
// epic's address ("project:epic") or one of its tickets ("project:epic/NN").
// New stops the Design's live session and starts a fresh one; State is the
// token of the live session the web saw, required on the web mux for New.
type DesignOpenRequest struct {
	Epic  string `json:"epic"`
	New   bool   `json:"new,omitempty"`
	State string `json:"state,omitempty"`
}

var designTicketRe = regexp.MustCompile(`^\d+[[:alpha:]]?\d*$`)

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
	epicAddr, ticket, _ := strings.Cut(req.Epic, "/")
	project, epic, ok := strings.Cut(epicAddr, ":")
	if !ok || project == "" || epic == "" || (strings.Contains(req.Epic, "/") && !designTicketRe.MatchString(ticket)) {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic or project:epic/NN)", req.Epic)), nil
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return sessionRefusal(ReasonUnknownEpic, err.Error()), nil
	}
	epicDir := filepath.Join(dir, epic)
	if fi, err := os.Stat(epicDir); err != nil || !fi.IsDir() {
		return sessionRefusal(ReasonUnknownEpic, fmt.Sprintf("no epic %s", epicAddr)), nil
	}
	if !tickets.IsMapEpic(epicDir) {
		return sessionRefusal(ReasonNotMapEpic, fmt.Sprintf("%s is not a map epic", epicAddr)), nil
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

// liveDesignSession is the newest session of the Design at epicAddr that has
// not exited, or nil.
func (s *Server) liveDesignSession(epicAddr string) (*SessionInfo, error) {
	list, err := s.listSessions()
	if err != nil {
		return nil, err
	}
	for i := len(list) - 1; i >= 0; i-- {
		if in := list[i]; in.Design && in.Status.State != SessionExited && epicAddress(in.Ticket) == epicAddr {
			return &in, nil
		}
	}
	return nil, nil
}
