package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
)

func (s *Server) sessionLinkWrite(w http.ResponseWriter, r *http.Request) {
	var req LinkSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.linkSession(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// linkSession sets the session's Session ticket, qualifying a bare address
// with the session's project, and marks it Design when the ticket is on a
// map epic. An existing Design flag stays: a new design session links to an
// epic Chart has not written yet.
func (s *Server) linkSession(req LinkSessionRequest) (SessionResult, error) {
	if req.Ticket == "" {
		return sessionRefusal(ReasonSessionBadRequest, "a link needs a ticket address"), nil
	}
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	ticket := req.Ticket
	if !strings.Contains(ticket, ":") {
		ticket = meta.Project + ":" + ticket
	}
	design := s.onMapEpic(ticket)
	var info SessionInfo
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.Ticket, m.Design = ticket, m.Design || design
		s.recordConversation(*m)
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Session: &info}, nil
}

func (s *Server) sessionStepWrite(w http.ResponseWriter, r *http.Request) {
	var req ReportStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.reportSessionStep(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// reportSessionStep saves the Design step an agent says it is running,
// replacing any earlier report.
func (s *Server) reportSessionStep(req ReportStepRequest) (SessionResult, error) {
	if req.Step == "" {
		return sessionRefusal(ReasonSessionBadRequest, "a step report needs a step"), nil
	}
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == SessionExited {
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info SessionInfo
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.ReportedStep, m.ReportedTitle = req.Step, req.Title
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Session: &info}, nil
}

// stampDesignStart records the first design session on the map epic behind
// address. The stamp is write-once, so every later session leaves it.
func (s *Server) stampDesignStart(address string, now time.Time) error {
	epicDir, ok, err := s.epicDir(address)
	if err != nil || !ok {
		return err
	}
	return tickets.StampEpicDesignStarted(epicDir, now)
}

// epicDir resolves address ("project:epic" or "project:epic/NN") to its epic
// directory; ok is false when the address names no epic.
func (s *Server) epicDir(address string) (dir string, ok bool, err error) {
	project, rest, found := strings.Cut(address, ":")
	epic, _, _ := strings.Cut(rest, "/")
	if !found || epic == "" {
		return "", false, nil
	}
	projectDir, err := s.projectDir(project)
	if err != nil {
		return "", false, err
	}
	return filepath.Join(projectDir, epic), true, nil
}

// onMapEpic reports whether address ("project:epic" or "project:epic/NN")
// is on a map epic; an unknown project or epic is not.
func (s *Server) onMapEpic(address string) bool {
	dir, ok, err := s.epicDir(address)
	return err == nil && ok && tickets.IsMapEpic(dir)
}
