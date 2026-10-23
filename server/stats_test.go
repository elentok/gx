package server_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// statsNoon is local noon n days before today: a stamp that stays on its day in any zone.
func statsNoon(n int) time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()-n, 12, 0, 0, 0, time.Local)
}

func statsStamp(n int) string { return statsNoon(n).UTC().Format(time.RFC3339) }

func statsDate(n int) string { return statsNoon(n).Format("2006-01-02") }

func statsAt(t *testing.T, s server.Stats, series []int, n int) int {
	t.Helper()
	i := slices.Index(s.Days, statsDate(n))
	if i < 0 {
		t.Fatalf("day %s not in %v", statsDate(n), s.Days)
	}
	return series[i]
}

func statsFixture(t *testing.T) (get func(query string) server.Stats) {
	t.Helper()
	// A zone away from UTC, so a UTC-day bucket would disagree with the local one.
	old := time.Local
	time.Local = time.FixedZone("test", 5*3600)
	t.Cleanup(func() { time.Local = old })

	loop := "---\nstatus: open\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeProjectEpic(t, store, "proj", "loop", loop, map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: "+statsStamp(1)+"\n", "A"),
			"02-b.md": issueMD("02", "done", "done_at: "+statsStamp(1)+"\n", "B"),
			"03-c.md": issueMD("03", "done", "", "C"),
			"04-d.md": issueMD("04", "open", "", "D"),
			"05-e.md": issueMD("05", "done", "done_at: "+statsStamp(100)+"\n", "E"),
		})
		writeProjectEpic(t, store, "proj", "design", "---\nkind: map\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: "+statsStamp(1)+"\n", "A"),
		})
		// 01:00 local on day 3 is still day 4 in UTC.
		local := statsNoon(3).Add(-11 * time.Hour).UTC().Format(time.RFC3339)
		writeProjectEpic(t, store, "proj", "zone", loop, map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: "+local+"\n", "A"),
		})
		writeProjectEpic(t, store, "proj", "merged", "---\nstatus: done\nmerged_at: "+statsStamp(2)+"\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: "+statsStamp(2)+"\n", "A"),
		})
		writeProjectEpic(t, store, "proj", ".archive/old", "---\nstatus: done\nmerged_at: "+statsStamp(2)+"\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: "+statsStamp(2)+"\n", "A"),
		})
		writeProjectEpic(t, store, "other", "loop", loop, map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: "+statsStamp(1)+"\n", "A"),
		})
	})
	return func(query string) server.Stats {
		t.Helper()
		code, body := webDo(t, h, http.MethodGet, base+"/v1/stats"+query)
		if code != http.StatusOK {
			t.Fatalf("GET /v1/stats%s = %d: %s", query, code, body)
		}
		var s server.Stats
		if err := json.Unmarshal([]byte(body), &s); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
		return s
	}
}

func TestStats_TicketsMergedFollowTheCountingRules(t *testing.T) {
	get := statsFixture(t)
	s := get("")
	// day 1: loop 01+02 and other/loop 01; the map ticket, the unstamped done
	// ticket and the open ticket are left out.
	if got := statsAt(t, s, s.Throughput.TicketsMerged, 1); got != 3 {
		t.Errorf("tickets merged on day 1 = %d, want 3", got)
	}
	// the local-time bucket, not the UTC one.
	if got := statsAt(t, s, s.Throughput.TicketsMerged, 3); got != 1 {
		t.Errorf("tickets merged on local day 3 = %d, want 1", got)
	}
	// loop/05 is outside the default 30 days; merged and archived epics' tickets count.
	if got := statsAt(t, s, s.Throughput.TicketsMerged, 2); got != 2 {
		t.Errorf("tickets merged on day 2 = %d, want 2 (live and archived epic)", got)
	}
	if len(s.Days) != 30 || s.Days[len(s.Days)-1] != statsDate(0) {
		t.Errorf("days = %v, want 30 ending today %s", s.Days, statsDate(0))
	}
}

func TestStats_EpicsMergedIncludeArchives(t *testing.T) {
	s := statsFixture(t)("")
	if got := statsAt(t, s, s.Throughput.EpicsMerged, 2); got != 2 {
		t.Errorf("epics merged on day 2 = %d, want 2 (live and archived)", got)
	}
	if got := statsAt(t, s, s.Throughput.EpicsMerged, 1); got != 0 {
		t.Errorf("epics merged on day 1 = %d, want 0", got)
	}
}

func TestStats_ProjectFilter(t *testing.T) {
	get := statsFixture(t)
	s := get("?project=other")
	if got := statsAt(t, s, s.Throughput.TicketsMerged, 1); got != 1 {
		t.Errorf("other: tickets merged on day 1 = %d, want 1", got)
	}
	if got := statsAt(t, s, s.Throughput.EpicsMerged, 2); got != 0 {
		t.Errorf("other: epics merged on day 2 = %d, want 0", got)
	}
}

func TestStats_Ranges(t *testing.T) {
	get := statsFixture(t)
	for query, want := range map[string]int{"": 30, "?range=7": 7, "?range=30": 30, "?range=90": 90} {
		s := get(query)
		if len(s.Days) != want || len(s.Throughput.TicketsMerged) != want {
			t.Errorf("%q: %d days, %d points, want %d", query, len(s.Days), len(s.Throughput.TicketsMerged), want)
		}
	}
}

func TestStats_BadRangeIs400(t *testing.T) {
	h, base := startWeb(t)
	if code, _ := webDo(t, h, http.MethodGet, base+"/v1/stats?range=14"); code != http.StatusBadRequest {
		t.Errorf("range=14 = %d, want 400", code)
	}
}

func TestStats_NeedsTheWebCookie(t *testing.T) {
	_, base := startWeb(t)
	if code, _ := webDoCookie(t, http.MethodGet, base+"/v1/stats", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/stats without cookie = %d, want 401", code)
	}
}
