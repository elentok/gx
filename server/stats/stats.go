// Package stats is the stats read model behind GET /v1/stats: throughput,
// friction, cost and grilling over a day range, built from the ticket store,
// the conversation records and the token ledger.
package stats

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/ticketmigrate"
	"github.com/elentok/gx/tokenledger"
)

// Ledger is the token ledger as the cost chart reads it.
type Ledger interface {
	Day(day string) ([]tokenledger.Row, error)
	FirstRebuiltDay() (string, error)
	FirstLiveDay() (string, error)
}

// Burns gives the limit-burn series.
type Burns interface {
	BurnStats(n int, now time.Time) wire.StatsLimitBurns
}

// MapEpics tells whether a ticket address is on a map epic. The root
// implements it until sessions move out.
type MapEpics interface {
	OnMapEpic(address string) bool
}

// Deps is what the stats read model needs from the root.
type Deps struct {
	TicketStore      string
	ConversationsDir string
	Tokens           Ledger
	Burns            Burns
	MapEpics         MapEpics
}

// Stats serves the stats payload.
type Stats struct{ Deps }

// New builds the stats read model.
func New(d Deps) *Stats { return &Stats{Deps: d} }

var statsRanges = []int{wire.StatsRangeWeek, wire.StatsRangeMonth, wire.StatsRangeQuarter}

func (s *Stats) read(w http.ResponseWriter, r *http.Request) {
	n := wire.StatsDefaultRange
	if raw := r.URL.Query().Get("range"); raw != "" {
		var err error
		if n, err = strconv.Atoi(raw); err != nil || !slices.Contains(statsRanges, n) {
			http.Error(w, "range must be 7, 30 or 90", http.StatusBadRequest)
			return
		}
	}
	project := r.URL.Query().Get("project")
	all, err := ticketmigrate.LoadStoreEpics(s.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recs, err := conversations.List(s.ConversationsDir)
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
	out.Grilling = buildGrilling(recs, s.MapEpics.OnMapEpic, limits.NewStatsWindow(n, now), project)
	out.LimitBurn = s.Burns.BurnStats(n, now)
	wire.WriteJSON(w, out)
}
