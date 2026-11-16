package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const unlockSentence = "I'm Dana and I'm addicted to AI"

func TestRoutes_FreshServerShowsFeatureOff(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t, servertest.WithRestClock(servertest.NewRestClock(start())))
	st, err := h.Client.Rest.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Period != nil || st.Warning != nil || st.WorkCount != 0 {
		t.Errorf("status = %+v, want off with nothing running", st)
	}
	if st.Zone != "test" || st.ZoneOffsetSeconds != 2*60*60 {
		t.Errorf("zone = %q %d, want test 7200", st.Zone, st.ZoneOffsetSeconds)
	}
	if !st.Now.Equal(start()) {
		t.Errorf("now = %v, want the server clock %v", st.Now, start())
	}
}

func TestRoutes_ActiveCountsOncePerMinute(t *testing.T) {
	t.Parallel()
	clock := servertest.NewRestClock(start())
	h := servertest.Start(t, servertest.WithRestClock(clock), servertest.WithMentalHealth(breakCfg()))
	ctx := context.Background()
	active := func() int {
		t.Helper()
		res, err := h.Client.Rest.Active(ctx)
		if err != nil || res.Refused {
			t.Fatalf("active = %+v, %v", res, err)
		}
		return res.Status.WorkCount
	}
	if a, b := active(), active(); a != 1 || b != 1 {
		t.Errorf("two heartbeats in one minute = %d, %d; want 1, 1", a, b)
	}
	clock.Advance(time.Minute)
	if got := active(); got != 2 {
		t.Errorf("next minute = %d, want 2", got)
	}
	st, err := h.Client.Rest.Status(ctx)
	if err != nil || st.WorkCount != 2 || !st.Enabled || st.Rule.WorkMinutes != 3 || st.Rule.BreakMinutes != 2 {
		t.Errorf("status = %+v, %v", st, err)
	}
}

func TestRoutes_UnlockRefusesWrongSentenceAndEndsBreak(t *testing.T) {
	t.Parallel()
	clock := servertest.NewRestClock(start())
	h := servertest.Start(t, servertest.WithRestClock(clock), servertest.WithMentalHealth(breakCfg()))
	ctx := context.Background()
	res, err := h.Client.Rest.Unlock(ctx, "hello")
	if err != nil || !res.Refused || res.Reason != wire.ReasonRestNoPeriod {
		t.Errorf("unlock with no period = %+v, %v; want %s", res, err, wire.ReasonRestNoPeriod)
	}
	for range 3 {
		if _, err := h.Client.Rest.Active(ctx); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute)
	}
	st, _ := h.Client.Rest.Status(ctx)
	if st.Period == nil || st.Period.Kind != wire.RestKindBreak || st.Period.Message == "" {
		t.Fatalf("status = %+v, want a running Break with its message", st)
	}
	res, err = h.Client.Rest.Unlock(ctx, "hello")
	if err != nil || !res.Refused || res.Reason != wire.ReasonRestSentenceMismatch || res.Message == "" || res.Status.Period == nil {
		t.Errorf("wrong sentence = %+v, %v; want a refusal with the Break still running", res, err)
	}
	res, err = h.Client.Rest.Unlock(ctx, unlockSentence)
	if err != nil || res.Refused || res.Status.Period != nil {
		t.Errorf("right sentence = %+v, %v; want no Rest period", res, err)
	}
}

func TestRoutes_WebServesWritesAndSnapshotShowsRest(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	for _, tc := range []struct{ path, body string }{
		{"/v1/rest/active", `{}`},
		{"/v1/rest/unlock", `{"sentence":"hello"}`},
	} {
		code, out := servertest.WebPostJSON(t, base+tc.path, tc.body, cookie, "", servertest.JSONPost(base))
		if code != http.StatusOK {
			t.Fatalf("web %s = %d %s", tc.path, code, out)
		}
		var res wire.RestResult
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil || snap.Rest == nil || snap.Rest.Now.IsZero() {
		t.Errorf("snapshot rest = %+v, %v; want the server time", snap.Rest, err)
	}
}

func TestRoutes_RestartKeepsBreakThroughRoutes(t *testing.T) {
	t.Parallel()
	clock := servertest.NewRestClock(start())
	h := servertest.Start(t, servertest.WithRestClock(clock), servertest.WithMentalHealth(breakCfg()))
	ctx := context.Background()
	for range 3 {
		if _, err := h.Client.Rest.Active(ctx); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute)
	}
	before, _ := h.Client.Rest.Status(ctx)
	if before.Period == nil {
		t.Fatal("no Break is running")
	}
	h.Restart(t)
	after, err := h.Client.Rest.Status(ctx)
	if err != nil || after.Period == nil || after.Period.Message != before.Period.Message || !after.Period.End.Equal(before.Period.End) {
		t.Errorf("after restart = %+v, %v; want %+v", after.Period, err, before.Period)
	}
}
