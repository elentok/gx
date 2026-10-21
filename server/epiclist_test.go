package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elentok/gx/server"
)

func getEpicList(t *testing.T, code int, body string) server.EpicList {
	t.Helper()
	var l server.EpicList
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return l
}

func TestEpicList_SectionRules(t *testing.T) {
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	claimed := map[string]string{"01-a.md": issueMD("01", "claimed", "", "A")}
	parked := map[string]string{"01-a.md": issueMD("01", "needs-answer", "park_kind: self-reported\n", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	mapMD := "---\nstatus: open\nkind: map\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "ready", loop, open)
		writeEpicFiles(t, store, "draft", "---\nstatus: draft\n---\n", open)
		writeEpicFiles(t, store, "queued", loop, open)
		writeEpicFiles(t, store, "awaiting", loop, done)
		writeEpicFiles(t, store, "parked", loop, parked)
		writeEpicFiles(t, store, "unqueued-claimed", loop, claimed)
		writeEpicFiles(t, store, "merged", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "noopen", loop, map[string]string{"01-a.md": issueMD("01", "cancelled", "", "A")})
		writeEpicFiles(t, store, "designing", mapMD, claimed)
		writeEpicFiles(t, store, "designing-done", mapMD, done)
		writeEpicFiles(t, store, "waiting", mapMD, open)
		writeEpicFiles(t, store, "mapdone", "---\nstatus: open\nkind: map\ncompleted_at: 2026-01-03T11:00:00Z\n---\n", done)
		for _, hidden := range []string{"one-offs", "follow-ups", "work-archived", "follow-ups-archived", "one-offs-archived"} {
			writeEpicFiles(t, store, hidden, loop, open)
		}
	})
	if res, err := h.Client.QueueAdd(t.Context(), "proj:queued", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	list := getEpicList(t, code, body)

	want := map[string]struct {
		state server.EpicListState
		kind  string
	}{
		"proj:ready":            {server.EpicListReady, server.EpicKindLoop},
		"proj:draft":            {server.EpicListDraft, server.EpicKindLoop},
		"proj:queued":           {server.EpicListQueued, server.EpicKindLoop},
		"proj:awaiting":         {server.EpicListAwaitingMerge, server.EpicKindLoop},
		"proj:parked":           {server.EpicListParked, server.EpicKindLoop},
		"proj:unqueued-claimed": {server.EpicListNoOpenTickets, server.EpicKindLoop},
		"proj:merged":           {server.EpicListFinished, server.EpicKindLoop},
		"proj:noopen":           {server.EpicListAwaitingMerge, server.EpicKindLoop},
		"proj:designing":        {server.EpicListDesigning, server.EpicKindMap},
		"proj:designing-done":   {server.EpicListDesigning, server.EpicKindMap},
		"proj:waiting":          {server.EpicListWaitingToDesign, server.EpicKindMap},
		"proj:mapdone":          {server.EpicListFinished, server.EpicKindMap},
	}
	got := map[string]server.EpicListRow{}
	for _, r := range list.Epics {
		got[r.Address] = r
	}
	for addr, w := range want {
		r, ok := got[addr]
		if !ok {
			t.Errorf("%s is missing from %s", addr, body)
			continue
		}
		if r.State != w.state || r.Kind != w.kind {
			t.Errorf("%s = state %q kind %q, want %q %q", addr, r.State, r.Kind, w.state, w.kind)
		}
	}
	if len(got) != len(want) {
		t.Errorf("listed %d epics, want %d (one-offs, follow-ups and archived are never listed): %s", len(got), len(want), body)
	}
}

func TestEpicList_RowFields(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "ready", "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "", "A"),
			"02-b.md": issueMD("02", "open", "", "B"),
		})
		writeEpicFiles(t, store, "merged", "---\nstatus: done\ncompleted_at: 2026-01-01T11:00:00Z\nmerged_at: 2026-01-02T11:00:00Z\n---\n",
			map[string]string{"01-a.md": issueMD("01", "done", "", "A")})
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	var byName = map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		byName[r.Name] = r
	}
	r := byName["ready"]
	if r.Address != "proj:ready" || r.Project != "proj" || r.Done != 1 || r.Total != 2 || !r.Queueable || r.Queued || r.CreatedAt.Format("2006-01-02T15:04:05Z") != "2026-01-01T10:00:00Z" {
		t.Errorf("ready = %+v", r)
	}
	m := byName["merged"]
	if m.CompletedAt.IsZero() || m.MergedAt.IsZero() || m.Queueable {
		t.Errorf("merged = %+v", m)
	}
}

func TestEpicList_NeedsTheWebCookie(t *testing.T) {
	_, base := startWeb(t)
	if code, _ := webDoCookie(t, http.MethodGet, base+"/v1/epics", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/epics without cookie = %d, want 401", code)
	}
}
