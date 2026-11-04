package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// limitBurnStats seeds five days of history for the Claude weekly limit (10
// points a day) and returns a getter for /v1/stats.
func limitBurnStats(t *testing.T) func(query string) wire.Stats {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	seedLimitHistory(t, h, []string{server.LimitIDClaudeWeekly}, 0)
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return func(query string) wire.Stats {
		t.Helper()
		code, body := webDo(t, h, http.MethodGet, hs.WebURL+"/v1/stats"+query)
		if code != http.StatusOK {
			t.Fatalf("GET /v1/stats%s = %d: %s", query, code, body)
		}
		var s wire.Stats
		if err := json.Unmarshal([]byte(body), &s); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
		return s
	}
}

func TestStatsLimitBurn_WeeklyCarriesSeriesHeadlineAndForecast(t *testing.T) {
	get := limitBurnStats(t)
	for _, n := range []int{7, 30, 90} {
		s := get("?range=" + strconv.Itoa(n))
		lb := s.LimitBurn
		for name, series := range map[string]wire.StatsLimitBurn{
			"claude5h": lb.Claude5h, "claudeWeekly": lb.ClaudeWeekly, "codex5h": lb.Codex5h, "codexWeekly": lb.CodexWeekly,
		} {
			if len(series.Days) != n {
				t.Errorf("range %d %s: %d points, want %d", n, name, len(series.Days), n)
			}
		}
		w := lb.ClaudeWeekly
		// Samples sit on the five days before today: the first adds nothing and
		// the next four add 10 each.
		if w.Sum != 40 {
			t.Errorf("range %d: sum = %v, want 40", n, w.Sum)
		}
		if n == 7 {
			// 7 days: today + 5 sampled days = 6 with data, 1 before.
			if w.StartDay == "" || w.Average != 40.0/6 {
				t.Errorf("range 7: start %q average %v, want a start day and 40/6", w.StartDay, w.Average)
			}
		}
		if n > 7 && w.StartDay == "" {
			t.Errorf("range %d: no start day, want one", n)
		}
		if w.Days[0].NoData != (n > 6) {
			t.Errorf("range %d: first day no_data = %v", n, w.Days[0].NoData)
		}
		if w.Forecast == nil || w.Forecast.Status != wire.LimitForecastProjected || w.Forecast.Pace != 10 {
			t.Errorf("range %d: forecast = %+v, want projected with pace 10", n, w.Forecast)
		}
		if lb.Claude5h.Forecast != nil || lb.Codex5h.Forecast != nil {
			t.Errorf("range %d: 5-hour series carry a forecast", n)
		}
	}
}

func TestStatsLimitBurn_CodexWithoutHistoryIsAllNoData(t *testing.T) {
	get := limitBurnStats(t)
	c := get("?range=7").LimitBurn.CodexWeekly
	for i, d := range c.Days {
		if !d.NoData || d.Burn != 0 {
			t.Errorf("codex day %d = %+v, want no data", i, d)
		}
	}
	if c.Sum != 0 || c.Average != 0 || c.StartDay != "" || c.Forecast != nil {
		t.Errorf("codex = %+v, want an empty headline and no forecast", c)
	}
}

func TestStatsLimitBurn_ProjectFilterLeavesItAlone(t *testing.T) {
	get := limitBurnStats(t)
	for _, n := range []string{"7", "30", "90"} {
		all, one := get("?range="+n).LimitBurn, get("?range="+n+"&project=other").LimitBurn
		a, _ := json.Marshal(all)
		b, _ := json.Marshal(one)
		if string(a) != string(b) {
			t.Errorf("range %s: project filter changed the burn:\n%s\n%s", n, a, b)
		}
	}
}
