package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

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
// map epic.
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
		m.Ticket, m.Design = ticket, design
		s.recordConversation(*m)
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Session: &info}, nil
}

// onMapEpic reports whether address ("project:epic" or "project:epic/NN")
// is on a map epic; an unknown project or epic is not.
func (s *Server) onMapEpic(address string) bool {
	project, rest, ok := strings.Cut(address, ":")
	epic, _, _ := strings.Cut(rest, "/")
	if !ok || epic == "" {
		return false
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return false
	}
	return tickets.IsMapEpic(filepath.Join(dir, epic))
}
