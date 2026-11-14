package stats

import (
	"cmp"
	"errors"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/webperf"
)

// WebPerf is the sample store as the web performance section reads it.
type WebPerf interface {
	Day(day string) (*webperf.Day, error)
	OldestDay() (string, bool)
}

const (
	tabHard  = "hard"
	tabRoute = "route"
)

var (
	hardMetrics  = []string{"lcp", "inp", "cls", "fcp", "app_ready"}
	routeMetrics = []string{"route_ready", "inp", "cls"}
	trendMetrics = []string{"lcp", "inp", "cls", "fcp", "app_ready", "route_ready"}
)

// cutoffs are the good and poor limits of the metrics that have a standard.
var cutoffs = map[string][2]float64{
	"lcp": {2500, 4000},
	"inp": {200, 500},
	"cls": {0.1, 0.25},
	"fcp": {1800, 3000},
}

const slowestRoutes = 5

type perfKey struct{ tab, metric string }

// tabOf is the tab a sample counts under. Readiness metrics have one tab; a
// standard metric with no tab came from a browser without soft-navigation
// reporting, so it is a hard load.
func tabOf(metric, tab string) string {
	switch {
	case metric == "route_ready":
		return tabRoute
	case metric == "app_ready":
		return tabHard
	case tab == tabRoute:
		return tabRoute
	}
	return tabHard
}

// perfRows calls fn for each kept sample of a day.
func perfRows(d *webperf.Day, includeDev bool, fn func(i int, metric, tab string)) {
	for i := range d.N {
		if !includeDev && d.Env.At(i) == "dev" {
			continue
		}
		metric := d.Metric.At(i)
		fn(i, metric, tabOf(metric, d.Tab.At(i)))
	}
}

// loadDays reads the days at once: a cold read parses up to 180 files.
func loadDays(src WebPerf, days []string) ([]*webperf.Day, error) {
	out := make([]*webperf.Day, len(days))
	errs := make([]error, len(days))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, day := range days {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i], errs[i] = src.Day(day)
		}()
	}
	wg.Wait()
	return out, errors.Join(errs...)
}

// perfValues are the values of every tab and metric over a run of days.
func perfValues(loaded []*webperf.Day, includeDev bool) map[perfKey][]float64 {
	out := map[perfKey][]float64{}
	for _, d := range loaded {
		perfRows(d, includeDev, func(i int, metric, tab string) {
			k := perfKey{tab, metric}
			out[k] = append(out[k], d.Value[i])
		})
	}
	return out
}

func p75(vals []float64) *float64 {
	v, ok := webperf.Percentile(vals, 75)
	if !ok {
		return nil
	}
	return &v
}

func statusOf(metric string, v *float64, count int) string {
	c, ok := cutoffs[metric]
	if !ok || v == nil || count < webperf.MinSamples {
		return ""
	}
	switch {
	case *v <= c[0]:
		return "good"
	case *v <= c[1]:
		return "needs-improvement"
	}
	return "poor"
}

// dayLabels are n local days starting offset days after first.
func dayLabels(first time.Time, offset, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = time.Date(first.Year(), first.Month(), first.Day()+offset+i, 0, 0, 0, 0, time.Local).Format(webperf.DayLayout)
	}
	return out
}

func buildWebPerformance(src WebPerf, win limits.StatsWindow, includeDev bool) (wire.StatsWebPerformance, error) {
	n := len(win.Days)
	days := win.Labels()
	out := wire.NewStatsWebPerformance()
	if src == nil {
		return out, nil
	}
	// Load every stored day once: the cards and trend read the window and the
	// window before it, the build strip reads all of history.
	start, hasPrev := days[0], false
	if oldest, ok := src.OldestDay(); ok {
		hasPrev = oldest <= dayLabels(win.Days[0], -n, n)[0]
		start = min(oldest, start)
	}
	loaded, err := loadDays(src, allDays(start, win.Days[n-1]))
	if err != nil {
		return out, err
	}
	loadedCur := loaded[len(loaded)-n:]
	cur := perfValues(loadedCur, includeDev)
	var prev map[perfKey][]float64
	if hasPrev {
		prev = perfValues(loaded[len(loaded)-2*n:len(loaded)-n], includeDev)
	}
	cards := func(tab string, metrics []string) []wire.StatsWebPerfCard {
		out := make([]wire.StatsWebPerfCard, len(metrics))
		for i, m := range metrics {
			vals := cur[perfKey{tab, m}]
			card := wire.StatsWebPerfCard{Metric: m, Count: len(vals), P75: p75(vals)}
			card.Status = statusOf(m, card.P75, card.Count)
			card.ChangePct = pctChange(vals, prev[perfKey{tab, m}])
			out[i] = card
		}
		return out
	}
	out.HardLoads, out.RouteChanges = cards(tabHard, hardMetrics), cards(tabRoute, routeMetrics)

	trend := map[string][][]float64{}
	for _, m := range trendMetrics {
		trend[m] = make([][]float64, n)
	}
	routes := map[string][]float64{}
	for di, d := range loadedCur {
		if !includeDev && !out.DevHidden {
			for i := range d.N {
				if d.Env.At(i) == "dev" {
					out.DevHidden = true
					break
				}
			}
		}
		perfRows(d, includeDev, func(i int, metric, _ string) {
			if t, ok := trend[metric]; ok {
				t[di] = append(t[di], d.Value[i])
			}
			if metric == "route_ready" {
				if r := d.Route.At(i); r != "" {
					routes[r] = append(routes[r], d.Value[i])
				}
			}
		})
	}
	for _, m := range trendMetrics {
		pts := make([]wire.StatsWebPerfPoint, n)
		for i, vals := range trend[m] {
			pts[i] = wire.StatsWebPerfPoint{P75: p75(vals), Count: len(vals)}
		}
		out.Trend = append(out.Trend, wire.StatsWebPerfTrend{Metric: m, Days: pts})
	}
	for r, vals := range routes {
		out.SlowestRoutes = append(out.SlowestRoutes, wire.StatsWebPerfRoute{Route: r, P75: *p75(vals), Count: len(vals)})
	}
	slices.SortFunc(out.SlowestRoutes, func(a, b wire.StatsWebPerfRoute) int {
		return cmp.Or(cmp.Compare(b.P75, a.P75), cmp.Compare(a.Route, b.Route))
	})
	out.SlowestRoutes = out.SlowestRoutes[:min(len(out.SlowestRoutes), slowestRoutes)]
	b := buildWebPerfBuilds(loaded, win, includeDev)
	out.BuildStrip, out.WhySlow, out.PoorINP = b.strip, b.whySlow, b.poorINP
	return out, nil
}
