package server

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) explain(w http.ResponseWriter, r *http.Request) {
	addr, err := tickets.ParseAddress(r.URL.Query().Get("address"), tickets.AddressContext{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ex, err := s.explainTicket(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, ex)
}

// explainTicket gives the stored ticket's verdict: the scheduler's own, then
// the server's queue and slot state for a ticket nothing else holds back.
func (s *Server) explainTicket(addr tickets.Address) (wire.Explanation, error) {
	epics, err := s.projects.Epics(addr.Project)
	if err != nil {
		return wire.Explanation{}, err
	}
	return s.explainIn(epics, addr)
}

// explainIn is explainTicket's lookup inside one project's epics.
func (s *Server) explainIn(epics []tickets.Epic, addr tickets.Address) (wire.Explanation, error) {
	for _, e := range epics {
		if e.Name != addr.Epic {
			continue
		}
		for _, t := range e.Tickets {
			if t.DisplayNumber() == addr.ID {
				return s.verdictOf(e, t, addr), nil
			}
		}
	}
	return wire.Explanation{}, &tickets.AddressError{Code: tickets.CodeUnknownTicket, Msg: "no ticket " + addr.String()}
}
