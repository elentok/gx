package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestEpicList_DesignAndJoinedCardsCarryTheTimePair(t *testing.T) {
	store := t.TempDir()
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	mapFront := func(extra string) string {
		return "---\nstatus: open\nkind: map\ndesign_step: spec\n" + extra + "---\n"
	}
	joinedFront := func(step, extra string) string {
		return "---\nstatus: open\nkind: map\ndesign_step: " + step + "\n" + extra + "---\n"
	}
	const (
		created = "created_at: 2026-01-01T10:00:00Z\n"
		started = "design_started_at: 2026-01-02T10:00:00Z\n"
		done    = "completed_at: 2026-01-03T10:00:00Z\n"
	)

	writeEpicFiles(t, store, "notstarted", "---\nstatus: open\nkind: map\ndesign_step: chart\ndestination_unconfirmed: true\n"+created+"---\n", open)
	writeEpicFiles(t, store, "stopped", mapFront(created+started), open)
	writeEpicFiles(t, store, "stopped-nostamp", mapFront(created), open)
	writeEpicFiles(t, store, "finished", "---\nstatus: done\nkind: map\ndesign_step: done\n"+created+done+"---\n", open)
	writeEpicFiles(t, store, "creating", joinedFront("create-tickets", created+started), open)
	writeEpicFiles(t, store, "creating-impl", "---\nstatus: draft\n---\n", open)
	writeEpicFiles(t, store, "branch", joinedFront("branch", created+started), open)
	writeEpicFiles(t, store, "branch-impl", "---\nstatus: open\ncreated_at: 2026-01-04T10:00:00Z\n---\n", open)
	writeEpicFiles(t, store, "merged", joinedFront("done", created+started+done), open)
	writeEpicFiles(t, store, "merged-impl", "---\nstatus: done\ncreated_at: 2026-01-04T10:00:00Z\ncompleted_at: 2026-01-05T10:00:00Z\nmerged_at: 2026-01-06T10:00:00Z\n---\n",
		map[string]string{"01-a.md": issueMD("01", "done", "", "A")})

	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	code, body := webDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Name] = r
	}

	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	for _, c := range []struct {
		name  string
		kind  server.CardTimeKind
		start time.Time
	}{
		{"notstarted", server.CardTimeCreated, at("2026-01-01T10:00:00Z")},
		{"stopped", server.CardTimeElapsed, at("2026-01-02T10:00:00Z")},
		{"stopped-nostamp", "", time.Time{}},
		{"finished", server.CardTimeFinished, at("2026-01-03T10:00:00Z")},
		{"creating", server.CardTimeElapsed, at("2026-01-02T10:00:00Z")},
		{"branch", server.CardTimeCreated, at("2026-01-04T10:00:00Z")},
		{"merged", server.CardTimeFinished, at("2026-01-06T10:00:00Z")},
	} {
		r, ok := got[c.name]
		if !ok {
			t.Errorf("%s: no row", c.name)
			continue
		}
		if r.TimeKind != c.kind || !r.TimeStart.Equal(c.start) {
			t.Errorf("%s: time = %q %v, want %q %v", c.name, r.TimeKind, r.TimeStart, c.kind, c.start)
		}
	}
}
