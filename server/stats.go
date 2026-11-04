package server

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

var statsRanges = []int{wire.StatsRangeWeek, wire.StatsRangeMonth, wire.StatsRangeQuarter}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	n := wire.StatsDefaultRange
	if raw := r.URL.Query().Get("range"); raw != "" {
		var err error
		if n, err = strconv.Atoi(raw); err != nil || !slices.Contains(statsRanges, n) {
			http.Error(w, "range must be 7, 30 or 90", http.StatusBadRequest)
			return
		}
	}
	project := r.URL.Query().Get("project")
	all, err := tickets.LoadStoreEpics(s.cfg.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recs, err := conversations.List(s.cfg.ConversationsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().In(time.Local)
	led, err := s.costLedger(limits.NewStatsWindow(n, now))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := buildStats(all, n, project, now, led)
	out.Grilling = buildGrilling(recs, s.onMapEpic, limits.NewStatsWindow(n, now), project)
	out.LimitBurn = s.limits.BurnStats(n, now)
	writeJSON(w, out)
}
