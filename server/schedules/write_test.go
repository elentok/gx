package schedules_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func startWrite(t *testing.T) (*servertest.Harness, *rigs.SchedulesClock) {
	t.Helper()
	clock := rigs.NewSchedulesClock(at(8, 0))
	h, _ := rigs.StartSchedulesHarness(t, clock)
	return h, clock
}

func baseCreate() wire.ScheduleCreateRequest {
	return wire.ScheduleCreateRequest{Name: "Nightly", Prompt: "check the logs", Days: []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}, Times: []string{"09:00"}, Project: "proj"}
}

func create(t *testing.T, h *servertest.Harness, req wire.ScheduleCreateRequest) wire.ScheduleShowResult {
	t.Helper()
	res, err := h.Client.Schedules.Create(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func edit(t *testing.T, h *servertest.Harness, req wire.ScheduleEditRequest) wire.ScheduleShowResult {
	t.Helper()
	res, err := h.Client.Schedules.Edit(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func countSchedules(t *testing.T, h *servertest.Harness) int {
	t.Helper()
	items, err := schedrec.NewStore(h.TicketStore).List()
	if err != nil {
		t.Fatal(err)
	}
	return len(items)
}

func TestSchedulesWrite_CreateRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	zero := 0.0
	cases := []struct {
		reason string
		mod    func(*wire.ScheduleCreateRequest)
	}{
		{schedrec.ReasonEmptyName, func(r *wire.ScheduleCreateRequest) { r.Name = " " }},
		{schedrec.ReasonEmptyPrompt, func(r *wire.ScheduleCreateRequest) { r.Prompt = "" }},
		{schedrec.ReasonNoDays, func(r *wire.ScheduleCreateRequest) { r.Days = nil }},
		{schedrec.ReasonNoTimes, func(r *wire.ScheduleCreateRequest) { r.Times = nil }},
		{schedrec.ReasonInvalidTime, func(r *wire.ScheduleCreateRequest) { r.Times = []string{"25:00"} }},
		{schedrec.ReasonInvalidLateLimit, func(r *wire.ScheduleCreateRequest) { r.LateLimitHours = &zero }},
		{schedrec.ReasonInvalidHarness, func(r *wire.ScheduleCreateRequest) { r.Harness = "nope" }},
		{schedrec.ReasonInvalidProject, func(r *wire.ScheduleCreateRequest) { r.Project = "nope" }},
	}
	for _, c := range cases {
		req := baseCreate()
		c.mod(&req)
		if res := create(t, h, req); !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: res = %+v", c.reason, res)
		}
	}
	if n := countSchedules(t, h); n != 0 {
		t.Errorf("%d Schedules written by refused requests", n)
	}
}

func TestSchedulesWrite_CreateDefaultsAndDuplicateTimes(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	req := baseCreate()
	req.Project = ""
	req.Times = []string{"10:00", "09:00", "10:00"}
	res := create(t, h, req)
	if res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	s := res.Schedule
	if s.Project != "scratch" || s.Harness != "" || s.DefaultHarness == "" || s.LateLimitHours != 2 || !slices.Equal(s.Times, []string{"09:00", "10:00"}) || !s.Switch {
		t.Errorf("schedule = %+v", s)
	}
	if res.Prompt != "check the logs" {
		t.Errorf("prompt = %q", res.Prompt)
	}
}

func TestSchedulesWrite_SameSlugInSameProjectIsRefused(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	create(t, h, baseCreate())
	req := baseCreate()
	req.Name = "nightly"
	if res := create(t, h, req); !res.Refused || res.Reason != schedrec.ReasonSlugExists {
		t.Errorf("res = %+v", res)
	}
	if n := countSchedules(t, h); n != 1 {
		t.Errorf("%d Schedules, want 1", n)
	}
}

func TestSchedulesWrite_NewScheduleIsOnHereAndOffElsewhere(t *testing.T) {
	t.Parallel()
	h, clock := startWrite(t)
	create(t, h, baseCreate())
	second := rigs.StartOneOffsHarness(t, h.TicketStore, rigs.WithSchedulesClock(clock))
	res, err := second.Client.Schedules.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Schedules) != 1 || res.Schedules[0].Switch {
		t.Errorf("second machine = %+v", res.Schedules)
	}
	here, err := h.Client.Schedules.List(context.Background())
	if err != nil || len(here.Schedules) != 1 || !here.Schedules[0].Switch {
		t.Errorf("first machine = %+v err = %v", here, err)
	}
}

func TestSchedulesWrite_EditRenameKeepsSlugAndOtherFields(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	create(t, h, baseCreate())
	name, late := "Evening", 3.0
	res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", Name: &name, LateLimitHours: &late})
	if res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	if s := res.Schedule; s.Name != "Evening" || s.Slug != "nightly" || s.LateLimitHours != 3 || !slices.Equal(s.Times, []string{"09:00"}) || res.Prompt != "check the logs" {
		t.Errorf("schedule = %+v prompt = %q", s, res.Prompt)
	}
	shown, err := h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || shown.Refused || shown.Schedule.Name != "Evening" {
		t.Errorf("show = %+v err = %v", shown, err)
	}
}

func TestSchedulesWrite_EditRefusals(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	create(t, h, baseCreate())
	other := "scratch"
	if res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", NewProject: &other}); !res.Refused || res.Reason != wire.ScheduleReasonProjectReadOnly {
		t.Errorf("project change: %+v", res)
	}
	if res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nope"}); !res.Refused || res.Reason != wire.ScheduleReasonUnknown {
		t.Errorf("unknown: %+v", res)
	}
	empty := ""
	if res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", Name: &empty}); !res.Refused || res.Reason != schedrec.ReasonEmptyName {
		t.Errorf("empty name: %+v", res)
	}
	if res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", Days: []string{}}); !res.Refused || res.Reason != schedrec.ReasonNoDays {
		t.Errorf("no days: %+v", res)
	}
	zero := 0.0
	if res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", LateLimitHours: &zero}); !res.Refused || res.Reason != schedrec.ReasonInvalidLateLimit {
		t.Errorf("late limit: %+v", res)
	}
	shown, err := h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || shown.Schedule.Name != "Nightly" || shown.Schedule.LateLimitHours != 2 {
		t.Errorf("a refused edit changed the Schedule: %+v err = %v", shown, err)
	}
}

func TestSchedulesWrite_RenameKeepsOldRuns(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.clock.Set(at(9, 0).Add(time.Minute))
	r.waitRuns(t, 1)
	name := "Renamed"
	if res := edit(t, r.h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", Name: &name}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	shown, err := r.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || len(shown.LastRuns) != 1 {
		t.Errorf("last runs = %+v err = %v", shown.LastRuns, err)
	}
}

func TestSchedulesWrite_EditStartsNoRunForAnEarlierTime(t *testing.T) {
	t.Parallel()
	clock := rigs.NewSchedulesClock(at(8, 0))
	h, _ := rigs.StartOneOffHarness(t, rigs.WithSchedulesClock(clock), func(c *server.Config) { c.SchedulesInterval = time.Hour })
	create(t, h, baseCreate())
	// The control is not edited, so the startup check runs it: that is the
	// signal that the check happened.
	ctl := baseCreate()
	ctl.Name = "Control"
	create(t, h, ctl)
	clock.Set(at(9, 30))
	late := 4.0
	if res := edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", LateLimitHours: &late}); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	r := &rig{h: h, clock: clock}
	r.waitRuns(t, 1)
	settle()
	got := r.waitRuns(t, 1)
	if !strings.Contains(got[0].Unique, "proj/control") {
		t.Errorf("run = %q, want the control's", got[0].Unique)
	}
}

func TestSchedulesWrite_AWriteDoesPublishSchedulesChanged(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	events := rigs.EventStream(t, h)
	create(t, h, baseCreate())
	name := "Evening"
	edit(t, h, wire.ScheduleEditRequest{Project: "proj", Slug: "nightly", Name: &name})
	for got := 0; got < 2; {
		select {
		case ev := <-events:
			if ev.Type == wire.EventSchedulesChanged && ev.Address == "proj:nightly" {
				got++
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("saw %d schedules-changed events, want 2", got)
		}
	}
}
