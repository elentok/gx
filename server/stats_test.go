package server_test

import (
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"strconv"
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
	loop := "---\nstatus: open\n---\n"
	return statsStore(t, func(store string) {
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
}

// statsStore serves the store built by setup, in a zone away from UTC so a
// UTC-day bucket would disagree with the local one.
func statsStore(t *testing.T, setup func(store string)) (get func(query string) server.Stats) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("test", 5*3600)
	t.Cleanup(func() { time.Local = old })
	h, base := startEpicDetail(t, setup)
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

func TestStats_LinesOfCodeLandOnTheMergeDay(t *testing.T) {
	s := statsStore(t, func(store string) {
		writeProjectEpic(t, store, "proj", "a", "---\nstatus: done\nmerged_at: "+statsStamp(2)+"\nlines_added: 120\nlines_removed: 30\n---\n", nil)
		writeProjectEpic(t, store, "proj", "b", "---\nstatus: done\nmerged_at: "+statsStamp(2)+"\nlines_added: 5\nlines_removed: 1\n---\n", nil)
		writeProjectEpic(t, store, "proj", "unmerged", "---\nstatus: open\nlines_added: 999\nlines_removed: 999\n---\n", nil)
	})("")
	if got := statsAt(t, s, s.Throughput.LinesAdded, 2); got != 125 {
		t.Errorf("lines added on day 2 = %d, want 125", got)
	}
	if got := statsAt(t, s, s.Throughput.LinesRemoved, 2); got != 31 {
		t.Errorf("lines removed on day 2 = %d, want 31", got)
	}
	if got := statsAt(t, s, s.Throughput.LinesAdded, 1); got != 0 {
		t.Errorf("lines added on day 1 = %d, want 0", got)
	}
}

func TestStats_FollowUpsCountByOriginalAddress(t *testing.T) {
	s := statsStore(t, func(store string) {
		created := func(n int) string { return "created_at: " + statsStamp(n) + "\n" }
		writeProjectEpic(t, store, "proj", "follow-ups", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "open", created(1), "A"),
			"02-b.md": issueMD("02", "done", created(1)+"done_at: "+statsStamp(0)+"\n", "B"),
			"03-c.md": issueMD("03", "cancelled", created(2)+"cancelled_at: "+statsStamp(1)+"\n", "C"),
		})
		writeProjectEpic(t, store, "proj", "follow-ups-archived", "---\nstatus: open\n---\n", map[string]string{
			"10-j.md": issueMD("10", "cancelled", "original_address: proj:follow-ups/10\n"+created(2)+"cancelled_at: "+statsStamp(1)+"\n", "J"),
			"12-l.md": issueMD("12", "done", created(1)+"done_at: "+statsStamp(0)+"\n", "L"),
		})
		writeProjectEpic(t, store, "proj", "loop", "---\nstatus: open\n---\n", map[string]string{
			// moved into another epic: counts by where it began.
			"09-i.md": issueMD("09", "done", "original_address: proj:follow-ups/09\n"+created(1)+"done_at: "+statsStamp(0)+"\n", "I"),
			// a move alone does not finish it.
			"11-k.md": issueMD("11", "open", "original_address: proj:follow-ups/11\n"+created(1), "K"),
			"13-m.md": issueMD("13", "done", created(1)+"done_at: "+statsStamp(0)+"\n", "M"),
		})
	})("")
	for _, c := range []struct {
		name   string
		series []int
		day    int
		want   int
	}{
		{"created", s.Throughput.FollowUpsCreated, 1, 4},
		{"created", s.Throughput.FollowUpsCreated, 2, 2},
		{"finished", s.Throughput.FollowUpsFinished, 0, 2},
		{"finished", s.Throughput.FollowUpsFinished, 1, 2},
	} {
		if got := statsAt(t, s, c.series, c.day); got != c.want {
			t.Errorf("follow-ups %s on day %d = %d, want %d", c.name, c.day, got, c.want)
		}
	}
}

func TestStats_WaitingForDesignCountsOpenMapDays(t *testing.T) {
	mapMD := func(status string, created int, completed int) string {
		md := "---\nkind: map\nstatus: " + status + "\ncreated_at: " + statsStamp(created) + "\n"
		if completed >= 0 {
			md += "completed_at: " + statsStamp(completed) + "\n"
		}
		return md + "---\n"
	}
	s := statsStore(t, func(store string) {
		writeProjectEpic(t, store, "proj", "draft", mapMD("draft", 10, -1), nil)
		writeProjectEpic(t, store, "proj", "open", mapMD("open", 5, -1), nil)
		writeProjectEpic(t, store, "proj", "done", mapMD("done", 8, 4), nil)
		writeProjectEpic(t, store, "proj", "old", mapMD("open", 100, -1), nil)
		writeProjectEpic(t, store, "proj", "not-a-map", "---\nstatus: open\ncreated_at: "+statsStamp(5)+"\n---\n", nil)
	})("")
	// old always; open from day 5; done from day 8 until it completes on day 4.
	for day, want := range map[int]int{9: 1, 8: 2, 6: 2, 5: 3, 4: 2, 0: 2} {
		if got := statsAt(t, s, s.Throughput.WaitingForDesign, day); got != want {
			t.Errorf("waiting for design on day %d = %d, want %d", day, got, want)
		}
	}
}

func statsAmountAt(t *testing.T, s server.Stats, series []server.StatsAmount, n int) server.StatsAmount {
	t.Helper()
	i := slices.Index(s.Days, statsDate(n))
	if i < 0 {
		t.Fatalf("day %s not in %v", statsDate(n), s.Days)
	}
	return series[i]
}

func TestStats_TokensAndSpendSplitDesignAndImplementation(t *testing.T) {
	stamp := func(tokens int, cost string) string {
		return "total_tokens: " + strconv.Itoa(tokens) + "\nactual_cost: " + cost + "\n"
	}
	done := func(day int) string { return "done_at: " + statsStamp(day) + "\n" }
	s := statsStore(t, func(store string) {
		writeProjectEpic(t, store, "proj", "design", "---\nkind: map\nstatus: done\ncompleted_at: "+statsStamp(2)+"\n"+stamp(100, "1.0")+"---\n", map[string]string{
			"01-a.md": issueMD("01", "done", done(3)+stamp(30, "0.3"), "A"),
			"02-b.md": issueMD("02", "done", done(2)+stamp(20, "0.2"), "B"),
		})
		writeProjectEpic(t, store, "proj", "old-map", "---\nkind: map\nstatus: done\ncompleted_at: "+statsStamp(8)+"\n---\n", nil)
		writeProjectEpic(t, store, "proj", "loop", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", done(4)+stamp(100, "1.0"), "A"),
			"02-b.md": issueMD("02", "done", done(4), "B"),
			"03-c.md": issueMD("03", "done", done(5), "C"),
			"04-d.md": issueMD("04", "cancelled", "cancelled_at: "+statsStamp(6)+"\n", "D"),
			"05-e.md": issueMD("05", "done", done(7)+stamp(0, "0"), "E"),
		})
	})("")
	cases := []struct {
		name   string
		series []server.StatsAmount
		day    int
		want   server.StatsAmount
	}{
		{"design tokens: ticket", s.Cost.TokensDesign, 3, server.StatsAmount{Value: 30}},
		{"design tokens: ticket plus map remainder", s.Cost.TokensDesign, 2, server.StatsAmount{Value: 70}},
		{"design spend: ticket plus map remainder", s.Cost.SpendDesign, 2, server.StatsAmount{Value: 0.7}},
		{"design: unstamped pre-recording map", s.Cost.TokensDesign, 8, server.StatsAmount{NoData: true}},
		{"design: no finished items", s.Cost.TokensDesign, 4, server.StatsAmount{}},
		{"impl tokens: mixed day is data", s.Cost.TokensImplementation, 4, server.StatsAmount{Value: 100}},
		{"impl spend: mixed day is data", s.Cost.SpendImplementation, 4, server.StatsAmount{Value: 1}},
		{"impl: only unstamped is no data", s.Cost.TokensImplementation, 5, server.StatsAmount{NoData: true}},
		{"impl spend: only unstamped is no data", s.Cost.SpendImplementation, 5, server.StatsAmount{NoData: true}},
		{"impl: only a cancel is zero", s.Cost.TokensImplementation, 6, server.StatsAmount{}},
		{"impl: stamped zero is data", s.Cost.TokensImplementation, 7, server.StatsAmount{}},
		{"impl: no finished items", s.Cost.TokensImplementation, 9, server.StatsAmount{}},
		{"impl: map days are not implementation", s.Cost.TokensImplementation, 2, server.StatsAmount{}},
	}
	for _, c := range cases {
		got := statsAmountAt(t, s, c.series, c.day)
		if math.Abs(got.Value-c.want.Value) > 1e-9 || got.NoData != c.want.NoData {
			t.Errorf("%s on day %d = %+v, want %+v", c.name, c.day, got, c.want)
		}
	}
}
