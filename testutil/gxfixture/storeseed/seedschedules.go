package storeseed

import (
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/schedules"
	serverschedules "github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// SeedSchedules writes the scenario's Schedule files with the schedules store and
// their switch, baseline and skip log with the server's state API, so the server
// reads what its own writes would have made. The machine id is fixed first so the
// seeded One-offs' Dedupe keys match the server's.
func SeedSchedules(stateDir, storeDir string, sc spec.Scenario) error {
	if len(sc.Schedules) == 0 {
		return nil
	}
	state, err := serverschedules.OpenWithID(stateDir, spec.SchedulesMachineID)
	if err != nil {
		return err
	}
	store := schedules.NewStore(storeDir)
	for _, s := range sc.Schedules {
		// A folder without project.json is a project the server does not know.
		if err := os.MkdirAll(filepath.Join(storeDir, s.Project), 0o755); err != nil {
			return err
		}
		made, err := store.Create(s.Project, schedules.Schedule{Name: s.Name, Prompt: s.Prompt, Days: s.Days, Times: s.Times, LateLimitHours: s.LateLimitHours})
		if err != nil {
			return err
		}
		key := serverschedules.Key{Project: s.Project, Slug: made.Slug}
		if s.On {
			if err := state.Enable(key, time.Now()); err != nil {
				return err
			}
		}
		for _, sk := range s.Skips {
			if err := state.AppendSkip(key, sk); err != nil {
				return err
			}
		}
	}
	return nil
}
