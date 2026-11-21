// Package perfdata holds the canned web performance samples of the
// web-performance scenarios. It is apart from spec to keep spec under its size limit.
package perfdata

import (
	"fmt"

	"github.com/elentok/gx/webperf"
)

// WebPerfDay is one past local day of seeded web performance samples.
type WebPerfDay struct {
	DaysAgo int
	Samples []webperf.Sample
}

// perfDayStride starts each day's counter at a different place. The value
// pattern repeats every 1000 steps, so a stride of 1000 would copy one day.
const perfDayStride = 1013

type perfBuild struct{ id, version string }

var (
	perfOldBuild = perfBuild{"b-1f3a9c", "1.8.0"}
	perfNewBuild = perfBuild{"b-7c2e55", "1.9.0"}
	perfLowBuild = perfBuild{"b-0a1b2c", "1.0.0"}
	perfDevBuild = perfBuild{"dev", "dev"}

	perfRoutes   = []string{"epics", "epic", "ticket", "stats", "agents", "settings", "design"}
	perfActions  = []string{"click-save-ticket", "open-command-menu", "type-search", "toggle-sidebar"}
	perfChunks   = []string{"vendor-charts", "epic-page", "markdown"}
	perfTriggers = []string{"click", "keyboard", "history"}
)

// perfGen builds samples from a counter, so a run always gives the same data.
type perfGen struct {
	n       int
	daysAgo int
	out     []webperf.Sample
}

// u is the next pseudo-random number in [0, 1).
func (g *perfGen) u() float64 {
	g.n++
	return float64((g.n*7919)%1000) / 1000
}

func ptr(v float64) *float64 { return &v }

// add appends one sample. lo and hi bound its value; edit may set the rest.
func (g *perfGen) add(b perfBuild, env, metric, tab string, lo, hi float64, edit func(s *webperf.Sample, u float64)) {
	u := g.u()
	s := webperf.Sample{
		ID:         fmt.Sprint(g.n),
		Session:    fmt.Sprintf("s-%d-%d", g.daysAgo, g.n%3),
		Metric:     metric,
		Value:      lo + u*(hi-lo),
		BuildID:    b.id,
		WebVersion: b.version,
		Env:        env,
		Tab:        tab,
	}
	if edit != nil {
		edit(&s, u)
	}
	g.out = append(g.out, s)
}

func (g *perfGen) lcp(b perfBuild, env string, lo, hi float64) {
	g.add(b, env, "lcp", "hard", lo, hi, func(s *webperf.Sample, u float64) {
		s.NavigationType = []string{"navigate", "reload"}[g.n%2]
		s.TTFB, s.APILatency = ptr(80+u*400), ptr(100+u*500)
		s.CSSBytes = ptr(30000 + u*8000)
		// Every fifth load comes from the cache, with no transfer.
		if g.n%5 != 0 {
			s.JSBytes = ptr(280000 + u*40000)
		} else {
			s.JSBytes = ptr(0)
		}
	})
}

func (g *perfGen) inp(b perfBuild, env, tab string, lo, hi float64) {
	g.add(b, env, "inp", tab, lo, hi, func(s *webperf.Sample, u float64) {
		if s.Value <= 200 {
			return
		}
		s.InputDelay, s.Processing, s.PresentationDelay = ptr(20+u*80), ptr(60+u*250), ptr(10+u*100)
		s.Action, s.WorstChunk = perfActions[g.n%len(perfActions)], perfChunks[g.n%len(perfChunks)]
		s.InteractionType, s.WorstFrame = []string{"pointer", "keyboard"}[g.n%2], ptr(60+u*140)
	})
}

// ready adds an app_ready or route_ready sample with its outcome and source.
func (g *perfGen) ready(b perfBuild, env, metric, tab, route string, lo, hi float64) {
	g.add(b, env, metric, tab, lo, hi, func(s *webperf.Sample, u float64) {
		s.Route = route
		s.APILatency = ptr(100 + u*500)
		s.Outcome = "ready"
		switch {
		case g.n%11 == 0:
			s.Outcome = "error"
		case g.n%13 == 0:
			s.Outcome = "empty"
		}
		s.DataSource = []string{"cache", "network"}[g.n%2]
		if metric == "route_ready" {
			s.Trigger = perfTriggers[g.n%len(perfTriggers)]
		}
	})
}

func (g *perfGen) cls(b perfBuild, env, tab string, hi float64) {
	g.add(b, env, "cls", tab, 0, hi, nil)
}

// WebPerfFull is the web-performance scenario: 14 days ending today, two
// builds that overlap, and a few dev samples a day. The old build is first
// seen on the oldest day and the new one 7 days later.
func WebPerfFull() []WebPerfDay {
	var days []WebPerfDay
	for ago := 13; ago >= 0; ago-- {
		g := &perfGen{n: ago * perfDayStride, daysAgo: ago}
		// Dev samples come first, so the dev build is never the newest one seen.
		g.lcp(perfDevBuild, "dev", 3500, 5500)
		g.lcp(perfDevBuild, "dev", 3500, 5500)
		g.inp(perfDevBuild, "dev", "hard", 250, 700)
		g.ready(perfDevBuild, "dev", "route_ready", "route", "epic", 1500, 3000)
		build := func() perfBuild {
			// Half of the second week still runs the old build, which keeps
			// 20 or more samples of it in the last 7 days.
			if ago <= 6 && (g.n*7919)%1000 < 500 {
				return perfNewBuild
			}
			return perfOldBuild
		}
		for range 10 {
			g.lcp(build(), "prod", 1400, 4400)
		}
		for range 6 {
			g.add(build(), "prod", "fcp", "hard", 900, 2500, nil)
		}
		for range 6 {
			g.ready(build(), "prod", "app_ready", "hard", "", 600, 2400)
		}
		for range 4 {
			g.inp(build(), "prod", "hard", 80, 600)
			g.cls(build(), "prod", "hard", 0.2)
			g.inp(build(), "prod", "route", 80, 600)
			g.cls(build(), "prod", "route", 0.3)
		}
		// The last 3 days are busy, so the lcp trend has days on both sides of 20.
		if ago <= 2 {
			for range 12 {
				g.lcp(build(), "prod", 1400, 4400)
			}
		}
		// The first 5 routes get 2 samples a day and the last 2 get 1, so
		// over 14 days some routes reach 20 samples and some stay under.
		for i := range 12 {
			r := i % len(perfRoutes)
			// Later routes are slower, so the slowest-routes list has an order.
			g.ready(build(), "prod", "route_ready", "route", perfRoutes[r], 300, 1800+float64(r)*400)
		}
		days = append(days, WebPerfDay{DaysAgo: ago, Samples: g.out})
	}
	return days
}

// WebPerfLow is the web-performance-low scenario: one build, 3 days, under 20
// samples of every metric, and one route with 6 route_ready samples.
func WebPerfLow() []WebPerfDay {
	var days []WebPerfDay
	for ago := 2; ago >= 0; ago-- {
		g := &perfGen{n: ago * perfDayStride, daysAgo: ago}
		b := perfLowBuild
		for range 3 {
			g.lcp(b, "prod", 1400, 4400)
			g.add(b, "prod", "fcp", "hard", 900, 2500, nil)
			g.ready(b, "prod", "app_ready", "hard", "", 600, 2400)
		}
		for range 2 {
			g.inp(b, "prod", "hard", 80, 600)
			g.cls(b, "prod", "hard", 0.2)
			g.inp(b, "prod", "route", 80, 600)
			g.cls(b, "prod", "route", 0.3)
			g.ready(b, "prod", "route_ready", "route", "epics", 300, 1800)
		}
		days = append(days, WebPerfDay{DaysAgo: ago, Samples: g.out})
	}
	return days
}
