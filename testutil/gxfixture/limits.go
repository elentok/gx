package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/epicwaiting"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// limitShapes are the --shape values of a limits scenario. Each builds the
// seeded state from the real clock, so countdowns and "seen … ago" stay right.
var limitShapes = map[string]func(now time.Time) wire.LimitsReport{
	// fresh: both windows received now; the weekly one is amber.
	"fresh": func(now time.Time) wire.LimitsReport {
		return wire.LimitsReport{
			FiveHour: limitWindow(now, 34, 2*time.Hour+10*time.Minute, 0),
			SevenDay: limitWindow(now, 81, 52*time.Hour, 0),
		}
	},
	// stale: both received 3 hours ago.
	"stale": func(now time.Time) wire.LimitsReport {
		return wire.LimitsReport{
			FiveHour: limitWindow(now, 34, 2*time.Hour+10*time.Minute, 3*time.Hour),
			SevenDay: limitWindow(now, 52, 52*time.Hour, 3*time.Hour),
		}
	},
	// past-reset: the 5h reset time is an hour gone.
	"past-reset": func(now time.Time) wire.LimitsReport {
		return wire.LimitsReport{
			FiveHour: limitWindow(now, 34, -time.Hour, 6*time.Hour),
			SevenDay: limitWindow(now, 52, 52*time.Hour, 6*time.Hour),
		}
	},
	// one-window: the weekly window only.
	"one-window": func(now time.Time) wire.LimitsReport {
		return wire.LimitsReport{
			SevenDay: limitWindow(now, 12, 5*24*time.Hour+9*time.Hour, 0),
		}
	},
}

var limitWindow = limitseed.Window

func limitShapeNames() []string {
	names := make([]string, 0, len(limitShapes))
	for n := range limitShapes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// checkShape resolves --shape before anything starts: "" is the scenario's default.
func checkShape(sc scenario, shape string) (string, error) {
	var def string
	var names []string
	switch {
	case sc.Limits:
		def, names = "fresh", limitShapeNames()
	case sc.Spend:
		def, names = "normal", spendShapeNames()
	case sc.LimitBurn:
		def, names = "menu", limitseed.ShapeNames()
	case sc.TriageShaped:
		def, names = "c-few", spec.TriageShapes
	case sc.SchedulesShaped:
		def, names = "list", spec.SchedulesShapes
	case sc.RestShaped:
		def, names = spec.RestShapes[0], spec.RestShapes
	case sc.EpicDiffShaped:
		def, names = spec.EpicDiffShapes[0], spec.EpicDiffShapes
	case sc.EpicWaitingShaped:
		def, names = epicwaiting.Shapes[0], epicwaiting.Shapes
	case sc.BgProgressShaped:
		def, names = spec.BgProgressShapes[0], spec.BgProgressShapes
	case sc.SubagentsShaped:
		def, names = spec.SubagentsShapes[0], spec.SubagentsShapes
	case sc.DevMode:
		def, names = spec.DevModeShapes[0], spec.DevModeShapes
	case sc.HostFiles:
		def, names = spec.HostFilesShapes[0], spec.HostFilesShapes
	default:
		if shape != "" {
			return "", fmt.Errorf("--shape only applies to the limits, spend, limit-burn, triage, schedules, rest, epic-diff, epic-waiting, bg-progress, subagents, dev-mode and host-files scenarios")
		}
		return "", nil
	}
	if shape == "" {
		return def, nil
	}
	if !slices.Contains(names, shape) {
		return "", fmt.Errorf("unknown shape %q; known: %s", shape, strings.Join(names, ", "))
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
