package schedules_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire/ops"
)

func TestSchedulesRead_ListShowsDaysTimesAndNextRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00", "17:30")
	r.enable(t)
	res, err := r.h.Client.Schedules.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Refused || len(res.Schedules) != 1 {
		t.Fatalf("list = %+v", res)
	}
	got := res.Schedules[0]
	if !slices.Equal(got.Days, []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}) || !slices.Equal(got.Times, []string{"09:00", "17:30"}) {
		t.Errorf("days/times = %v %v", got.Days, got.Times)
	}
	want := at(9, 0)
	if next, _ := time.Parse(time.RFC3339, got.NextRun); !next.Equal(want) {
		t.Errorf("next run = %q, want %v", got.NextRun, want)
	}
	if !got.Switch || !got.ProjectFound || got.DefaultHarness != "claude" || got.Name != "Nightly" || got.Slug != "nightly" || got.Project != "proj" {
		t.Errorf("item = %+v", got)
	}
	if !slices.Contains(res.Projects, "proj") {
		t.Errorf("projects = %v, want proj", res.Projects)
	}
}

func TestSchedulesRead_SwitchOffHasNoNextRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	res, err := r.h.Client.Schedules.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Schedules[0]; got.Switch || got.NextRun != "" {
		t.Errorf("switch-off item = %+v", got)
	}
}

func TestSchedulesRead_UnregisteredProjectHasNoNextRun(t *testing.T) {
	t.Parallel()
	m := startMissing(t)
	res, err := m.h.Client.Schedules.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Schedules[0]; got.ProjectFound || got.NextRun != "" || !got.Switch {
		t.Errorf("item = %+v", got)
	}
}

func TestSchedulesRead_ShowReturnsEveryFieldAndTheLastRuns(t *testing.T) {
	t.Parallel()
	r := start(t, 3, "09:00")
	r.enable(t)
	if err := r.h.Server.Schedules().AppendSkip(r.key, schedules.Skip{SetTime: at(7, 0), Reason: schedules.ReasonLate, LoggedAt: at(8, 0)}); err != nil {
		t.Fatal(err)
	}
	res, err := r.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil {
		t.Fatal(err)
	}
	if res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	s := res.Schedule
	if res.Prompt != "check the logs" || s.LateLimitHours != 3 || s.Harness != "" || s.DefaultHarness != "claude" || s.UpdatedAt == "" || s.NextRun == "" {
		t.Errorf("show = %+v", res)
	}
	if len(res.LastRuns) != 1 || res.LastRuns[0].Status != ops.ScheduleRunSkipped || res.LastRuns[0].Reason != schedules.ReasonLate {
		t.Errorf("last runs = %+v", res.LastRuns)
	}
}

func TestSchedulesRead_UnknownScheduleIsARefusalResult(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	res, err := r.h.Client.Schedules.Show(context.Background(), "proj", "nope")
	if err != nil {
		t.Fatalf("an unknown schedule must not be an error: %v", err)
	}
	if !res.Refused || res.Reason != ops.ScheduleReasonUnknown {
		t.Errorf("res = %+v", res)
	}
	res, err = r.h.Client.Schedules.Show(context.Background(), "nope", "nightly")
	if err != nil || !res.Refused || res.Reason != ops.ScheduleReasonInvalidProject {
		t.Errorf("unknown project: res = %+v err = %v", res, err)
	}
}

func TestSchedulesRead_SwitchIsPerMachine(t *testing.T) {
	t.Parallel()
	clock := rigs.NewSchedulesClock(at(8, 0))
	first := startOn(t, clock, 2, "09:00")
	first.enable(t)
	second := rigs.StartOneOffsHarness(t, first.h.TicketStore, rigs.WithSchedulesClock(clock))
	res, err := second.Client.Schedules.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Schedules) != 1 || res.Schedules[0].Switch || res.Schedules[0].NextRun != "" {
		t.Errorf("second machine = %+v", res.Schedules)
	}
}
