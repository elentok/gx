package server

import (
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	followUpsEpic   = "follow-ups"
	statsDayLayout  = "2006-01-02"
	statsTopReasons = 5
	statsOtherLabel = "other"
)

// statsWindow maps instants onto the range's local days, ending today.
type statsWindow struct {
	days  []time.Time // local midnight of each day, oldest first
	index map[string]int
}

func newStatsWindow(n int, now time.Time) statsWindow {
	w := statsWindow{index: map[string]int{}}
	for i := n - 1; i >= 0; i-- {
		day := time.Date(now.Year(), now.Month(), now.Day()-i, 0, 0, 0, 0, time.Local)
		w.index[day.Format(statsDayLayout)] = len(w.days)
		w.days = append(w.days, day)
	}
	return w
}

func (w statsWindow) labels() []string {
	out := make([]string, len(w.days))
	for i, d := range w.days {
		out[i] = d.Format(statsDayLayout)
	}
	return out
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

func emptyCounts(n int) []map[string]int {
	out := make([]map[string]int, n)
	for i := range out {
		out[i] = map[string]int{}
	}
	return out
}

// newStats is an all-zero payload with one point per day in every series.
func newStats(n int) Stats {
	ints := func() []int { return make([]int, n) }
	amounts := func() []StatsAmount { return make([]StatsAmount, n) }
	return Stats{
		Range: n,
		Throughput: StatsThroughput{
			TicketsMerged: ints(), EpicsMerged: ints(),
			LinesAdded: ints(), LinesRemoved: ints(),
			FollowUpsCreated: ints(), FollowUpsFinished: ints(),
			WaitingForDesign: ints(),
		},
		Cost: StatsCost{
			TokensDesign: amounts(), TokensImplementation: amounts(),
			SpendDesign: amounts(), SpendImplementation: amounts(),
		},
		Friction: StatsFriction{
			PauseReasons:   []string{},
			PausesByReason: emptyCounts(n), RunEvents: emptyCounts(n), PauseEndings: emptyCounts(n),
		},
		Grilling: newStatsGrilling(n),
	}
}

func buildStats(all []tickets.StoreEpic, n int, project string, now time.Time) Stats {
	win := newStatsWindow(n, now)
	out := newStats(n)
	out.Project, out.Days = project, win.labels()
	b := statsBuilder{
		win: win, tp: &out.Throughput,
		design: make(costSeries, n), impl: make(costSeries, n),
		friction: frictionTally{
			pauses: out.Friction.PausesByReason, events: out.Friction.RunEvents, endings: out.Friction.PauseEndings,
			reasonTotal: map[string]int{},
		},
	}
	for _, se := range all {
		if project == "" || se.Project == project {
			b.addEpic(se.Epic)
		}
	}
	b.friction.fill(&out.Friction)
	out.Cost.TokensDesign, out.Cost.SpendDesign = b.design.amounts()
	out.Cost.TokensImplementation, out.Cost.SpendImplementation = b.impl.amounts()
	return out
}

// statsBuilder sums every series while the epics are walked once.
type statsBuilder struct {
	win      statsWindow
	tp       *StatsThroughput
	design   costSeries
	impl     costSeries
	friction frictionTally
}

func (b *statsBuilder) addEpic(e tickets.Epic) {
	cost := b.impl
	if e.IsMap {
		cost = b.design
		cost.addMapRemainder(b.win, e)
		b.addWaiting(e)
	}
	if i := b.win.bucket(e.MergedAt); i >= 0 {
		b.tp.EpicsMerged[i]++
		if e.LinesAdded != nil {
			b.tp.LinesAdded[i] += *e.LinesAdded
		}
		if e.LinesRemoved != nil {
			b.tp.LinesRemoved[i] += *e.LinesRemoved
		}
	}
	for _, tk := range e.Tickets {
		b.addTicket(e, tk, cost)
	}
}

// addTicket counts one ticket in the cost, merged, friction and follow-up
// series. Only done tickets are finished items for cost and merged counts;
// cancelled ones still count for friction and finished follow-ups. Map epics'
// tickets are design work, not merged tickets.
func (b *statsBuilder) addTicket(e tickets.Epic, tk tickets.Ticket, cost costSeries) {
	if tk.IsDone() {
		if i := b.win.bucket(tk.DoneAt); i >= 0 {
			cost[i].add(tk.TotalTokens, tk.ActualCost)
			if !e.IsMap {
				b.tp.TicketsMerged[i]++
			}
		}
	}
	finished := b.win.bucket(tk.FinishedAt())
	b.friction.addTicket(finished, tk)
	if originalEpic(tk, e.Name) == followUpsEpic {
		// A move finishes nothing: it sets neither done_at nor cancelled_at.
		if i := b.win.bucket(tk.CreatedAt); i >= 0 {
			b.tp.FollowUpsCreated[i]++
		}
		if finished >= 0 {
			b.tp.FollowUpsFinished[finished]++
		}
	}
}

// addWaiting adds a map epic to every day it existed and was not yet
// completed at the end of. A draft map is not waiting for design.
func (b *statsBuilder) addWaiting(e tickets.Epic) {
	if e.CreatedAt.IsZero() || e.Status == string(schema.StatusDraft) {
		return
	}
	for i, start := range b.win.days {
		end := start.AddDate(0, 0, 1)
		if e.CreatedAt.Before(end) && (e.CompletedAt.IsZero() || !e.CompletedAt.Before(end)) {
			b.tp.WaitingForDesign[i]++
		}
	}
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
	tokens, spend = make([]StatsAmount, len(c)), make([]StatsAmount, len(c))
	for i, d := range c {
		if d.finished && !d.stamped {
			tokens[i].NoData, spend[i].NoData = true, true
			continue
		}
		tokens[i].Value, spend[i].Value = float64(d.tokens), d.spend
	}
	return tokens, spend
}

// frictionTally is the Friction section while it is being summed.
type frictionTally struct {
	pauses      []map[string]int
	events      []map[string]int
	endings     []map[string]int
	personSecs  []int
	reasonTotal map[string]int
}

// addTicket counts a ticket's friction on day i, its finish day (-1 for none).
func (f *frictionTally) addTicket(i int, tk tickets.Ticket) {
	if tk.Friction == nil || i < 0 {
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

	total := map[string]int{}
	for _, day := range f.endings {
		for src, n := range day {
			total[src] += n
		}
	}
	out.WebShare = webShare(total)
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
