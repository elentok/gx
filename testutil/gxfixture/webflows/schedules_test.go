package webflows

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func (r *running) schedulesList(t *testing.T) []wire.ScheduleInfo {
	t.Helper()
	var res wire.SchedulesResult
	if err := json.Unmarshal([]byte(r.Gx(t, "schedules", "list", "--json")), &res); err != nil {
		t.Fatal(err)
	}
	return res.Schedules
}

func (r *running) scheduleRuns(t *testing.T, addr string) []wire.ScheduleRun {
	t.Helper()
	var res wire.ScheduleShowResult
	if err := json.Unmarshal([]byte(r.Gx(t, "schedules", "show", addr, "--json")), &res); err != nil {
		t.Fatal(err)
	}
	return res.LastRuns
}

func TestSchedulesScenario_EveryShapeSeedsWhatItNames(t *testing.T) {
	t.Parallel()
	for _, w := range []struct {
		shape string
		names []string
		on    int
		found bool
	}{
		{"list", []string{"Weather check", "Morning PR digest", "Weekly cleanup report"}, 2, true},
		{"all-off", []string{"Weather check", "Morning PR digest", "Weekly cleanup report"}, 0, true},
		{"empty", nil, 0, true},
		{"page", []string{"Weather check"}, 1, true},
		{"paused", []string{"Weather check"}, 1, true},
		{"missing-project", []string{"Weather check"}, 1, false},
	} {
		t.Run(w.shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "schedules", nil, "--shape", w.shape)
			got := r.schedulesList(t)
			if len(got) != len(w.names) {
				t.Fatalf("%d Schedules, want %d: %+v", len(got), len(w.names), got)
			}
			on := 0
			for _, s := range got {
				if !slices.Contains(w.names, s.Name) {
					t.Errorf("unexpected Schedule %q", s.Name)
				}
				if s.ProjectFound != w.found {
					t.Errorf("%s: project_found = %v, want %v", s.Name, s.ProjectFound, w.found)
				}
				if s.Switch {
					on++
				}
				if s.Switch == (s.NextRun == "") && s.ProjectFound {
					t.Errorf("%s: switch %v but next_run %q", s.Name, s.Switch, s.NextRun)
				}
			}
			if on != w.on {
				t.Errorf("%d on, want %d", on, w.on)
			}
		})
	}
}

func TestSchedulesScenario_PageShapesSeedLastRuns(t *testing.T) {
	t.Parallel()
	addr := "demo:weather-check"
	for _, w := range []struct {
		shape  string
		status []wire.ScheduleRunStatus
	}{
		{"page", []wire.ScheduleRunStatus{wire.ScheduleRunFinished, wire.ScheduleRunFinished, wire.ScheduleRunRanLate, wire.ScheduleRunSkipped}},
		{"paused", []wire.ScheduleRunStatus{wire.ScheduleRunPaused, wire.ScheduleRunFinished, wire.ScheduleRunFinished, wire.ScheduleRunRanLate, wire.ScheduleRunSkipped}},
	} {
		t.Run(w.shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "schedules", nil, "--shape", w.shape)
			runs := r.scheduleRuns(t, addr)
			if len(runs) != len(w.status) {
				t.Fatalf("%d runs, want %d: %+v", len(runs), len(w.status), runs)
			}
			for i, run := range runs {
				if run.Status != w.status[i] {
					t.Errorf("run %d status = %q, want %q", i, run.Status, w.status[i])
				}
				switch run.Status {
				case wire.ScheduleRunSkipped:
					if run.Reason != "late" || run.Address != "" {
						t.Errorf("skip row %+v", run)
					}
				case wire.ScheduleRunFinished, wire.ScheduleRunRanLate:
					if run.Result == "" || run.Address == "" {
						t.Errorf("finished row without a Result line or address: %+v", run)
					}
				case wire.ScheduleRunPaused:
					if run.Address == "" {
						t.Errorf("paused row without a ticket address: %+v", run)
					}
				}
			}
		})
	}
}
