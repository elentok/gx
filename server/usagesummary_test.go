package server_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func usageSummary(t *testing.T, h *servertest.Harness) server.UsageSummary {
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

	if u := usageSummary(t, h); u.Mode != server.UsageModeSpending || u.Limits.FiveHour != nil || u.Limits.SevenDay != nil {
		t.Fatalf("before any report = %+v, want spending with no windows", u)
	}

	sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	u := usageSummary(t, h)
	if u.Mode != server.UsageModeLimits || u.Limits.FiveHour == nil || u.Limits.FiveHour.ReceivedAt <= 0 || u.Limits.SevenDay != nil {
		t.Fatalf("after a report = %+v, want limits with five_hour and its received-at", u)
	}

	sendLimits(t, h, server.LimitsReport{})
	if u := usageSummary(t, h); u.Mode != server.UsageModeLimits {
		t.Errorf("after an empty report mode = %q, want limits", u.Mode)
	}

	h.Restart(t)
	if u := usageSummary(t, h); u.Mode != server.UsageModeLimits || u.Limits.FiveHour == nil {
		t.Errorf("after restart = %+v, want limits with five_hour", u)
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
