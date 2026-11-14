package stats_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/webperf"
)

// perfSeed is a set of samples written straight into the day files, n days ago.
type perfSeed struct {
	daysAgo int
	sample  webperf.Sample
}

func perfSample(metric, tab string, v float64) webperf.Sample {
	return webperf.Sample{Metric: metric, Tab: tab, Value: v, Env: "prod"}
}

// perfRepeat is count copies of s, all on the same day.
func perfRepeat(daysAgo, count int, s webperf.Sample) []perfSeed {
	out := make([]perfSeed, count)
	for i := range out {
		out[i] = perfSeed{daysAgo, s}
	}
	return out
}

func perfStats(t *testing.T, seeds ...[]perfSeed) (get func(query string) wire.Stats) {
	t.Helper()
	h, base := rigs.StartEpicDetail(t, func(string) {})
	byDay := map[int][]string{}
	n := 0
	for _, group := range seeds {
		for _, s := range group {
			n++
			s.sample.ID, s.sample.Session = fmt.Sprintf("id-%d", n), fmt.Sprintf("s-%d", n/400)
			line, err := json.Marshal(s.sample)
			if err != nil {
				t.Fatal(err)
			}
			byDay[s.daysAgo] = append(byDay[s.daysAgo], string(line))
		}
	}
	dir := filepath.Join(h.StateDir, webperf.DirName)
	for ago, lines := range byDay {
		path := filepath.Join(dir, statsNoon(ago).Format(webperf.DayLayout)+".jsonl")
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return func(query string) wire.Stats {
		t.Helper()
		code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/stats"+query)
		if code != http.StatusOK {
			t.Fatalf("GET /v1/stats%s = %d: %s", query, code, body)
		}
		var s wire.Stats
		if err := json.Unmarshal([]byte(body), &s); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
		return s
	}
}

func perfCard(t *testing.T, cards []wire.StatsWebPerfCard, metric string) wire.StatsWebPerfCard {
	t.Helper()
	i := slices.IndexFunc(cards, func(c wire.StatsWebPerfCard) bool { return c.Metric == metric })
	if i < 0 {
		t.Fatalf("no %s card in %+v", metric, cards)
	}
	return cards[i]
}

func ptrVal(t *testing.T, name string, p *float64, want float64) {
	t.Helper()
	if p == nil || *p != want {
		t.Errorf("%s = %v, want %v", name, p, want)
	}
}

func TestStats_WebPerformanceCardsCarryP75StatusAndChange(t *testing.T) {
	t.Parallel()
	var appReady []perfSeed
	for i := 1; i <= 20; i++ {
		appReady = append(appReady, perfSeed{1, perfSample("app_ready", "hard", float64(i))})
	}
	get := perfStats(t,
		perfRepeat(1, 20, perfSample("lcp", "hard", 1500)),
		perfRepeat(13, 20, perfSample("lcp", "hard", 1000)),
		perfRepeat(1, 19, perfSample("inp", "hard", 300)),
		perfRepeat(1, 20, perfSample("inp", "route", 600)),
		perfRepeat(1, 20, perfSample("fcp", "", 2500)),
		appReady,
	)
	wp := get("?range=7").WebPerformance

	lcp := perfCard(t, wp.HardLoads, "lcp")
	ptrVal(t, "lcp p75", lcp.P75, 1500)
	ptrVal(t, "lcp change", lcp.ChangePct, 50)
	if lcp.Count != 20 || lcp.Status != "good" {
		t.Errorf("lcp = %+v, want count 20, good", lcp)
	}

	inp := perfCard(t, wp.HardLoads, "inp")
	ptrVal(t, "hard inp p75", inp.P75, 300)
	if inp.Count != 19 || inp.Status != "" || inp.ChangePct != nil {
		t.Errorf("hard inp = %+v, want count 19, no status, no change", inp)
	}

	routeINP := perfCard(t, wp.RouteChanges, "inp")
	ptrVal(t, "route inp p75", routeINP.P75, 600)
	if routeINP.Status != "poor" {
		t.Errorf("route inp status = %q, want poor", routeINP.Status)
	}

	// A sample with no tab is a hard load; the preceding window has no samples.
	fcp := perfCard(t, wp.HardLoads, "fcp")
	if fcp.Status != "needs-improvement" || fcp.ChangePct != nil {
		t.Errorf("fcp = %+v, want needs-improvement and no change", fcp)
	}

	app := perfCard(t, wp.HardLoads, "app_ready")
	ptrVal(t, "app_ready nearest-rank p75 of 1..20", app.P75, 15)
	if app.Status != "" {
		t.Errorf("app_ready status = %q, want none", app.Status)
	}

	cls := perfCard(t, wp.HardLoads, "cls")
	if cls.Count != 0 || cls.P75 != nil {
		t.Errorf("cls with no samples = %+v", cls)
	}
}

func TestStats_WebPerformanceChangeIsNullWithoutStoredPrecedingWindow(t *testing.T) {
	t.Parallel()
	// The oldest file is day 5, inside the 7-day window: the window before it
	// (days 7 to 13) is not covered, even though it would hold samples.
	get := perfStats(t, perfRepeat(1, 20, perfSample("lcp", "hard", 1500)), perfRepeat(5, 20, perfSample("lcp", "hard", 1000)))
	lcp := perfCard(t, get("?range=7").WebPerformance.HardLoads, "lcp")
	if lcp.Count != 40 || lcp.ChangePct != nil {
		t.Errorf("lcp = %+v, want count 40 and null change", lcp)
	}
}

func TestStats_WebPerformanceChangeIsAlwaysNullFor90Days(t *testing.T) {
	t.Parallel()
	get := perfStats(t, perfRepeat(1, 20, perfSample("lcp", "hard", 1500)), perfRepeat(100, 20, perfSample("lcp", "hard", 1000)))
	lcp := perfCard(t, get("?range=90").WebPerformance.HardLoads, "lcp")
	if lcp.ChangePct != nil {
		t.Errorf("90-day change = %v, want null", *lcp.ChangePct)
	}
}

func TestStats_WebPerformanceDevSamplesNeedIncludeDev(t *testing.T) {
	t.Parallel()
	dev := perfSample("lcp", "hard", 9000)
	dev.Env = "dev"
	get := perfStats(t, perfRepeat(1, 20, perfSample("lcp", "hard", 1000)), perfRepeat(1, 25, dev))

	lcp := perfCard(t, get("").WebPerformance.HardLoads, "lcp")
	if lcp.Count != 20 {
		t.Errorf("without include_dev count = %d, want 20", lcp.Count)
	}
	lcp = perfCard(t, get("?include_dev=true").WebPerformance.HardLoads, "lcp")
	if lcp.Count != 45 {
		t.Errorf("with include_dev count = %d, want 45", lcp.Count)
	}
	ptrVal(t, "include_dev p75", lcp.P75, 9000)
}

func TestStats_WebPerformanceBadIncludeDevIs400(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	for _, v := range []string{"yes", "false", "1"} {
		if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/stats?include_dev="+v); code != http.StatusBadRequest {
			t.Errorf("include_dev=%s = %d, want 400", v, code)
		}
	}
}

func TestStats_WebPerformanceIsPresentWithNoSamples(t *testing.T) {
	t.Parallel()
	get := perfStats(t)
	wp := get("?range=30").WebPerformance
	if len(wp.HardLoads) != 5 || len(wp.RouteChanges) != 3 || len(wp.Trend) != 6 || wp.SlowestRoutes == nil || len(wp.SlowestRoutes) != 0 {
		t.Fatalf("empty section = %+v", wp)
	}
	for _, c := range append(slices.Clone(wp.HardLoads), wp.RouteChanges...) {
		if c.Count != 0 || c.P75 != nil || c.Status != "" || c.ChangePct != nil {
			t.Errorf("card %+v, want empty", c)
		}
	}
	for _, tr := range wp.Trend {
		if len(tr.Days) != 30 {
			t.Errorf("trend %s has %d points, want 30", tr.Metric, len(tr.Days))
		}
	}
}

func TestStats_WebPerformanceTrendIsDailyAcrossBothTabs(t *testing.T) {
	t.Parallel()
	get := perfStats(t,
		perfRepeat(2, 3, perfSample("inp", "hard", 100)),
		perfRepeat(2, 1, perfSample("inp", "route", 400)),
		perfRepeat(0, 5, perfSample("lcp", "hard", 2000)),
	)
	s := get("?range=7")
	var inp, lcp wire.StatsWebPerfTrend
	for _, tr := range s.WebPerformance.Trend {
		switch tr.Metric {
		case "inp":
			inp = tr
		case "lcp":
			lcp = tr
		}
	}
	if len(inp.Days) != len(s.Days) {
		t.Fatalf("inp has %d points, want %d", len(inp.Days), len(s.Days))
	}
	day2 := slices.Index(s.Days, statsDate(2))
	if inp.Days[day2].Count != 4 {
		t.Errorf("inp count on day 2 = %d, want 4 (both tabs)", inp.Days[day2].Count)
	}
	ptrVal(t, "inp p75 on day 2", inp.Days[day2].P75, 100)
	if empty := inp.Days[slices.Index(s.Days, statsDate(3))]; empty.Count != 0 || empty.P75 != nil {
		t.Errorf("a day with no samples = %+v, want no value", empty)
	}
	today := lcp.Days[len(lcp.Days)-1]
	ptrVal(t, "lcp p75 today", today.P75, 2000)
	if today.Count != 5 {
		t.Errorf("lcp count today = %d, want 5", today.Count)
	}
}

func TestStats_WebPerformanceSlowestRoutesAreTheFiveHighestP75(t *testing.T) {
	t.Parallel()
	var seeds [][]perfSeed
	for i, route := range []string{"epics", "epic", "agents", "stats", "tips", "design", "ticket"} {
		s := perfSample("route_ready", "route", float64(100*(i+1)))
		s.Route = route
		seeds = append(seeds, perfRepeat(1, 20, s))
	}
	few := perfSample("route_ready", "route", 5000)
	few.Route = "settings"
	seeds = append(seeds, perfRepeat(1, 3, few))
	app := perfSample("app_ready", "hard", 9000)
	app.Route = "cleanup"
	seeds = append(seeds, perfRepeat(1, 20, app))

	got := perfStats(t, seeds...)("?range=7").WebPerformance.SlowestRoutes
	var names []string
	for _, r := range got {
		names = append(names, r.Route)
	}
	if want := []string{"settings", "ticket", "design", "tips", "stats"}; !slices.Equal(names, want) {
		t.Fatalf("routes = %v, want %v", names, want)
	}
	if got[0].Count != 3 || got[1].Count != 20 || got[1].P75 != 700 {
		t.Errorf("rows = %+v", got)
	}
}
