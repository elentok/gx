package server_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	paused := map[string]string{"01-a.md": issueMD("01", "paused", "pause_reason: self-reported\n", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	mapMD := "---\nstatus: open\nkind: map\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "ready", loop, open)
		writeEpicFiles(t, store, "draft", "---\nstatus: draft\n---\n", open)
		writeEpicFiles(t, store, "queued", loop, open)
		writeEpicFiles(t, store, "awaiting", loop, done)
		writeEpicFiles(t, store, "paused", loop, paused)
		writeEpicFiles(t, store, "blocked", "---\nstatus: open\ncompleted_at: 2026-01-01T11:00:00Z\nland_block_kind: self-reported\nland_block_reason: conflict\n---\n", done)
		writeEpicFiles(t, store, "merging", loop, map[string]string{
			"01-a.md": issueMD("01", "done", "", "A"),
			"02-m.md": strings.Replace(issueMD("02", "open", "", "M"), "type: implement", "type: merge", 1),
		})
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
		"proj:paused":           {server.EpicListPaused, server.EpicKindLoop},
		"proj:blocked":          {server.EpicListMergeBlocked, server.EpicKindLoop},
		"proj:merging":          {server.EpicListMerging, server.EpicKindLoop},
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
	for addr, r := range got {
		kind, reason := "", ""
		if addr == "proj:blocked" {
			kind, reason = "self-reported", "conflict"
		}
		if r.MergeBlockKind != kind || r.MergeBlockReason != reason {
			t.Errorf("%s merge block = %q %q, want %q %q", addr, r.MergeBlockKind, r.MergeBlockReason, kind, reason)
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

func TestEpicList_BlockedByEpicsIgnoresMerged(t *testing.T) {
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "live", "---\nstatus: open\n---\n", open)
		writeEpicFiles(t, store, "landed", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "waiter", "---\nstatus: open\nblocked_by: [live, \"proj:landed\", ghost]\n---\n", open)
		writeEpicFiles(t, store, "free", "---\nstatus: open\nblocked_by: [landed]\n---\n", open)
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	byName := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		byName[r.Name] = r
	}
	if got := byName["waiter"].BlockedByEpics; !slices.Equal(got, []string{"proj:live"}) {
		t.Errorf("waiter blocked_by_epics = %v, want [proj:live]", got)
	}
	if got := byName["free"].BlockedByEpics; len(got) != 0 {
		t.Errorf("free blocked_by_epics = %v, want none", got)
	}
}

func TestEpicList_LastActivityIsTheNewestStamp(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "busy", "---\nstatus: open\nkind: map\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "claimed_at: 2026-01-01T10:00:00Z\ndone_at: 2026-01-03T10:00:00Z\n", "A"),
			"02-b.md": issueMD("02", "claimed", "claimed_at: 2026-01-02T10:00:00Z\n", "B"),
		})
		writeEpicFiles(t, store, "idle", "---\nstatus: open\nkind: map\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	byName := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		byName[r.Name] = r
	}
	if got := byName["busy"].LastActivityAt.Format("2006-01-02T15:04:05Z"); got != "2026-01-03T10:00:00Z" {
		t.Errorf("busy last_activity_at = %s", got)
	}
	if !byName["idle"].LastActivityAt.IsZero() {
		t.Errorf("idle last_activity_at = %v, want zero", byName["idle"].LastActivityAt)
	}
}

func TestEpicList_FollowUpCounts(t *testing.T) {
	loop := "---\nstatus: open\n---\n"
	followUps := map[string]string{
		"01-a.md": issueMD("01", "draft", "", "A"),
		"02-b.md": issueMD("02", "open", "", "B"),
		"03-c.md": issueMD("03", "done", "", "C"),
		"04-d.md": issueMD("04", "cancelled", "", "D"),
	}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "follow-ups", loop, followUps)
		writeEpicFiles(t, store, "work", loop, map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
		// "empty" has a follow-ups epic with nothing open; "none" has no such epic.
		for project, epics := range map[string]map[string]map[string]string{
			"other": {"follow-ups": {"01-a.md": issueMD("01", "open", "", "A")}},
			"empty": {"follow-ups": {"01-a.md": issueMD("01", "done", "", "A")}},
			"none":  {"work": {"01-a.md": issueMD("01", "open", "", "A")}},
		} {
			for epic, issues := range epics {
				writeProjectEpic(t, store, project, epic, loop, issues)
			}
		}
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]int{}
	for _, f := range getEpicList(t, code, body).FollowUps {
		got[f.Project] = f.Open
	}
	if want := map[string]int{"proj": 2, "other": 1}; !maps.Equal(got, want) {
		t.Errorf("follow_ups = %v, want %v: %s", got, want, body)
	}
}

func writeProjectEpic(t *testing.T, store, project, epic, ticketMD string, issues map[string]string) {
	t.Helper()
	dir := filepath.Join(store, project, epic)
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(store, project, "project.json"): `{"name":"` + project + `"}`,
		filepath.Join(dir, "ticket.md"):               ticketMD,
	}
	for name, content := range issues {
		files[filepath.Join(dir, "issues", name)] = content
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEpicList_NeedsTheWebCookie(t *testing.T) {
	_, base := startWeb(t)
	if code, _ := webDoCookie(t, http.MethodGet, base+"/v1/epics", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/epics without cookie = %d, want 401", code)
	}
}
