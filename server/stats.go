package server

import (
	"net/http"
	"slices"
	"strconv"
	"time"

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
	LinesAdded        []int `json:"lines_added"`
	LinesRemoved      []int `json:"lines_removed"`
	FollowUpsCreated  []int `json:"follow_ups_created"`
	FollowUpsFinished []int `json:"follow_ups_finished"`
	WaitingForDesign  []int `json:"waiting_for_design"`
}

// StatsCost is the Tokens and spend section, each split into Design (map
// epics and their tickets) and Implementation (everything else).
type StatsCost struct {
	TokensDesign         []StatsAmount `json:"tokens_design"`
	TokensImplementation []StatsAmount `json:"tokens_implementation"`
	SpendDesign          []StatsAmount `json:"spend_design"`
	SpendImplementation  []StatsAmount `json:"spend_implementation"`
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
	writeJSON(w, buildStats(all, n, project, time.Now().In(time.Local)))
}
