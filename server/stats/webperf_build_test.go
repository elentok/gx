package stats_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/webperf"
)

func fptr(v float64) *float64 { return &v }

func inBuild(id, version string, s webperf.Sample) webperf.Sample {
	s.BuildID, s.WebVersion = id, version
	return s
}

func whySlow(t *testing.T, rows []wire.StatsWebPerfWhySlow, metric string) wire.StatsWebPerfWhySlow {
	t.Helper()
	i := slices.IndexFunc(rows, func(r wire.StatsWebPerfWhySlow) bool { return r.Metric == metric })
	if i < 0 {
		t.Fatalf("no %s row in %+v", metric, rows)
	}
	return rows[i]
}

func TestStats_WebPerformanceBuildStripComparesCurrentWithPreviousBuild(t *testing.T) {
	t.Parallel()
	old := func(s webperf.Sample) webperf.Sample { return inBuild("a1", "1.0", s) }
	cur := func(s webperf.Sample) webperf.Sample { return inBuild("b2", "1.1", s) }
	js := func(s webperf.Sample, v float64) webperf.Sample { s.JSBytes = fptr(v); return s }
	get := perfStats(t,
		perfRepeat(20, 20, old(perfSample("lcp", "hard", 1000))),
		perfRepeat(5, 20, old(perfSample("lcp", "hard", 1000))),
		perfRepeat(5, 20, old(perfSample("inp", "route", 200))),
		perfRepeat(5, 20, old(js(perfSample("lcp", "hard", 1000), 100))),
		perfRepeat(1, 20, cur(perfSample("lcp", "hard", 1500))),
		perfRepeat(1, 10, cur(perfSample("inp", "hard", 300))),
		perfRepeat(1, 10, cur(perfSample("inp", "route", 300))),
		perfRepeat(1, 19, cur(js(perfSample("lcp", "hard", 1500), 150))),
		// A dev build seen last stays out unless include_dev is on.
		perfRepeat(0, 20, inBuild("c3", "1.2", webperf.Sample{Metric: "lcp", Tab: "hard", Value: 9000, Env: "dev"})),
	)

	strip := get("?range=7").WebPerformance.BuildStrip
	if strip == nil || strip.Previous == nil {
		t.Fatalf("strip = %+v, want current and previous", strip)
	}
	if strip.Current.BuildID != "b2" || strip.Current.WebVersion != "1.1" || strip.Previous.BuildID != "a1" || strip.Previous.WebVersion != "1.0" {
		t.Errorf("builds = %+v / %+v, want b2 1.1 / a1 1.0", strip.Current, *strip.Previous)
	}
	ptrVal(t, "lcp change", strip.LCPChangePct, 50)
	ptrVal(t, "inp change", strip.INPChangePct, 50)
	ptrVal(t, "current lcp p75", strip.Current.LCPP75, 1500)
	if strip.JSChangePct != nil {
		t.Errorf("js change = %v, want nil with 19 current samples", *strip.JSChangePct)
	}

	withDev := get("?range=7&include_dev=true").WebPerformance.BuildStrip
	if withDev.Current.BuildID != "c3" || withDev.Previous.BuildID != "b2" {
		t.Errorf("with dev builds = %s / %s, want c3 / b2", withDev.Current.BuildID, withDev.Previous.BuildID)
	}
}

func TestStats_WebPerformanceBuildStripPreviousSeenOutsideRangeHasNullChanges(t *testing.T) {
	t.Parallel()
	get := perfStats(t,
		perfRepeat(20, 20, inBuild("a1", "1.0", perfSample("lcp", "hard", 1000))),
		perfRepeat(1, 20, inBuild("b2", "1.1", perfSample("lcp", "hard", 1500))),
	)
	strip := get("?range=7").WebPerformance.BuildStrip
	if strip == nil || strip.Previous == nil || strip.Previous.BuildID != "a1" {
		t.Fatalf("strip = %+v, want previous a1", strip)
	}
	if strip.LCPChangePct != nil || strip.Previous.LCPP75 != nil {
		t.Errorf("previous with no samples in range = %+v, change %v, want nil", strip.Previous, strip.LCPChangePct)
	}
}

func TestStats_WebPerformanceBuildStripWithOneBuildHasNoPrevious(t *testing.T) {
	t.Parallel()
	get := perfStats(t, perfRepeat(1, 20, inBuild("b2", "1.1", perfSample("lcp", "hard", 1500))))
	wp := get("?range=7").WebPerformance
	if wp.BuildStrip == nil || wp.BuildStrip.Previous != nil || wp.BuildStrip.LCPChangePct != nil {
		t.Errorf("strip = %+v, want current only and no change", wp.BuildStrip)
	}
	if empty := get("?range=7").WebPerformance; empty.PoorINP.TopActions == nil {
		t.Errorf("top actions must be a list, not null")
	}
}

func TestStats_WebPerformanceWhySlowRowsCompareBuildsAndSkipZeroTransfers(t *testing.T) {
	t.Parallel()
	with := func(build string, ttfb, api, jsb, css float64) webperf.Sample {
		s := inBuild(build, "v-"+build, perfSample("app_ready", "hard", 1000))
		s.TTFB, s.APILatency, s.JSBytes, s.CSSBytes = fptr(ttfb), fptr(api), fptr(jsb), fptr(css)
		return s
	}
	route := with("b2", 0, 400, 0, 0)
	route.Metric, route.Tab = "route_ready", "route"
	get := perfStats(t,
		perfRepeat(5, 20, with("a1", 100, 200, 1000, 50)),
		perfRepeat(1, 20, with("b2", 150, 300, 1500, 0)),
		perfRepeat(1, 20, route),
	)
	rows := get("?range=7").WebPerformance.WhySlow

	ttfb := whySlow(t, rows, "ttfb")
	ptrVal(t, "ttfb p75", ttfb.P75, 150)
	ptrVal(t, "ttfb previous", ttfb.PreviousP75, 100)
	ptrVal(t, "ttfb change", ttfb.ChangePct, 50)

	// API latency counts both readiness metrics: 20 at 300 and 20 at 400.
	api := whySlow(t, rows, "api_latency")
	if api.Count != 40 {
		t.Errorf("api count = %d, want 40", api.Count)
	}
	ptrVal(t, "api p75", api.P75, 400)

	js := whySlow(t, rows, "js_bytes")
	if js.Count != 20 {
		t.Errorf("js count = %d, want 20 (route sample is not a hard load)", js.Count)
	}
	ptrVal(t, "js change", js.ChangePct, 50)

	// Zero means cached or unknown: no samples, no p75, no change.
	css := whySlow(t, rows, "css_bytes")
	if css.Count != 0 || css.P75 != nil || css.ChangePct != nil {
		t.Errorf("css = %+v, want zero count and nil values", css)
	}
	ptrVal(t, "css previous", css.PreviousP75, 50)
}

func TestStats_WebPerformancePoorINPCoversBothTabsAndAllBuilds(t *testing.T) {
	t.Parallel()
	poor := func(build, tab, action, chunk string, v float64) webperf.Sample {
		s := inBuild(build, "v", perfSample("inp", tab, v))
		s.InputDelay, s.Processing, s.PresentationDelay = fptr(10), fptr(v/2), fptr(30)
		s.Action, s.WorstChunk = action, chunk
		return s
	}
	get := perfStats(t,
		perfRepeat(1, 4, poor("a1", "hard", "save", "editor", 400)),
		perfRepeat(1, 3, poor("b2", "route", "open", "editor", 400)),
		perfRepeat(1, 2, poor("b2", "route", "close", "", 400)),
		perfRepeat(1, 2, poor("b2", "route", "alpha", "", 400)),
		perfRepeat(1, 5, poor("b2", "route", "zzz", "", 200)), // not poor: 200 is the limit
		perfRepeat(1, 1, poor("b2", "route", "", "graph", 400)),
	)
	p := get("?range=7").WebPerformance.PoorINP
	if p.Count != 12 {
		t.Errorf("poor count = %d, want 12", p.Count)
	}
	ptrVal(t, "input delay", p.InputDelayP75, 10)
	ptrVal(t, "processing", p.ProcessingP75, 200)
	ptrVal(t, "presentation", p.PresentationDelayP75, 30)
	want := []wire.StatsWebPerfAction{{Action: "save", Count: 4}, {Action: "open", Count: 3}, {Action: "alpha", Count: 2}}
	if !slices.Equal(p.TopActions, want) {
		t.Errorf("top actions = %+v, want %+v (ties by name)", p.TopActions, want)
	}
	if p.WorstChunk != "editor" {
		t.Errorf("worst chunk = %q, want editor (samples without one do not vote)", p.WorstChunk)
	}
}
