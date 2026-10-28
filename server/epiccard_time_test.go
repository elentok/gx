package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func TestEpicList_CardTimePair(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	paused := map[string]string{"01-a.md": issueMD("01", "paused", "pause_reason: self-reported\nclaimed_at: 2026-01-05T08:00:00Z\n", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "ready", loop, open)
		writeEpicFiles(t, store, "queued", loop, open)
		writeEpicFiles(t, store, "paused", loop, paused)
		writeEpicFiles(t, store, "draft", "---\nstatus: draft\ncreated_at: 2026-01-03T10:00:00Z\n---\n", open)
		writeEpicFiles(t, store, "noopen", loop, map[string]string{"01-a.md": issueMD("01", "claimed", "", "A")})
		writeEpicFiles(t, store, "merging", "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\nstarted_at: 2026-01-02T09:00:00Z\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "", "A"),
			"02-m.md": strings.Replace(issueMD("02", "open", "", "M"), "type: implement", "type: merge", 1),
		})
		writeEpicFiles(t, store, "merged", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "follow-ups", "---\nstatus: open\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "F")})
		writeEpicFiles(t, store, "one-offs", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "open", "", "Queued one"),
			"02-b.md": issueMD("02", "claimed", "claimed_at: 2026-01-04T08:00:00Z\n", "Running one"),
			"03-c.md": issueMD("03", "open", "", "Idle one"),
		})
	})
	before := time.Now().Add(-time.Second)
	for _, addr := range []string{"proj:queued", "proj:one-offs/01"} {
		if res, err := h.Client.QueueAdd(t.Context(), addr, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", addr, res, err)
		}
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Address] = r
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	fixed := map[string]struct {
		kind  server.CardTimeKind
		start time.Time
	}{
		"proj:ready":       {server.CardTimeCreated, at("2026-01-01T10:00:00Z")},
		"proj:draft":       {server.CardTimeCreated, at("2026-01-03T10:00:00Z")},
		"proj:noopen":      {server.CardTimeCreated, at("2026-01-01T10:00:00Z")},
		"proj:merging":     {server.CardTimeElapsed, at("2026-01-02T09:00:00Z")},
		"proj:merged":      {server.CardTimeFinished, at("2026-01-02T11:00:00Z")},
		"proj:one-offs/02": {server.CardTimeElapsed, at("2026-01-04T08:00:00Z")},
		"proj:follow-ups":  {"", time.Time{}},
		"proj:paused":      {server.CardTimeWaiting, at("2026-01-05T08:00:00Z")},
	}
	for addr, w := range fixed {
		r, ok := got[addr]
		if !ok {
			t.Errorf("%s is missing from %s", addr, body)
			continue
		}
		if r.TimeKind != w.kind || !r.TimeStart.Equal(w.start) {
			t.Errorf("%s time = %q %v, want %q %v", addr, r.TimeKind, r.TimeStart, w.kind, w.start)
		}
	}
	// Queued cards count from their own enqueue, taken now.
	for _, addr := range []string{"proj:queued", "proj:one-offs/01"} {
		r := got[addr]
		if r.TimeKind != server.CardTimeElapsed || r.TimeStart.Before(before) {
			t.Errorf("%s time = %q %v, want elapsed from the enqueue", addr, r.TimeKind, r.TimeStart)
		}
	}
}
