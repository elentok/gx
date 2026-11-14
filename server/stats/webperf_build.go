package stats

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/webperf"
)

const topActions = 3

// buildVals are one build's values over the selected range. Transfer sizes of
// zero mean cached or unknown and are never added.
type buildVals struct {
	id, webVersion               string
	lcp, inp, ttfb, api, js, css []float64
}

func addVal(dst *[]float64, v float64, nonZero bool) {
	if math.IsNaN(v) || (nonZero && v == 0) {
		return
	}
	*dst = append(*dst, v)
}

func (b *buildVals) add(d *webperf.Day, i int, metric, tab string) {
	switch metric {
	case "lcp":
		if tab == tabHard {
			addVal(&b.lcp, d.Value[i], false)
		}
	case "inp":
		addVal(&b.inp, d.Value[i], false)
	case "app_ready", "route_ready":
		addVal(&b.api, d.APILatency[i], false)
	}
	// Poor LCP samples also carry ttfb; counting them would double a hard load.
	if metric == "app_ready" {
		addVal(&b.ttfb, d.TTFB[i], false)
	}
	if tab == tabHard {
		addVal(&b.js, d.JSBytes[i], true)
		addVal(&b.css, d.CSSBytes[i], true)
	}
}

// pctChange is nil unless both sides have enough samples.
func pctChange(cur, prev []float64) *float64 {
	if len(cur) < webperf.MinSamples || len(prev) < webperf.MinSamples {
		return nil
	}
	c, p := p75(cur), p75(prev)
	if c == nil || p == nil || *p == 0 {
		return nil
	}
	v := (*c - *p) / *p * 100
	return &v
}

func buildSide(b *buildVals) wire.StatsWebPerfBuild {
	return wire.StatsWebPerfBuild{BuildID: b.id, WebVersion: b.webVersion, LCPP75: p75(b.lcp), INPP75: p75(b.inp), JSP75: p75(b.js)}
}

// allDays are the stored days up to the last day of the window.
func allDays(oldest string, last time.Time) []string {
	first, err := time.ParseInLocation(webperf.DayLayout, oldest, time.Local)
	if err != nil {
		return nil
	}
	var out []string
	end := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.Local)
	for d := first; !d.After(end); d = time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, time.Local) {
		out = append(out, d.Format(webperf.DayLayout))
	}
	return out
}

// topCount is the keys by count, ties by name, at most limit of them.
func topCount(counts map[string]int, limit int) []wire.StatsWebPerfAction {
	out := make([]wire.StatsWebPerfAction, 0, len(counts))
	for k, n := range counts {
		out = append(out, wire.StatsWebPerfAction{Action: k, Count: n})
	}
	slices.SortFunc(out, func(a, b wire.StatsWebPerfAction) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Action, b.Action))
	})
	return out[:min(len(out), limit)]
}

// webPerfBuilds are the build strip, Why slow? rows and poor-INP block.
type webPerfBuilds struct {
	strip   *wire.StatsWebPerfBuildStrip
	whySlow []wire.StatsWebPerfWhySlow
	poorINP wire.StatsWebPerfPoorINP
}

// buildWebPerfBuilds reads every loaded day for build order; values count only
// inside the window.
func buildWebPerfBuilds(loaded []*webperf.Day, win limits.StatsWindow, includeDev bool) webPerfBuilds {
	empty := wire.NewStatsWebPerformance()
	out := webPerfBuilds{whySlow: empty.WhySlow, poorINP: empty.PoorINP}
	rangeStart := win.Labels()[0]

	// builds is in first-seen order: the last is current, the one before previous.
	var builds []*buildVals
	byID := map[string]*buildVals{}
	var poor struct {
		in, proc, pres  []float64
		actions, chunks map[string]int
	}
	poor.actions, poor.chunks = map[string]int{}, map[string]int{}
	for _, d := range loaded {
		inRange := d.Day >= rangeStart
		perfRows(d, includeDev, func(i int, metric, tab string) {
			if id := d.BuildID.At(i); id != "" {
				b := byID[id]
				if b == nil {
					b = &buildVals{id: id, webVersion: d.WebVersion.At(i)}
					byID[id] = b
					builds = append(builds, b)
				}
				if inRange {
					b.add(d, i, metric, tab)
				}
			}
			if inRange && metric == "inp" && d.Value[i] > webperf.PoorINP {
				out.poorINP.Count++
				addVal(&poor.in, d.InputDelay[i], false)
				addVal(&poor.proc, d.Processing[i], false)
				addVal(&poor.pres, d.PresentationDelay[i], false)
				if a := d.Action.At(i); a != "" {
					poor.actions[a]++
				}
				if c := d.WorstChunk.At(i); c != "" {
					poor.chunks[c]++
				}
			}
		})
	}
	out.poorINP.InputDelayP75, out.poorINP.ProcessingP75, out.poorINP.PresentationDelayP75 = p75(poor.in), p75(poor.proc), p75(poor.pres)
	out.poorINP.TopActions = topCount(poor.actions, topActions)
	if c := topCount(poor.chunks, 1); len(c) > 0 {
		out.poorINP.WorstChunk = c[0].Action
	}

	var cur, prev *buildVals
	if len(builds) > 0 {
		cur = builds[len(builds)-1]
	}
	if len(builds) > 1 {
		prev = builds[len(builds)-2]
	}
	if cur == nil {
		return out
	}
	strip := &wire.StatsWebPerfBuildStrip{Current: buildSide(cur)}
	var prevVals buildVals
	if prev != nil {
		p := buildSide(prev)
		strip.Previous = &p
		prevVals = *prev
		strip.LCPChangePct = pctChange(cur.lcp, prev.lcp)
		strip.INPChangePct = pctChange(cur.inp, prev.inp)
		strip.JSChangePct = pctChange(cur.js, prev.js)
	}
	out.strip = strip
	for _, r := range []struct {
		metric    string
		cur, prev []float64
	}{
		{"ttfb", cur.ttfb, prevVals.ttfb},
		{"api_latency", cur.api, prevVals.api},
		{"js_bytes", cur.js, prevVals.js},
		{"css_bytes", cur.css, prevVals.css},
	} {
		out.whySlow = append(out.whySlow, wire.StatsWebPerfWhySlow{
			Metric: r.metric, Count: len(r.cur), P75: p75(r.cur), PreviousP75: p75(r.prev),
			ChangePct: pctChange(r.cur, r.prev),
		})
	}
	return out
}
