package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func limitsHarness(t *testing.T) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
}

func sendLimits(t *testing.T, h *servertest.Harness, r server.LimitsReport) server.LimitsReportResult {
	t.Helper()
	res, err := h.Client.ReportLimits(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func limitWin(pct float64, resets int64) *server.LimitWindow {
	return &server.LimitWindow{UsedPercentage: pct, ResetsAt: resets}
}

func TestLimitsReport_OneWindowStoredAsOne(t *testing.T) {
	h := limitsHarness(t)
	res := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(34, 4_000_000_000)})
	if res.Refused || res.FiveHour == nil || res.SevenDay != nil {
		t.Fatalf("res = %+v, want only five_hour", res)
	}
	if res.FiveHour.UsedPercentage != 34 || res.FiveHour.ReceivedAt <= 0 {
		t.Errorf("five_hour = %+v, want 34%% with a server received-at", *res.FiveHour)
	}
}

func TestLimitsReport_TwoWindows(t *testing.T) {
	h := limitsHarness(t)
	res := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(10, 4_000_000_000), SevenDay: limitWin(20, 4_000_100_000)})
	if res.FiveHour == nil || res.SevenDay == nil || res.SevenDay.UsedPercentage != 20 {
		t.Fatalf("res = %+v, want both windows", res)
	}
}

func TestLimitsReport_OutOfOrderLeavesWindowUnchanged(t *testing.T) {
	h := limitsHarness(t)
	first := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(50, 4_000_000_000)})
	lower := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(40, 4_000_000_000)})
	earlier := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(60, 3_999_999_000)})
	for name, got := range map[string]server.LimitsReportResult{"lower percentage": lower, "earlier reset": earlier} {
		if *got.FiveHour != *first.FiveHour {
			t.Errorf("%s: stored %+v, want unchanged %+v", name, *got.FiveHour, *first.FiveHour)
		}
	}
}

func TestLimitsReport_NoWindowsChangesNothing(t *testing.T) {
	h := limitsHarness(t)
	first := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(50, 4_000_000_000)})
	got := sendLimits(t, h, server.LimitsReport{})
	if got.Refused || got.FiveHour == nil || *got.FiveHour != *first.FiveHour {
		t.Errorf("read-back = %+v, want %+v", got, first)
	}
}

func TestLimitsReport_RestartKeepsWindows(t *testing.T) {
	h := limitsHarness(t)
	first := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(50, 4_000_000_000), SevenDay: limitWin(5, 4_000_100_000)})
	h.Restart(t)
	got := sendLimits(t, h, server.LimitsReport{})
	if got.FiveHour == nil || got.SevenDay == nil || *got.FiveHour != *first.FiveHour || *got.SevenDay != *first.SevenDay {
		t.Errorf("after restart = %+v, want %+v", got, first)
	}
}

func TestLimitsReport_ReportAfterStartIsStored(t *testing.T) {
	h := limitsHarness(t)
	if got := sendLimits(t, h, server.LimitsReport{}); got.FiveHour != nil || got.SevenDay != nil {
		t.Fatalf("fresh server stores %+v, want nothing", got)
	}
	sendLimits(t, h, server.LimitsReport{SevenDay: limitWin(7, 4_000_000_000)})
	if got := sendLimits(t, h, server.LimitsReport{}); got.SevenDay == nil || got.SevenDay.UsedPercentage != 7 {
		t.Errorf("stored = %+v, want the seven_day report", got)
	}
}

func TestLimitsReport_BothInvalidNamesFiveHour(t *testing.T) {
	h := limitsHarness(t)
	for range 20 {
		res := sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(101, 4_000_000_000), SevenDay: limitWin(-1, 4_000_000_000)})
		if !res.Refused || !strings.HasPrefix(res.Message, "five_hour") {
			t.Fatalf("res = %+v, want a five_hour refusal", res)
		}
	}
}

func TestLimitsReport_InvalidIsRefused(t *testing.T) {
	h := limitsHarness(t)
	for name, r := range map[string]server.LimitsReport{
		"percentage over 100": {FiveHour: limitWin(101, 4_000_000_000)},
		"negative percentage": {SevenDay: limitWin(-1, 4_000_000_000)},
		"resets_at zero":      {FiveHour: limitWin(10, 0)},
	} {
		res := sendLimits(t, h, r)
		if !res.Refused || res.Reason != server.ReasonInvalidReport {
			t.Errorf("%s: res = %+v, want invalid-report refusal", name, res)
		}
	}
	if got := sendLimits(t, h, server.LimitsReport{}); got.FiveHour != nil || got.SevenDay != nil {
		t.Errorf("a refused report stored %+v", got)
	}
}
