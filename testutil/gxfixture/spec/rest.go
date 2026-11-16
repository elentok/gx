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
// cuts a configured message at the first sentence end, so the seeder writes
// these two into the stored Break (see seedRest).
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
	clock := func(t time.Time) string { return t.Format("15:04") }
	cfg := config.MentalHealthConfig{
		Enabled: true,
		Break:   config.MentalHealthBreak{WorkMinutes: restWorkMinutes, BreakMinutes: restBreakMinutes},
	}
	var seed *RestSeed
	var overrides []string
	setBedtime := func(start, end time.Time) {
		overrides = append(overrides,
			"mental-health.bedtime.enabled=true",
			"mental-health.bedtime.start="+jsonString(clock(start)),
			"mental-health.bedtime.end="+jsonString(clock(end)))
	}
	setBreak := func(message string) {
		overrides = append(overrides,
			"mental-health.break.enabled=true",
			fmt.Sprintf("mental-health.break.work-minutes=%d", restWorkMinutes),
			fmt.Sprintf("mental-health.break.break-minutes=%d", restBreakMinutes))
		if message != "" {
			overrides = append(overrides, "mental-health.break.message="+jsonString(message))
		}
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
		setBreak("")
		// The 55th active minute starts the Break, so it is the Break's start.
		start := now.Add(restBreakLeft - restBreakMinutes*time.Minute)
		seed = &RestSeed{Config: cfg, Active: minutes(restWorkMinutes, start)}
	case "break-warning":
		setBreak("")
		// Two below Work time, last active minute now: the warning is due.
		cfg.Break.Enabled = true
		seed = &RestSeed{Config: cfg, Active: minutes(restWorkMinutes-2, now)}
	case "bedtime-warning":
		setBedtime(now.Add(10*time.Minute), now.Add(3*time.Hour))
	default:
		return Scenario{}, false
	}
	keys := append([]string{"mental-health.enabled=true"}, overrides...)
	return Scenario{Projects: []string{project}, Config: keys, Rest: seed}, true
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
