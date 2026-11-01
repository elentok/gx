package server

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
)

// The ranges the stats page offers, in days. The web UI reads them from the
// generated types. They stay separate declarations: tygo turns a const block
// into a union type named after the block.
const StatsRangeWeek = 7
const StatsRangeMonth = 30
const StatsRangeQuarter = 90
const StatsDefaultRange = StatsRangeMonth

var statsRanges = []int{StatsRangeWeek, StatsRangeMonth, StatsRangeQuarter}

// StatsAmount is one day of a token or spend series. NoData marks a day that
// had finished items and none carried a stamp; Value is then 0.
type StatsAmount struct {
	Value  float64 `json:"value"`
	NoData bool    `json:"no_data,omitempty"`
}

// StatsThroughput is the Throughput section. Every series has one point per
// entry of Stats.Days.
type StatsThroughput struct {
	TicketsMerged     []int `json:"tickets_merged"`
	EpicsMerged       []int `json:"epics_merged"`
	DesignsCompleted  []int `json:"designs_completed"`
	LinesAdded        []int `json:"lines_added"`
	LinesRemoved      []int `json:"lines_removed"`
	FollowUpsCreated  []int `json:"follow_ups_created"`
	FollowUpsFinished []int `json:"follow_ups_finished"`
	WaitingForDesign  []int `json:"waiting_for_design"`
}

// StatsHarnessAmount is one harness's share of a group's day, for tooltips.
type StatsHarnessAmount struct {
	Tokens  int64   `json:"tokens"`
	Dollars float64 `json:"dollars"`
}

// StatsCostGroup is one agent group's series, one point per entry of
// Stats.Days. Harness is empty on stamp days: stamps do not record it.
type StatsCostGroup struct {
	Group   string                          `json:"group"`
	Tokens  []StatsAmount                   `json:"tokens"`
	Spend   []StatsAmount                   `json:"spend"`
	Harness []map[string]StatsHarnessAmount `json:"harness"`
}

// StatsCost is the Tokens and spend section: Queue, Design, One-off and Other.
// Sources names each day's source (live, rebuilt or stamp). Partial is set when
// any day is not live.
type StatsCost struct {
	Groups  []StatsCostGroup `json:"groups"`
	Sources []string         `json:"sources"`
	Partial bool             `json:"partial"`
}

// StatsFriction is the Friction section. The per-day maps are keyed by
// reason, event type or ending source.
type StatsFriction struct {
	// PauseReasons lists the top reasons of the range, then "other" when more
	// exist. PausesByReason keeps every reason, so the tooltip can list them and
	// the chart folds the ones outside PauseReasons into "other".
	PauseReasons    []string         `json:"pause_reasons"`
	PausesByReason  []map[string]int `json:"pauses_by_reason"`
	RunEvents       []map[string]int `json:"run_events"`
	PauseEndings    []map[string]int `json:"pause_endings"`
	WebShare        *float64         `json:"web_share,omitempty"`
	MedianResumeSec *int             `json:"median_resume_secs,omitempty"`
}

// Stats is the /v1/stats payload.
type Stats struct {
	Range      int             `json:"range"`
	Project    string          `json:"project,omitempty"`
	Days       []string        `json:"days"`
	Throughput StatsThroughput `json:"throughput"`
	Cost       StatsCost       `json:"cost"`
	Friction   StatsFriction   `json:"friction"`
	Grilling   StatsGrilling   `json:"grilling"`
	LimitBurn  StatsLimitBurns `json:"limit_burn"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	n := StatsDefaultRange
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
	led, err := s.costLedger(newStatsWindow(n, now))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := buildStats(all, n, project, now, led)
	out.Grilling = buildGrilling(recs, s.onMapEpic, newStatsWindow(n, now), project)
	out.LimitBurn = s.buildLimitBurns(n, now)
	writeJSON(w, out)
}
