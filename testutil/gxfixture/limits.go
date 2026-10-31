package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/elentok/gx/server"
)

// limitShapes are the --shape values of a limits scenario. Each builds the
// seeded state from the real clock, so countdowns and "seen … ago" stay right.
var limitShapes = map[string]func(now time.Time) server.LimitsReport{
	// fresh: both windows received now; the weekly one is amber.
	"fresh": func(now time.Time) server.LimitsReport {
		return server.LimitsReport{
			FiveHour: limitWindow(now, 34, 2*time.Hour+10*time.Minute, 0),
			SevenDay: limitWindow(now, 81, 52*time.Hour, 0),
		}
	},
	// stale: both received 3 hours ago.
	"stale": func(now time.Time) server.LimitsReport {
		return server.LimitsReport{
			FiveHour: limitWindow(now, 34, 2*time.Hour+10*time.Minute, 3*time.Hour),
			SevenDay: limitWindow(now, 52, 52*time.Hour, 3*time.Hour),
		}
	},
	// past-reset: the 5h reset time is an hour gone.
	"past-reset": func(now time.Time) server.LimitsReport {
		return server.LimitsReport{
			FiveHour: limitWindow(now, 34, -time.Hour, 6*time.Hour),
			SevenDay: limitWindow(now, 52, 52*time.Hour, 6*time.Hour),
		}
	},
	// one-window: the weekly window only.
	"one-window": func(now time.Time) server.LimitsReport {
		return server.LimitsReport{
			SevenDay: limitWindow(now, 12, 5*24*time.Hour+9*time.Hour, 0),
		}
	},
}

// limitWindow resets in resetsIn and was received receivedAgo before now.
func limitWindow(now time.Time, pct float64, resetsIn, receivedAgo time.Duration) *server.LimitWindow {
	return &server.LimitWindow{
		UsedPercentage: pct,
		ResetsAt:       now.Add(resetsIn).Unix(),
		ReceivedAt:     now.Add(-receivedAgo).Unix(),
	}
}

func limitShapeNames() []string {
	names := make([]string, 0, len(limitShapes))
	for n := range limitShapes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// checkLimitShape resolves the --shape value ("" is fresh) before anything starts.
func checkLimitShape(sc scenario, shape string) (string, error) {
	if !sc.limits {
		if shape != "" {
			return "", fmt.Errorf("--shape only applies to the limits scenario")
		}
		return "", nil
	}
	if shape == "" {
		return "fresh", nil
	}
	if _, ok := limitShapes[shape]; !ok {
		return "", fmt.Errorf("unknown shape %q; known: %s", shape, strings.Join(limitShapeNames(), ", "))
	}
	return shape, nil
}

// seedLimits writes the limits state file the server loads at start.
func (f *fixture) seedLimits(shape string) error {
	data, err := json.Marshal(limitShapes[shape](time.Now()))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.stateD, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.stateD, "claude-limits.json"), data, 0o600)
}
