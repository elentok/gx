package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func usageSummary(t *testing.T, h *servertest.Harness) wire.UsageSummary {
	t.Helper()
	u, err := h.Client.UsageSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestUsageSummary_ModeFollowsStoredWindows(t *testing.T) {
	dir := t.TempDir()
	h := servertest.StartWithStore(t, dir, servertest.WithTestHolder())

	if u := usageSummary(t, h); u.Mode != wire.UsageModeSpending || u.Limits.FiveHour != nil || u.Limits.SevenDay != nil {
		t.Fatalf("before any report = %+v, want spending with no windows", u)
	}

	sendLimits(t, h, wire.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	u := usageSummary(t, h)
	if u.Mode != wire.UsageModeLimits || u.Limits.FiveHour == nil || u.Limits.FiveHour.ReceivedAt <= 0 || u.Limits.SevenDay != nil {
		t.Fatalf("after a report = %+v, want limits with five_hour and its received-at", u)
	}

	sendLimits(t, h, wire.LimitsReport{})
	if u := usageSummary(t, h); u.Mode != wire.UsageModeLimits {
		t.Errorf("after an empty report mode = %q, want limits", u.Mode)
	}

	h.Restart(t)
	if u := usageSummary(t, h); u.Mode != wire.UsageModeLimits || u.Limits.FiveHour == nil {
		t.Errorf("after restart = %+v, want limits with five_hour", u)
	}
}

// seedLimitHistory writes the history file and restarts the server on it. Each id
// gets one sample per local day for the five days before today, rising 10
// points a day to 50, and its latest reading is the last sample's value read
// latestAge ago. It returns the reset time of the seeded windows: a later report
// must reuse it, since a different one reads as a reset and burns today.
func seedLimitHistory(t *testing.T, h *servertest.Harness, ids []string, latestAge time.Duration) (resets int64) {
	t.Helper()
	now := time.Now()
	resets = now.Add(72 * time.Hour).Unix()
	state := struct {
		Samples map[string][]wire.LimitSample `json:"samples"`
		Latest  map[string]wire.LimitSample   `json:"latest"`
	}{map[string][]wire.LimitSample{}, map[string]wire.LimitSample{}}
	for _, id := range ids {
		for d := 5; d >= 1; d-- {
			day := time.Date(now.Year(), now.Month(), now.Day()-d, 1, 0, 0, 0, time.Local)
			state.Samples[id] = append(state.Samples[id], wire.LimitSample{
				LimitID: id, UsedPercentage: float64(10 * (6 - d)), ResetsAt: resets, ReceivedAt: day.Unix(),
			})
		}
		state.Latest[id] = wire.LimitSample{LimitID: id, UsedPercentage: 50, ResetsAt: resets, ReceivedAt: now.Add(-latestAge).Unix()}
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.StateDir, "limit-history.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	return resets
}

func TestUsageSummary_WeeklyCarriesChipAndForecast(t *testing.T) {
	h := limitsHarness(t)
	resets := seedLimitHistory(t, h, []string{wire.LimitIDClaudeWeekly}, 0)
	sendLimits(t, h, wire.LimitsReport{
		FiveHour: limitWin(20, 4_000_000_000),
		SevenDay: limitWin(50, resets),
	})

	u := usageSummary(t, h)
	five, week := u.Limits.FiveHour, u.Limits.SevenDay
	if five == nil || five.Chip != nil || five.Forecast != nil {
		t.Errorf("5h = %+v, want a plain window", five)
	}
	if week == nil || week.Chip == nil || week.Chip.Yesterday != 10 || week.Chip.Today != 0 || week.Chip.Direction != wire.LimitChipDown {
		t.Fatalf("weekly chip = %+v, want yesterday 10, today 0, down", week)
	}
	f := week.Forecast
	if f == nil || f.Status != wire.LimitForecastProjected || f.Pace != 10 || f.AtReset != 80 || f.RunsOutAt != 0 {
		t.Errorf("weekly forecast = %+v, want projected, pace 10, 80 at reset", f)
	}
	if u.Codex != nil {
		t.Errorf("codex = %+v, want absent for Claude-only history", u.Codex)
	}
}

func TestUsageSummary_NoChipWithoutAFullDay(t *testing.T) {
	h := limitsHarness(t)
	sendLimits(t, h, wire.LimitsReport{SevenDay: limitWin(50, time.Now().Add(72*time.Hour).Unix())})
	week := usageSummary(t, h).Limits.SevenDay
	if week == nil || week.Chip != nil || week.Forecast == nil || week.Forecast.Status != wire.LimitForecastNeedsData {
		t.Errorf("weekly = %+v, want no chip and a needs-data forecast", week)
	}
}

func TestUsageSummary_CodexRowsAreStaleByLatestReceivedAt(t *testing.T) {
	h := limitsHarness(t)
	seedLimitHistory(t, h, []string{wire.LimitIDCodex5h, wire.LimitIDCodexWeekly}, 3*time.Hour)

	u := usageSummary(t, h)
	if u.Codex == nil || u.Codex.FiveHour == nil || u.Codex.SevenDay == nil {
		t.Fatalf("codex = %+v, want both windows", u.Codex)
	}
	if age := time.Now().Unix() - u.Codex.SevenDay.ReceivedAt; age < 3*3600-5 || age > 3*3600+60 {
		t.Errorf("codex weekly age = %ds, want about 3h", age)
	}
	if u.Codex.FiveHour.Forecast != nil || u.Codex.SevenDay.Forecast == nil || u.Codex.SevenDay.Chip == nil {
		t.Errorf("codex = %+v, want chip and forecast on weekly only", u.Codex)
	}
	if u.Limits.FiveHour != nil || u.Limits.SevenDay != nil {
		t.Errorf("claude limits = %+v, want none", u.Limits)
	}
}

func TestUsageSummary_HasNoBudgetLimitFields(t *testing.T) {
	h := limitsHarness(t)
	raw, err := json.Marshal(usageSummary(t, h))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"mode", "limits", "spend"} {
		if _, ok := top[k]; !ok {
			t.Errorf("missing key %q in %s", k, raw)
		}
	}
	if len(top) != 3 {
		t.Errorf("top-level keys = %v, want mode, limits, spend only", top)
	}
	spend, _ := top["spend"].(map[string]any)
	for k := range spend {
		switch k {
		case "today", "since_sunday", "since_sunday_short", "since_first", "since_first_short", "first_day":
		default:
			t.Errorf("unexpected spend key %q", k)
		}
	}
}
