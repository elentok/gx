package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

// limitBurnSeed is the state of a limit-burn shape: the Claude limits file and
// the limit history, whose file shape comes from server.limitHistoryState.
type limitBurnSeed struct {
	report  wire.LimitsReport
	samples map[string][]server.LimitSample
	// ledger is the token ledger and the stamped tickets behind it; nil leaves
	// both empty.
	ledger *ledgerSeed
}

// ledgerSeed is the cost data of a shape. Rebuilt rows are written first, as
// the backfill would; live rows are written with the clock at liveSince, so the
// ledger's first live day is that day. Stamped tickets feed the days before the
// first rebuilt day.
type ledgerSeed struct {
	rebuilt, live []tokenledger.Row
	liveSince     time.Time
	stamped       []ticket
}

const statsHistoryEpic = "stats-history"

// limitBurnShapes are the --shape values of the limit-burn scenario. Every time
// is built from the real clock, and the daily steps sit on local midnights, so
// "today so far" and "yesterday" stay right whatever the hour.
var limitBurnShapes = map[string]func(now time.Time) limitBurnSeed{
	// menu: Claude weekly at 81% (7 so far today, 5 yesterday, forecast over
	// 100%), Codex weekly at 40%, both 5-hour rows.
	"menu": menuSeed,
	// live: the menu limits and an empty ledger, with one held agent per cost
	// group (see limitBurnLive).
	"live": menuSeed,
	// few-days: Claude weekly at 22%, the first sample earlier today, no Codex data.
	"few-days": func(now time.Time) limitBurnSeed {
		reset := localMidnight(now).AddDate(0, 0, 4)
		first := localMidnight(now).Add(now.Sub(localMidnight(now)) / 2)
		return limitBurnSeed{
			report: wire.LimitsReport{SevenDay: limitWindow(now, 22, reset.Sub(now), 0)},
			samples: map[string][]server.LimitSample{server.LimitIDClaudeWeekly: {
				limitSample(server.LimitIDClaudeWeekly, 15, reset, first),
				limitSample(server.LimitIDClaudeWeekly, 22, reset, now),
			}},
		}
	},
	// stale: Claude and Codex weekly with 7 days of history, the reset ahead and
	// the newest reading half an hour old.
	"stale": func(now time.Time) limitBurnSeed {
		m := localMidnight(now)
		claudeReset, codexReset := m.AddDate(0, 0, 2), m.AddDate(0, 0, 3)
		const age = 30 * time.Minute
		return limitBurnSeed{
			report: wire.LimitsReport{SevenDay: limitWindow(now, 52, claudeReset.Sub(now), age)},
			samples: map[string][]server.LimitSample{
				server.LimitIDClaudeWeekly: weeklyHistory(server.LimitIDClaudeWeekly, now, claudeReset, age,
					[]float64{8, 16, 24, 33, 40}, 52, []float64{50, 70}),
				server.LimitIDCodexWeekly: weeklyHistory(server.LimitIDCodexWeekly, now, codexReset, age,
					[]float64{5, 10, 17, 25}, 40, []float64{20, 33}),
			},
		}
	},
	// past-reset: Claude weekly whose reset time is two hours gone, with history from three days ago so the row keeps its burn chip.
	"past-reset": func(now time.Time) limitBurnSeed {
		reset := now.Add(-2 * time.Hour)
		return limitBurnSeed{
			report: wire.LimitsReport{SevenDay: limitWindow(now, 75, -2*time.Hour, 6*time.Hour)},
			samples: map[string][]server.LimitSample{server.LimitIDClaudeWeekly: {
				limitSample(server.LimitIDClaudeWeekly, 30, reset, now.Add(-72*time.Hour)),
				limitSample(server.LimitIDClaudeWeekly, 60, reset, now.Add(-30*time.Hour)),
				limitSample(server.LimitIDClaudeWeekly, 75, reset, now.Add(-6*time.Hour)),
			}},
		}
	},
	// stats: 30 days of cost data and 23 days (22 days ago to today) of history
	// for all four limits, so the head of a 30-day range has no limit data. Each
	// history has reset days. The ledger has stamp-only days (29 to 21 days ago,
	// one with an unstamped ticket, so no data), rebuilt days (20 to 11) and live days (10 to today,
	// 5 days ago quiet), across four groups, both harnesses and two projects.
	"stats": func(now time.Time) limitBurnSeed {
		const historyDays = 22
		weekly := []int{17, 10, 3}
		fiveHour := []int{14, 6, 0}
		s := limitBurnSeed{
			samples: map[string][]server.LimitSample{
				server.LimitIDClaudeWeekly: burnHistory(server.LimitIDClaudeWeekly, now, historyDays, 7*24*time.Hour, weekly, 3),
				server.LimitIDCodexWeekly:  burnHistory(server.LimitIDCodexWeekly, now, historyDays, 7*24*time.Hour, weekly, 5),
				server.LimitIDClaude5h:     burnHistory(server.LimitIDClaude5h, now, historyDays, 5*time.Hour, fiveHour, 2),
				server.LimitIDCodex5h:      burnHistory(server.LimitIDCodex5h, now, historyDays, 5*time.Hour, fiveHour, 4),
			},
			ledger: statsLedger(now),
		}
		last := func(id string) server.LimitSample { v := s.samples[id]; return v[len(v)-1] }
		window := func(id string) *wire.LimitWindow {
			l := last(id)
			return &wire.LimitWindow{UsedPercentage: l.UsedPercentage, ResetsAt: l.ResetsAt, ReceivedAt: l.ReceivedAt}
		}
		s.report = wire.LimitsReport{FiveHour: window(server.LimitIDClaude5h), SevenDay: window(server.LimitIDClaudeWeekly)}
		return s
	},
}

func menuSeed(now time.Time) limitBurnSeed {
	m := localMidnight(now)
	claudeReset := m.AddDate(0, 0, 2)
	codexReset := m.AddDate(0, 0, 3)
	s := limitBurnSeed{
		report: wire.LimitsReport{
			FiveHour: limitWindow(now, 34, 2*time.Hour+10*time.Minute, 0),
			SevenDay: limitWindow(now, 81, claudeReset.Sub(now), 0),
		},
		samples: map[string][]server.LimitSample{},
	}
	s.samples[server.LimitIDClaudeWeekly] = weeklyHistory(server.LimitIDClaudeWeekly, now, claudeReset, 0,
		[]float64{20, 36, 52, 69, 74}, 81, []float64{20, 99})
	s.samples[server.LimitIDCodexWeekly] = weeklyHistory(server.LimitIDCodexWeekly, now, codexReset, 0,
		[]float64{14, 20, 24, 36}, 40, []float64{2, 45})
	s.samples[server.LimitIDClaude5h] = fiveHourHistory(server.LimitIDClaude5h, now, 2*time.Hour+10*time.Minute, 10, 34)
	s.samples[server.LimitIDCodex5h] = fiveHourHistory(server.LimitIDCodex5h, now, 3*time.Hour, 5, 12)
	return s
}

const (
	liveQueueEpic = "limit-burn-epic"
	liveMapEpic   = "design-epic"
	// liveTokenPollInterval is short so a usage line appended by a flow shows within a second or two.
	liveTokenPollInterval = "1s"
)

// limitBurnLive replaces the limit-burn scenario for --shape live: one held
// Claude agent each for Queue (an implement ticket), One-off (a ticket in
// one-offs), Design (a session linked to a map) and Other (a session with no
// ticket). Their transcripts are printed (see liveTranscripts).
func limitBurnLive() scenario {
	return scenario{
		limitBurn: true,
		tokenPoll: true,
		chat:      true,
		hold:      true,
		epics: []epic{
			{project, liveQueueEpic, []ticket{{"01", "queue-held", servertest.TicketOpts{}}}},
			{project, "one-offs", []ticket{{"01", "one-off-held", servertest.TicketOpts{}}}},
			{project, liveMapEpic, nil},
		},
		designSteps: map[string]string{project + ":" + liveMapEpic: string(tickets.StepResolveTickets)},
		queue:       []string{project + ":" + liveQueueEpic, project + ":one-offs/01"},
		// The design session is opened on the map, not linked afterwards: the
		// token poll fixes a session's group when it first sees it.
		designByMap: map[string][]string{project + ":" + liveMapEpic: {agentfake.DesignTurnPlain}},
		openDesigns: []string{project + ":" + liveMapEpic},
		sessions:    []sessionSpec{{project: project, prompt: "hello", read: true}},
	}
}

// liveTranscripts are the Claude transcript paths of the live shape's four
// agents, keyed queue, one_off, design and other. The directories may not exist
// yet.
func (f *fixture) liveTranscripts(ctx context.Context) (map[string]string, error) {
	home := filepath.Join(f.root, "home")
	out := map[string]string{}
	for key, label := range map[string]string{"queue": liveQueueEpic + "-iter-01", "one_off": "one-offs-iter-01"} {
		m, err := f.waitAgentMetaLabel(ctx, label)
		if err != nil {
			return nil, err
		}
		out[key] = transcript.PathIn(home, m.Cwd, m.SessionID)
	}
	raw, err := f.cli("agents", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w\n%s", err, raw)
	}
	var list []wire.SessionInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	for _, si := range list {
		if si.Queue {
			continue
		}
		key := "other"
		if si.Ticket != "" {
			key = "design"
		}
		raw, err := f.cli("agents", "chat", si.ID, "--json")
		if err != nil {
			return nil, fmt.Errorf("chat %s: %w\n%s", si.ID, err, raw)
		}
		var chat wire.SessionChatResponse
		if err := json.Unmarshal(raw, &chat); err != nil || chat.ConversationID == "" {
			return nil, fmt.Errorf("session %s has no conversation id in %s", si.ID, raw)
		}
		out[key] = transcript.PathIn(home, si.Dir, chat.ConversationID)
	}
	if len(out) != 4 {
		return nil, fmt.Errorf("want 4 live transcripts, got %v", out)
	}
	return out, nil
}

// Harness names in ledger rows.
const (
	ledgerClaude = "claude"
	ledgerCodex  = "codex"
)

// burnHistory is one sample a day from daysAgo days ago to today, each at local
// noon (today's just before now). The limit rises by a few points a day, with
// quiet days, and starts a new window on each reset day: the reset time moves
// forward and the percentage starts again. period is the window length; the last
// window ends after now. step shifts the daily rise per limit.
func burnHistory(id string, now time.Time, daysAgo int, period time.Duration, resetDays []int, step int) []server.LimitSample {
	var out []server.LimitSample
	used := float64(step * 3)
	var resetsAt time.Time
	for ago := daysAgo; ago >= 0; ago-- {
		at := localMidnight(now).AddDate(0, 0, -ago).Add(12 * time.Hour)
		if ago == 0 && at.After(now) {
			at = now.Add(-time.Minute)
		}
		rise := float64((ago*7+step)%5 + 1)
		if ago%6 == 4 {
			rise = 0
		}
		switch {
		case slices.Contains(resetDays, ago):
			used = rise + 1
			resetsAt = at.Add(period)
			if ago == 0 {
				resetsAt = now.Add(period / 3)
			}
		case resetsAt.IsZero():
			resetsAt = at.Add(period / 2)
			used += rise
		default:
			used += rise
		}
		out = append(out, limitSample(id, used, resetsAt, at))
	}
	return out
}

// statsLedger builds the cost data of the stats shape (see its comment).
func statsLedger(now time.Time) *ledgerSeed {
	day := func(ago int) string { return localMidnight(now).AddDate(0, 0, -ago).Format(tokenledger.DayLayout) }
	other := "docs-site"
	combos := []tokenledger.Key{
		{Project: project, Group: tokenledger.GroupQueue, Harness: ledgerClaude},
		{Project: project, Group: tokenledger.GroupDesign, Harness: ledgerClaude},
		{Project: other, Group: tokenledger.GroupQueue, Harness: ledgerCodex},
		{Project: project, Group: tokenledger.GroupOneOff, Harness: ledgerCodex},
		{Project: other, Group: tokenledger.GroupOther, Harness: ledgerClaude},
	}
	rows := func(from, to int, quiet int) []tokenledger.Row {
		var out []tokenledger.Row
		for ago := from; ago >= to; ago-- {
			if ago == quiet {
				continue
			}
			for i, k := range combos {
				if (ago+i)%2 != 0 {
					continue
				}
				k.Day = day(ago)
				tokens := int64(40_000 + ago*3_100 + i*17_000)
				dollars := float64(tokens) / 80_000
				if k.Harness == ledgerCodex {
					dollars = 0 // Codex reports tokens, not a price.
				}
				out = append(out, tokenledger.Row{Key: k, Tokens: tokens, Dollars: dollars})
			}
		}
		return out
	}
	l := &ledgerSeed{
		rebuilt:   rows(20, 11, -1),
		live:      rows(10, 0, 5),
		liveSince: localMidnight(now).AddDate(0, 0, -10).Add(time.Hour),
	}
	for ago := 29; ago >= 21; ago-- {
		tokens := int64(60_000 + ago*2_300)
		if ago == 25 {
			// Finished without a stamp: a no-data day, not a zero one.
			tokens = 0
		}
		l.stamped = append(l.stamped, ticket{fmt.Sprintf("%02d", 30-ago), fmt.Sprintf("stamped-%d-days-ago", ago), servertest.TicketOpts{
			Status:      "done",
			DoneAt:      localMidnight(now).AddDate(0, 0, -ago).Add(14 * time.Hour).Format(time.RFC3339),
			TotalTokens: tokens,
			ActualCost:  float64(tokens) / 80_000,
		}})
	}
	return l
}

func limitBurnShapeNames() []string {
	names := make([]string, 0, len(limitBurnShapes))
	for n := range limitBurnShapes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func localMidnight(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}

func limitSample(id string, pct float64, resetsAt, receivedAt time.Time) server.LimitSample {
	return server.LimitSample{LimitID: id, UsedPercentage: pct, ResetsAt: resetsAt.Unix(), ReceivedAt: receivedAt.Unix()}
}

// weeklyHistory is the history of one weekly window that resets at reset. The
// window opened 7 days before, so dayEnds (one per local day since then,
// earliest first) fill it up to yesterday and today ends at current. Each day
// has a sample just after midnight carrying the day before and one at the end
// of the day. prev is the end-of-day percentages of the window before it, which
// give the 7th day of history. The newest sample was received newestAgo ago.
func weeklyHistory(id string, now, reset time.Time, newestAgo time.Duration, dayEnds []float64, current float64, prev []float64) []server.LimitSample {
	start := reset.AddDate(0, 0, -7)
	var out []server.LimitSample
	for i, pct := range prev {
		day := start.AddDate(0, 0, i-len(prev))
		out = append(out, limitSample(id, pct, start, day.Add(30*time.Minute)))
	}
	carry := 0.0
	for i, pct := range dayEnds {
		day := start.AddDate(0, 0, i)
		out = append(out,
			limitSample(id, carry, reset, day.Add(30*time.Minute)),
			limitSample(id, pct, reset, day.Add(23*time.Hour+30*time.Minute)))
		carry = pct
	}
	newest := now.Add(-newestAgo)
	today := localMidnight(now).Add(30 * time.Minute)
	if today.After(newest.Add(-time.Minute)) {
		today = newest.Add(-time.Minute)
	}
	return append(out, limitSample(id, carry, reset, today), limitSample(id, current, reset, newest))
}

// fiveHourHistory is two samples of a 5-hour window that resets in resetsIn.
func fiveHourHistory(id string, now time.Time, resetsIn time.Duration, before, current float64) []server.LimitSample {
	reset := now.Add(resetsIn)
	return []server.LimitSample{
		limitSample(id, before, reset, now.Add(-time.Hour)),
		limitSample(id, current, reset, now),
	}
}

// seedLimitBurn writes the Claude limits file and the limit history the server
// loads at start. The history file is written as server.limitHistoryState:
// every sample per limit, and the newest one as latest.
func (f *fixture) seedLimitBurn(shape string) error {
	seed := limitBurnShapes[shape](time.Now())
	latest := map[string]server.LimitSample{}
	for id, s := range seed.samples {
		latest[id] = s[len(s)-1]
	}
	if err := os.MkdirAll(f.stateD, 0o700); err != nil {
		return err
	}
	for name, v := range map[string]any{
		"claude-limits.json": seed.report,
		"limit-history.json": map[string]any{"samples": seed.samples, "latest": latest},
	} {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(f.stateD, name), data, 0o600); err != nil {
			return err
		}
	}
	if seed.ledger != nil {
		return f.seedLedger(seed.ledger)
	}
	return nil
}

// seedLedger writes the token ledger through the tokenledger package, rebuilt
// rows first (they stop at the first live day), and the stamped tickets.
func (f *fixture) seedLedger(l *ledgerSeed) error {
	rebuilt, err := tokenledger.Open(f.stateD)
	if err != nil {
		return err
	}
	if _, _, err := rebuilt.ApplyRebuilt(l.rebuilt); err != nil {
		return err
	}
	live, err := tokenledger.OpenWithClock(f.stateD, func() time.Time { return l.liveSince })
	if err != nil {
		return err
	}
	for _, r := range l.live {
		if err := live.Add(tokenledger.Delta{Key: r.Key, Tokens: r.Tokens, Dollars: r.Dollars}); err != nil {
			return err
		}
	}
	for _, tk := range l.stamped {
		if err := servertest.WriteTicketFile(f.store, project, statsHistoryEpic, tk.id, tk.slug, tk.opts); err != nil {
			return err
		}
	}
	return nil
}
