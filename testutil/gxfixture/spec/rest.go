package spec

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/elentok/gx/config"
)

// RestShapes are the --shape names of the rest scenario. Each shape is its own
// server start; the first is the default.
var RestShapes = []string{"bedtime", "break", "break-warning", "bedtime-warning"}

// The Break text of the stored mock. The engine picks its text at random and
// cuts a configured message at the first sentence end, so the seeder passes
// these two to the runner (see storeseed.SeedRest).
const (
	RestBreakMessage    = "Fifty-five minutes in a row? The agents will survive without you."
	RestBreakSubMessage = "Stand up, drink some water, look at something far away."
)

const (
	restWorkMinutes  = 55
	restBreakMinutes = 5
	// restBreakLeft is how long the break shape's Break still has to run.
	restBreakLeft = 4*time.Minute + 37*time.Second
)

// RestSeed is the Rest state written before the server starts: the runner is
// opened on a clock that reads each of Active in turn, and an Active heartbeat
// is sent at each, so the state and the Break start record are what the
// runner's own writes make. Config is the runner's settings for that run.
type RestSeed struct {
	Config config.MentalHealthConfig
	Active []time.Time
}

// RestScenario is the rest scenario in the given shape. Every time is relative
// to now, so the state is right whenever the fixture runs.
func RestScenario(shape string) (Scenario, bool) {
	now := time.Now()
	cfg := config.MentalHealthConfig{
		Enabled: true,
		Break:   config.MentalHealthBreak{WorkMinutes: restWorkMinutes, BreakMinutes: restBreakMinutes},
	}
	var seed *RestSeed
	setBedtime := func(start, end time.Time) {
		cfg.Bedtime = config.MentalHealthBedtime{Enabled: true, Start: start.Format("15:04"), End: end.Format("15:04")}
	}
	// minutes returns n heartbeat times, one a minute, the last at last.
	minutes := func(n int, last time.Time) []time.Time {
		out := make([]time.Time, n)
		for i := range out {
			out[i] = last.Add(-time.Duration(n-1-i) * time.Minute)
		}
		return out
	}
	switch shape {
	case "bedtime":
		setBedtime(now.Add(-time.Hour), now.Add(time.Hour))
	case "break":
		cfg.Break.Enabled = true
		// The 55th active minute starts the Break, so it is the Break's start.
		start := now.Add(restBreakLeft - restBreakMinutes*time.Minute)
		seed = &RestSeed{Config: cfg, Active: minutes(restWorkMinutes, start)}
	case "break-warning":
		// Two below Work time, last active minute now: the warning is due.
		cfg.Break.Enabled = true
		seed = &RestSeed{Config: cfg, Active: minutes(restWorkMinutes-2, now)}
	case "bedtime-warning":
		setBedtime(now.Add(10*time.Minute), now.Add(3*time.Hour))
	default:
		return Scenario{}, false
	}
	return Scenario{Projects: []string{project}, Config: restOverrides(cfg), Rest: seed}, true
}

// restOverrides is the server's config as `dotted.key=<json>` overrides,
// derived from the same cfg the seed runs the runner on so the two cannot drift.
func restOverrides(cfg config.MentalHealthConfig) []string {
	out := []string{"mental-health.enabled=" + fmt.Sprint(cfg.Enabled)}
	if cfg.Bedtime.Enabled {
		out = append(out,
			"mental-health.bedtime.enabled=true",
			"mental-health.bedtime.start="+jsonString(cfg.Bedtime.Start),
			"mental-health.bedtime.end="+jsonString(cfg.Bedtime.End))
	}
	if cfg.Break.Enabled {
		out = append(out,
			"mental-health.break.enabled=true",
			fmt.Sprintf("mental-health.break.work-minutes=%d", cfg.Break.WorkMinutes),
			fmt.Sprintf("mental-health.break.break-minutes=%d", cfg.Break.BreakMinutes))
	}
	return out
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
