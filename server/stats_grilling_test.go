package server_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server"
)

func grillItem(id string, at time.Time, q, w int) conversations.GrillingItem {
	return conversations.GrillingItem{MessageID: id, At: at, Questions: q, Words: w}
}

// grillingStats serves /v1/stats with the given records seeded straight into
// the conversations dir.
func grillingStats(t *testing.T, recs []conversations.Record) (get func(query string) server.Stats) {
	t.Helper()
	h, base := startEpicDetail(t, func(store string) {
		writeProjectEpic(t, store, "proj", "design", "---\nkind: map\nstatus: open\n---\n", nil)
	})
	for _, r := range recs {
		if r.RecordedAt.IsZero() {
			r.RecordedAt = time.Now()
		}
		if err := conversations.Save(h.ConversationsDir, r); err != nil {
			t.Fatal(err)
		}
	}
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

func amountAt(t *testing.T, s server.Stats, series []server.StatsAmount, n int) server.StatsAmount {
	t.Helper()
	i := slices.Index(s.Days, statsDate(n))
	if i < 0 {
		t.Fatalf("day %s not in %v", statsDate(n), s.Days)
	}
	return series[i]
}

func TestStats_GrillingCountsLandOnTheirLocalDays(t *testing.T) {
	t.Parallel()
	late := statsNoon(3).Add(11*time.Hour + 59*time.Minute) // 23:59 local
	early := late.Add(2 * time.Minute)                      // 00:01 next local day
	get := grillingStats(t, []conversations.Record{
		{ID: "a", Ticket: "proj:design/01", Design: true, Grilling: []conversations.GrillingItem{
			grillItem("m1", late, 2, 40), grillItem("m2", early, 1, 10), grillItem("m3", early, 3, 5),
		}},
	})
	s := get("")
	if got := amountAt(t, s, s.Grilling.Words, 3); got.Value != 40 || got.NoData {
		t.Errorf("words before midnight = %+v, want 40", got)
	}
	if got := amountAt(t, s, s.Grilling.Words, 2); got.Value != 15 {
		t.Errorf("words after midnight = %+v, want 15", got)
	}
	if got := amountAt(t, s, s.Grilling.Questions, 2); got.Value != 4 {
		t.Errorf("questions after midnight = %+v, want 4", got)
	}
}

func TestStats_GrillingHatchesBeforeTheFirstItemAndZeroesAfter(t *testing.T) {
	t.Parallel()
	get := grillingStats(t, []conversations.Record{
		{ID: "a", Ticket: "proj:design/01", Design: true, Grilling: []conversations.GrillingItem{grillItem("m1", statsNoon(5), 1, 9)}},
	})
	s := get("")
	if got := amountAt(t, s, s.Grilling.Words, 6); !got.NoData {
		t.Errorf("day before first item = %+v, want no_data", got)
	}
	if got := amountAt(t, s, s.Grilling.Words, 5); got.NoData || got.Value != 9 {
		t.Errorf("first item day = %+v, want 9", got)
	}
	if got := amountAt(t, s, s.Grilling.Questions, 2); got.NoData || got.Value != 0 {
		t.Errorf("empty later day = %+v, want 0", got)
	}
}

func TestStats_GrillingNothingCountedIsAllNoData(t *testing.T) {
	t.Parallel()
	get := grillingStats(t, []conversations.Record{
		{ID: "uncounted", Ticket: "proj:design/01", Design: true},
		{ID: "impl", Ticket: "proj:loop/01", Grilling: []conversations.GrillingItem{grillItem("m1", statsNoon(2), 1, 9)}},
	})
	s := get("")
	for _, series := range [][]server.StatsAmount{s.Grilling.Words, s.Grilling.Questions} {
		if len(series) != len(s.Days) {
			t.Fatalf("series has %d points, want %d", len(series), len(s.Days))
		}
		for i, a := range series {
			if !a.NoData {
				t.Errorf("day %s = %+v, want no_data", s.Days[i], a)
			}
		}
	}
}

func TestStats_GrillingCountsAMessageOnce(t *testing.T) {
	t.Parallel()
	item := grillItem("same", statsNoon(2), 1, 7)
	get := grillingStats(t, []conversations.Record{
		{ID: "a", Ticket: "proj:design/01", Design: true, Grilling: []conversations.GrillingItem{item}},
		{ID: "b", Ticket: "proj:design/02", Design: true, Grilling: []conversations.GrillingItem{item}},
	})
	s := get("")
	if got := amountAt(t, s, s.Grilling.Words, 2); got.Value != 7 {
		t.Errorf("words = %+v, want 7", got)
	}
}

func TestStats_GrillingProjectFilterKeepsTheGlobalBoundary(t *testing.T) {
	t.Parallel()
	get := grillingStats(t, []conversations.Record{
		{ID: "a", Ticket: "proj:design/01", Design: true, Grilling: []conversations.GrillingItem{grillItem("m1", statsNoon(2), 1, 7)}},
		{ID: "b", Ticket: "other:design/01", Design: true, Grilling: []conversations.GrillingItem{grillItem("m2", statsNoon(6), 1, 30)}},
	})
	s := get("?project=proj")
	if got := amountAt(t, s, s.Grilling.Words, 2); got.Value != 7 {
		t.Errorf("proj day 2 = %+v, want 7", got)
	}
	if got := amountAt(t, s, s.Grilling.Words, 6); got.NoData || got.Value != 0 {
		t.Errorf("other project's day = %+v, want 0 (counted, not hatched)", got)
	}
	if got := amountAt(t, s, s.Grilling.Words, 7); !got.NoData {
		t.Errorf("day before the global first item = %+v, want no_data", got)
	}
}
