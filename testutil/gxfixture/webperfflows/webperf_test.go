package webperfflows

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestMain(m *testing.M) {
	os.Exit(fixturetest.Run(m, nil))
}

// perf reads the web performance section through `gx server stats --json`.
func perf(t *testing.T, r *fixturetest.Running, days int, extra ...string) wire.StatsWebPerformance {
	t.Helper()
	args := append([]string{"server", "stats", "--json", "--range", strconv.Itoa(days)}, extra...)
	var s wire.Stats
	if err := json.Unmarshal([]byte(r.Gx(t, args...)), &s); err != nil {
		t.Fatal(err)
	}
	return s.WebPerformance
}

func cards(p wire.StatsWebPerformance) []wire.StatsWebPerfCard {
	return append(append([]wire.StatsWebPerfCard{}, p.HardLoads...), p.RouteChanges...)
}

func TestWebPerformanceFixture_HasTwoBuildsAndBothTabs(t *testing.T) {
	r := fixturetest.Start(t, "web-performance")
	p := perf(t, r, 7)

	if len(p.HardLoads) != 5 || len(p.RouteChanges) != 3 {
		t.Fatalf("want 5 hard and 3 route cards, got %d and %d", len(p.HardLoads), len(p.RouteChanges))
	}
	for _, c := range cards(p) {
		// A change needs 20 samples in the preceding 7 days too.
		if c.Count < 20 || c.ChangePct == nil {
			t.Errorf("card %s: count %d, change %v; want 20 or more samples and a change", c.Metric, c.Count, c.ChangePct)
		}
	}
	if len(p.SlowestRoutes) < 5 {
		t.Errorf("want 5 slowest routes, got %d", len(p.SlowestRoutes))
	}
	if p.PoorINP.Count < 20 || len(p.PoorINP.TopActions) == 0 || p.PoorINP.WorstChunk == "" {
		t.Errorf("poor INP block is thin: %+v", p.PoorINP)
	}

	b := p.BuildStrip
	if b == nil || b.Previous == nil || b.Previous.BuildID == b.Current.BuildID || b.Current.BuildID == "dev" {
		t.Fatalf("want two real builds: %+v", b)
	}
	if b.LCPChangePct == nil || b.INPChangePct == nil || b.JSChangePct == nil {
		t.Errorf("build strip has a dash: %+v", b)
	}

	for _, days := range []int{30, 90} {
		for _, c := range cards(perf(t, r, days)) {
			if c.ChangePct != nil {
				t.Errorf("range %d, card %s: want no change, got %v", days, c.Metric, *c.ChangePct)
			}
		}
	}
}

func TestWebPerformanceFixture_IncludeDevRaisesCounts(t *testing.T) {
	r := fixturetest.Start(t, "web-performance")
	without := perf(t, r, 7)
	with := perf(t, r, 7, "--include-dev")
	raised := 0
	for i, c := range cards(with) {
		if c.Count > cards(without)[i].Count {
			raised++
		}
	}
	if raised == 0 {
		t.Error("--include-dev changed no card count")
	}
}

func TestWebPerformanceFixture_VolumeSpansTheSampleThreshold(t *testing.T) {
	r := fixturetest.Start(t, "web-performance")
	p := perf(t, r, 30)

	var lcp []wire.StatsWebPerfPoint
	for _, tr := range p.Trend {
		if tr.Metric == "lcp" {
			lcp = tr.Days
		}
	}
	days, lowDays, highDays := 0, 0, 0
	for _, d := range lcp {
		switch {
		case d.Count == 0:
		case d.Count < 20:
			days++
			lowDays++
		default:
			days++
			highDays++
		}
	}
	if days != 14 || lowDays == 0 || highDays == 0 {
		t.Errorf("lcp trend: want 14 days with some under and some at or above 20 samples, got %d days, %d under, %d at or above", days, lowDays, highDays)
	}

	// The server lists only the 5 slowest routes, so it shows the counts on
	// both sides of 20 but not the route total.
	lowRoutes, highRoutes := 0, 0
	for _, rt := range p.SlowestRoutes {
		if rt.Count < 20 {
			lowRoutes++
		} else {
			highRoutes++
		}
	}
	if lowRoutes == 0 || highRoutes == 0 {
		t.Errorf("want some routes under and some at or above 20 samples, got %d under, %d at or above", lowRoutes, highRoutes)
	}
}

func TestWebPerformanceFixture_SeedsDaysRoutesAndDevSamples(t *testing.T) {
	routes := map[string]bool{}
	dev := 0
	data := spec.WebPerfFull()
	for _, d := range data {
		for _, s := range d.Samples {
			if s.Metric == "route_ready" && s.Env == "prod" {
				routes[s.Route] = true
			}
			if s.Env == "dev" {
				dev++
			}
		}
	}
	if len(data) != 14 {
		t.Errorf("want 14 days, got %d", len(data))
	}
	if len(routes) < 6 {
		t.Errorf("want route_ready on 6 or more routes, got %d", len(routes))
	}
	if dev == 0 {
		t.Error("want env dev samples")
	}
}

func TestWebPerformanceLowFixture_HasFewSamplesAndOneBuild(t *testing.T) {
	r := fixturetest.Start(t, "web-performance-low")
	p := perf(t, r, 7)

	for _, c := range cards(p) {
		if c.Count >= 20 {
			t.Errorf("card %s has %d samples, want under 20", c.Metric, c.Count)
		}
	}
	for _, tr := range p.Trend {
		for i, d := range tr.Days {
			if d.Count >= 20 {
				t.Errorf("trend %s day %d has %d samples, want under 20", tr.Metric, i, d.Count)
			}
		}
	}
	if len(p.SlowestRoutes) != 1 || p.SlowestRoutes[0].Count != 6 {
		t.Errorf("want one route with 6 samples, got %+v", p.SlowestRoutes)
	}
	if p.BuildStrip == nil || p.BuildStrip.Previous != nil {
		t.Errorf("want a single build, got %+v", p.BuildStrip)
	}
}
