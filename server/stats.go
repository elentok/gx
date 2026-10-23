package server

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/elentok/gx/tickets"
)

const (
	statsDefaultRange = 30
	statsDayLayout    = "2006-01-02"
)

var statsRanges = []int{7, 30, 90}

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
	// PauseReasons lists the top reasons of the range; the rest are "other" in
	// PausesByReason, which carries every reason for the tooltip.
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

func zeroInts(n int) []int { return make([]int, n) }

func emptyAmounts(n int) []StatsAmount { return make([]StatsAmount, n) }

func emptyCounts(n int) []map[string]int {
	out := make([]map[string]int, n)
	for i := range out {
		out[i] = map[string]int{}
	}
	return out
}

// statsWindow maps instants onto the range's local days, ending today.
type statsWindow struct {
	days  []string
	index map[string]int
}

func newStatsWindow(n int, now time.Time) statsWindow {
	w := statsWindow{index: map[string]int{}}
	for i := n - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format(statsDayLayout)
		w.index[day] = len(w.days)
		w.days = append(w.days, day)
	}
	return w
}

// bucket is the day index of t in the server's local time zone, or -1 when t is
// unset or outside the range.
func (w statsWindow) bucket(t time.Time) int {
	if t.IsZero() {
		return -1
	}
	i, ok := w.index[t.In(time.Local).Format(statsDayLayout)]
	if !ok {
		return -1
	}
	return i
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	n := statsDefaultRange
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
	win := newStatsWindow(n, time.Now().In(time.Local))
	out := Stats{
		Range: n, Project: project, Days: win.days,
		Throughput: StatsThroughput{
			TicketsMerged: zeroInts(n), EpicsMerged: zeroInts(n),
			LinesAdded: zeroInts(n), LinesRemoved: zeroInts(n),
			FollowUpsCreated: zeroInts(n), FollowUpsFinished: zeroInts(n),
			WaitingForDesign: zeroInts(n),
		},
		Cost: StatsCost{
			TokensDesign: emptyAmounts(n), TokensImplementation: emptyAmounts(n),
			SpendDesign: emptyAmounts(n), SpendImplementation: emptyAmounts(n),
		},
		Friction: StatsFriction{
			PauseReasons:   []string{},
			PausesByReason: emptyCounts(n), RunEvents: emptyCounts(n), PauseEndings: emptyCounts(n),
		},
	}
	for _, se := range all {
		if project != "" && se.Project != project {
			continue
		}
		countMerged(&out.Throughput, win, se.Epic)
	}
	writeJSON(w, out)
}

// countMerged adds one epic to the tickets-merged and epics-merged series.
// Map epics' tickets are design work, not merged tickets.
func countMerged(t *StatsThroughput, win statsWindow, e tickets.Epic) {
	if i := win.bucket(e.MergedAt); i >= 0 {
		t.EpicsMerged[i]++
	}
	if e.IsMap {
		return
	}
	for _, tk := range e.Tickets {
		if !tk.IsDone() {
			continue
		}
		if i := win.bucket(tk.DoneAt); i >= 0 {
			t.TicketsMerged[i]++
		}
	}
}
