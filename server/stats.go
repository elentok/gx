package server

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	followUpsEpic     = "follow-ups"
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
	writeJSON(w, buildStats(all, n, project, time.Now().In(time.Local)))
}

func buildStats(all []tickets.StoreEpic, n int, project string, now time.Time) Stats {
	win := newStatsWindow(n, now)
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
	design, impl := make(costSeries, n), make(costSeries, n)
	friction := frictionTally{
		pauses: emptyCounts(n), events: emptyCounts(n), endings: emptyCounts(n),
		reasonTotal: map[string]int{},
	}
	for _, se := range all {
		if project != "" && se.Project != project {
			continue
		}
		countCost(design, impl, win, se.Epic)
		countMerged(&out.Throughput, win, se.Epic)
		countLines(&out.Throughput, win, se.Epic)
		countFollowUps(&out.Throughput, win, se.Epic)
		countWaiting(&out.Throughput, win, se.Epic)
		for _, tk := range se.Epic.Tickets {
			friction.addTicket(win, tk)
		}
	}
	friction.fill(&out.Friction)
	out.Cost.TokensDesign, out.Cost.SpendDesign = design.amounts()
	out.Cost.TokensImplementation, out.Cost.SpendImplementation = impl.amounts()
	return out
}

// costDay is one day of one cost series while it is being summed.
type costDay struct {
	finished bool // an item finished on this day, stamped or not
	stamped  bool // at least one finished item carries the stamp
	tokens   int64
	spend    float64
}

func (d *costDay) add(tokens *int64, spend float64) {
	d.finished = true
	if tokens == nil {
		return
	}
	d.stamped = true
	d.tokens += *tokens
	d.spend += spend
}

// costSeries is the Design or Implementation side of the cost charts.
type costSeries []costDay

// addTicket counts a done ticket on its done_at. Cancelled tickets are not
// finished items.
func (c costSeries) addTicket(win statsWindow, tk tickets.Ticket) {
	if !tk.IsDone() {
		return
	}
	if i := win.bucket(tk.DoneAt); i >= 0 {
		c[i].add(tk.TotalTokens, tk.ActualCost)
	}
}

// addMapRemainder counts a completed map epic on its stamp day: the epic's
// stamp minus its tickets' stamps, which the tickets already count. An
// unstamped map still finishes an item that day.
func (c costSeries) addMapRemainder(win statsWindow, e tickets.Epic) {
	i := win.bucket(e.CompletedAt)
	if i < 0 {
		return
	}
	if e.TotalTokens == nil {
		c[i].add(nil, 0)
		return
	}
	tokens, spend := *e.TotalTokens, e.ActualCost
	for _, tk := range e.Tickets {
		if tk.IsDone() && tk.TotalTokens != nil {
			tokens -= *tk.TotalTokens
			spend -= tk.ActualCost
		}
	}
	tokens = max(tokens, 0)
	c[i].add(&tokens, max(spend, 0))
}

// amounts returns the tokens and spend series. Stamps are all or nothing, so
// both share the same "no data" days.
func (c costSeries) amounts() (tokens, spend []StatsAmount) {
	tokens, spend = emptyAmounts(len(c)), emptyAmounts(len(c))
	for i, d := range c {
		if d.finished && !d.stamped {
			tokens[i].NoData, spend[i].NoData = true, true
			continue
		}
		tokens[i].Value, spend[i].Value = float64(d.tokens), d.spend
	}
	return tokens, spend
}

// countCost sums the ticket stamps of every epic into design and
// implementation. Epic stamps count only as a map's remainder.
func countCost(design, impl costSeries, win statsWindow, e tickets.Epic) {
	target := impl
	if e.IsMap {
		target = design
		design.addMapRemainder(win, e)
	}
	for _, tk := range e.Tickets {
		target.addTicket(win, tk)
	}
}

const (
	statsTopReasons = 5
	statsOtherLabel = "other"
)

// frictionTally is the Friction section while it is being summed.
type frictionTally struct {
	pauses      []map[string]int
	events      []map[string]int
	endings     []map[string]int
	personSecs  []int
	reasonTotal map[string]int
}

func (f *frictionTally) addTicket(win statsWindow, tk tickets.Ticket) {
	if tk.Friction == nil {
		return
	}
	finished := tk.DoneAt
	if finished.IsZero() {
		finished = tk.CancelledAt
	}
	i := win.bucket(finished)
	if i < 0 {
		return
	}
	for k, n := range tk.Friction.PausesByReason {
		f.pauses[i][k] += n
		f.reasonTotal[k] += n
	}
	for k, n := range tk.Friction.RunEvents {
		f.events[i][k] += n
	}
	for k, n := range tk.Friction.PauseEndings {
		f.endings[i][k] += n
	}
	f.personSecs = append(f.personSecs, tk.Friction.PersonPauseSecs...)
}

// fill writes the tally into the payload: the top reasons of the range plus
// "other", and the headline values over person-ended pauses only.
func (f *frictionTally) fill(out *StatsFriction) {
	out.PausesByReason, out.RunEvents, out.PauseEndings = f.pauses, f.events, f.endings
	reasons := make([]string, 0, len(f.reasonTotal))
	for k := range f.reasonTotal {
		reasons = append(reasons, k)
	}
	slices.SortFunc(reasons, func(a, b string) int {
		if c := f.reasonTotal[b] - f.reasonTotal[a]; c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	if len(reasons) > statsTopReasons {
		reasons = append(reasons[:statsTopReasons], statsOtherLabel)
	}
	out.PauseReasons = append(out.PauseReasons, reasons...)

	var web, person int
	for _, day := range f.endings {
		web += day[tickets.SourceWeb]
		person += day[tickets.SourceWeb] + day[tickets.SourceTUI] + day[tickets.SourceCLI]
	}
	if person > 0 {
		share := float64(web) / float64(person)
		out.WebShare = &share
	}
	if len(f.personSecs) > 0 {
		secs := slices.Clone(f.personSecs)
		slices.Sort(secs)
		mid := len(secs) / 2
		median := secs[mid]
		if len(secs)%2 == 0 {
			median = (secs[mid-1] + secs[mid]) / 2
		}
		out.MedianResumeSec = &median
	}
}

// countLines adds a merged epic's lines to its merge day.
func countLines(t *StatsThroughput, win statsWindow, e tickets.Epic) {
	i := win.bucket(e.MergedAt)
	if i < 0 {
		return
	}
	if e.LinesAdded != nil {
		t.LinesAdded[i] += *e.LinesAdded
	}
	if e.LinesRemoved != nil {
		t.LinesRemoved[i] += *e.LinesRemoved
	}
}

// originalEpic is the epic a ticket was first created in, wherever it lives
// now: its original address, else the move's Resolution line, else its epic.
func originalEpic(tk tickets.Ticket, current string) string {
	raw := tk.OriginalAddress
	if raw == "" {
		for line := range strings.SplitSeq(schema.Section(tk.Body, "Resolution"), "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- original: "); ok {
				raw = rest
			}
		}
	}
	if raw == "" {
		return current
	}
	addr, err := tickets.ParseAddress(raw, tickets.AddressContext{})
	if err != nil {
		return current
	}
	return addr.Epic
}

// countFollowUps adds the follow-ups of one epic: created on created_at,
// finished on done_at or cancelled_at. A move finishes nothing.
func countFollowUps(t *StatsThroughput, win statsWindow, e tickets.Epic) {
	for _, tk := range e.Tickets {
		if originalEpic(tk, e.Name) != followUpsEpic {
			continue
		}
		if i := win.bucket(tk.CreatedAt); i >= 0 {
			t.FollowUpsCreated[i]++
		}
		finished := tk.DoneAt
		if finished.IsZero() {
			finished = tk.CancelledAt
		}
		if i := win.bucket(finished); i >= 0 {
			t.FollowUpsFinished[i]++
		}
	}
}

// countWaiting adds a map epic to every day it existed and was not yet
// completed at the end of. A draft map is not waiting for design.
func countWaiting(t *StatsThroughput, win statsWindow, e tickets.Epic) {
	if !e.IsMap || e.CreatedAt.IsZero() || e.Status == string(schema.StatusDraft) {
		return
	}
	for i, day := range win.days {
		start, err := time.ParseInLocation(statsDayLayout, day, time.Local)
		if err != nil {
			continue
		}
		end := start.AddDate(0, 0, 1)
		if e.CreatedAt.Before(end) && (e.CompletedAt.IsZero() || !e.CompletedAt.Before(end)) {
			t.WaitingForDesign[i]++
		}
	}
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
