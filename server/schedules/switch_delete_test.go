package schedules_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func (r *rig) switchTo(t *testing.T, on bool) wire.ScheduleShowResult {
	t.Helper()
	res, err := r.h.Client.Schedules.Switch(context.Background(), wire.ScheduleSwitchRequest{Project: r.key.Project, Slug: r.key.Slug, On: on})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (r *rig) deleteVia(t *testing.T) wire.ScheduleDeleteResult {
	t.Helper()
	res, err := r.h.Client.Schedules.Delete(context.Background(), wire.ScheduleDeleteRequest{Project: r.key.Project, Slug: r.key.Slug})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSchedulesSwitch_OffThenOnMovesSwitchInShowAndList(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	if res := r.switchTo(t, false); res.Refused || res.Schedule.Switch || res.Schedule.NextRun != "" {
		t.Fatalf("off = %+v", res)
	}
	list, err := r.h.Client.Schedules.List(context.Background())
	if err != nil || len(list.Schedules) != 1 || list.Schedules[0].Switch {
		t.Fatalf("list after off = %+v err = %v", list, err)
	}
	if res := r.switchTo(t, true); res.Refused || !res.Schedule.Switch || res.Schedule.NextRun == "" {
		t.Fatalf("on = %+v", res)
	}
	shown, err := r.h.Client.Schedules.Show(context.Background(), r.key.Project, r.key.Slug)
	if err != nil || !shown.Schedule.Switch {
		t.Errorf("show after on = %+v err = %v", shown, err)
	}
}

func TestSchedulesSwitch_OnNeverRunsTimeBeforeIt(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.switchTo(t, false)
	r.clock.Set(at(9, 30))
	settle()
	r.switchTo(t, true)
	settle()
	if got := r.runs(t); len(got) != 0 {
		t.Errorf("runs = %d, want none for 09:00 passed while off", len(got))
	}
}

func TestSchedulesSwitch_OffLeavesAQueuedRunAlone(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.writeRun(t, "90", r.h.Server.Schedules().MachineID(), servertest.TicketOpts{})
	r.switchTo(t, false)
	settle()
	if s := r.statusOf(t, "90"); s != "open" {
		t.Errorf("queued run status = %q, want open", s)
	}
}

func TestSchedulesDelete_CancelsQueuedAtOnceAndKeepsLiveAndPaused(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	id := r.h.Server.Schedules().MachineID()
	r.writeRun(t, "90", id, servertest.TicketOpts{})
	r.writeRun(t, "91", id, servertest.TicketOpts{Status: "claimed", ClaimedAt: at(8, 30).Format(time.RFC3339)})
	r.writeRun(t, "92", id, servertest.TicketOpts{PauseReason: "question"})

	if res := r.deleteVia(t); res.Refused {
		t.Fatalf("refused: %+v", res)
	}

	// No waiting: the delete itself ran the check.
	if r.hasState(r.h) {
		t.Error("state kept after the delete")
	}
	if got := r.cancelled(t); len(got) != 1 {
		t.Errorf("cancelled = %v, want one run", got)
	}
	if s := r.statusOf(t, "91"); s != "claimed" {
		t.Errorf("started run status = %q", s)
	}
	if s := r.statusOf(t, "92"); s != "paused" {
		t.Errorf("paused run status = %q", s)
	}
	if countSchedules(t, r.h) != 0 {
		t.Error("the Schedule file is still there")
	}
}

func TestSchedulesSwitchAndDelete_UnknownScheduleIsRefused(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	sw, err := h.Client.Schedules.Switch(context.Background(), wire.ScheduleSwitchRequest{Project: "proj", Slug: "nope", On: true})
	if err != nil || !sw.Refused || sw.Reason != wire.ScheduleReasonUnknown {
		t.Errorf("switch = %+v err = %v", sw, err)
	}
	del, err := h.Client.Schedules.Delete(context.Background(), wire.ScheduleDeleteRequest{Project: "proj", Slug: "nope"})
	if err != nil || !del.Refused || del.Reason != wire.ScheduleReasonUnknown {
		t.Errorf("delete = %+v err = %v", del, err)
	}
}

func TestSchedulesSwitchAndDelete_PublishSchedulesChanged(t *testing.T) {
	t.Parallel()
	h, _ := startWrite(t)
	create(t, h, baseCreate())
	events := rigs.EventStream(t, h)
	ctx := context.Background()
	if _, err := h.Client.Schedules.Switch(ctx, wire.ScheduleSwitchRequest{Project: "proj", Slug: "nightly", On: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Client.Schedules.Delete(ctx, wire.ScheduleDeleteRequest{Project: "proj", Slug: "nightly"}); err != nil {
		t.Fatal(err)
	}
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
