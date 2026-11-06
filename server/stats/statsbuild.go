package stats

import (
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmigrate"
)

const (
	followUpsEpic   = "follow-ups"
	statsTopReasons = 5
	statsOtherLabel = "other"
)

func emptyCounts(n int) []map[string]int {
	out := make([]map[string]int, n)
	for i := range out {
		out[i] = map[string]int{}
	}
	return out
}

// newStats is an all-zero payload with one point per day in every series.
func newStats(n int) wire.Stats {
	ints := func() []int { return make([]int, n) }
	return wire.Stats{
		Range: n,
		Throughput: wire.StatsThroughput{
			TicketsMerged: ints(), EpicsMerged: ints(), DesignsCompleted: ints(),
			LinesAdded: ints(), LinesRemoved: ints(),
			FollowUpsCreated: ints(), FollowUpsFinished: ints(),
			WaitingForDesign: ints(),
		},
		Friction: wire.StatsFriction{
			PauseReasons:   []string{},
			PausesByReason: emptyCounts(n), RunEvents: emptyCounts(n), PauseEndings: emptyCounts(n),
		},
		Launch: wire.StatsLaunch{
			Days: make([]wire.StatsAmount, n), StepMedianMs: map[string]int64{},
		},
	}
}

// statsInput is everything one Stats payload is built from.
type statsInput struct {
	Epics    []ticketmigrate.StoreEpic
	Days     int
	Project  string
	Now      time.Time
	Ledger   costLedger
	Records  []conversations.Record
	IsDesign func(ticket string) bool
	Burn     wire.StatsLimitBurns
}

func buildStats(in statsInput) wire.Stats {
	n, project, led := in.Days, in.Project, in.Ledger
	win := limits.NewStatsWindow(n, in.Now)
	out := newStats(n)
	out.Project, out.Days = project, win.Labels()
	b := statsBuilder{
		win: win, tp: &out.Throughput,
		design: make(costSeries, n), impl: make(costSeries, n),
		friction: frictionTally{
			pauses: out.Friction.PausesByReason, events: out.Friction.RunEvents, endings: out.Friction.PauseEndings,
			reasonTotal: map[string]int{},
		},
		launch: newLaunchTally(n),
	}
	for _, se := range in.Epics {
		if project == "" || se.Project == project {
			b.addEpic(se.Epic)
		}
	}
	b.friction.fill(&out.Friction)
	b.launch.fill(&out.Launch)
	out.Cost = statsCostOf(pickCostSource(costSourceInput{
		costLedger: led, Days: out.Days,
		Design: b.design, Impl: b.impl, Project: project,
	}))
	out.Grilling = buildGrilling(in.Records, in.IsDesign, win, project)
	out.LimitBurn = in.Burn
	return out
}

// statsBuilder sums every series while the epics are walked once.
type statsBuilder struct {
	win      limits.StatsWindow
	tp       *wire.StatsThroughput
	design   costSeries
	impl     costSeries
	friction frictionTally
	launch   launchTally
}

func (b *statsBuilder) addEpic(e tickets.Epic) {
	cost := b.impl
	if e.IsMap {
		cost = b.design
		cost.addMapRemainder(b.win, e)
		b.addWaiting(e)
		// A map's merged_at is copied from its impl epic, so a map counts once,
		// as a completed design, and not again as a merged epic.
		if i := b.win.Bucket(e.CompletedAt); i >= 0 {
			b.tp.DesignsCompleted[i]++
		}
	} else if i := b.win.Bucket(e.MergedAt); i >= 0 {
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
		if i := b.win.Bucket(tk.DoneAt); i >= 0 {
			cost[i].add(tk.TotalTokens, tk.ActualCost)
			if !e.IsMap {
				b.tp.TicketsMerged[i]++
			}
		}
	}
	finished := b.win.Bucket(tk.FinishedAt())
	b.friction.addTicket(finished, tk)
	b.launch.addTicket(b.win, tk)
	if originalEpic(tk, e.Name) == followUpsEpic {
		// A move finishes nothing: it sets neither done_at nor cancelled_at.
		if i := b.win.Bucket(tk.CreatedAt); i >= 0 {
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
	for i, start := range b.win.Days {
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
func (c costSeries) addMapRemainder(win limits.StatsWindow, e tickets.Epic) {
	i := win.Bucket(e.CompletedAt)
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
func (c costSeries) amounts() (tokens, spend []wire.StatsAmount) {
	tokens, spend = make([]wire.StatsAmount, len(c)), make([]wire.StatsAmount, len(c))
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
func (f *frictionTally) fill(out *wire.StatsFriction) {
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
	out.WebShare = WebShare(total)
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

// LaunchTargetMs is the launch-time target the under-target share counts
// against.
const LaunchTargetMs = int64(events.SlowLaunch / time.Millisecond)

// launchTally is the Launch section while it is being summed.
type launchTally struct {
	totals []int64
	days   [][]int64
	steps  map[string][]int64
}

func newLaunchTally(n int) launchTally {
	return launchTally{days: make([][]int64, n), steps: map[string][]int64{}}
}

// addTicket counts each launch of a finished ticket on its own claim day.
func (l *launchTally) addTicket(win limits.StatsWindow, tk tickets.Ticket) {
	if tk.FinishedAt().IsZero() {
		return
	}
	for _, s := range tk.Launch {
		i := win.Bucket(s.ClaimedAt)
		if i < 0 {
			continue
		}
		l.totals = append(l.totals, s.TotalMs)
		l.days[i] = append(l.days[i], s.TotalMs)
		l.steps["worktree"] = append(l.steps["worktree"], s.WorktreeMs)
		l.steps["deps"] = append(l.steps["deps"], s.DepsMs)
		l.steps["agent"] = append(l.steps["agent"], s.AgentMs)
		l.steps["prompt"] = append(l.steps["prompt"], s.PromptMs)
	}
}

func (l *launchTally) fill(out *wire.StatsLaunch) {
	out.Count, out.TargetMs = len(l.totals), LaunchTargetMs
	for i, d := range l.days {
		if len(d) == 0 {
			out.Days[i].NoData = true
			continue
		}
		out.Days[i].Value = float64(medianMs(d))
	}
	if len(l.totals) == 0 {
		return
	}
	m := medianMs(l.totals)
	out.MedianMs = &m
	under := 0
	for _, t := range l.totals {
		if t <= LaunchTargetMs {
			under++
		}
	}
	share := float64(under) / float64(len(l.totals))
	out.ShareUnderTgt = &share
	for k, v := range l.steps {
		out.StepMedianMs[k] = medianMs(v)
	}
}

func medianMs(xs []int64) int64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 0 {
		return (s[mid-1] + s[mid]) / 2
	}
	return s[mid]
}

// WebShare is web / (web + TUI + CLI) over pause endings by source; gx- and
// unknown-ended pauses are not person endings, so with none it returns nil.
func WebShare(endings map[string]int) *float64 {
	web := endings[tickets.SourceWeb]
	person := web + endings[tickets.SourceTUI] + endings[tickets.SourceCLI]
	if person == 0 {
		return nil
	}
	share := float64(web) / float64(person)
	return &share
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
