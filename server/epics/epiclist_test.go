package epics_test

import (
	"github.com/elentok/gx/server/servertest/rigs"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/servertest"
)

func TestEpicList_SectionRules(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	claimed := map[string]string{"01-a.md": rigs.IssueMD("01", "claimed", "", "A")}
	paused := map[string]string{"01-a.md": rigs.IssueMD("01", "paused", "pause_reason: self-reported\n", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	mapMD := "---\nstatus: open\nkind: map\n---\n"
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "ready", loop, open)
		rigs.WriteEpicFiles(t, store, "draft", "---\nstatus: draft\n---\n", open)
		rigs.WriteEpicFiles(t, store, "queued", loop, open)
		rigs.WriteEpicFiles(t, store, "awaiting", loop, done)
		rigs.WriteEpicFiles(t, store, "paused", loop, paused)
		rigs.WriteEpicFiles(t, store, "blocked", "---\nstatus: open\ncompleted_at: 2026-01-01T11:00:00Z\nland_block_kind: self-reported\nland_block_reason: conflict\n---\n", done)
		rigs.WriteEpicFiles(t, store, "merging", loop, map[string]string{
			"01-a.md": rigs.IssueMD("01", "done", "", "A"),
			"02-m.md": strings.Replace(rigs.IssueMD("02", "open", "", "M"), "type: implement", "type: merge", 1),
		})

		rigs.WriteEpicFiles(t, store, "unqueued-claimed", loop, claimed)
		rigs.WriteEpicFiles(t, store, "merged", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
		rigs.WriteEpicFiles(t, store, "noopen", loop, map[string]string{"01-a.md": rigs.IssueMD("01", "cancelled", "", "A")})
		rigs.WriteEpicFiles(t, store, "designing", mapMD, claimed)
		rigs.WriteEpicFiles(t, store, "designing-done", mapMD, done)
		rigs.WriteEpicFiles(t, store, "waiting", mapMD, open)
		rigs.WriteEpicFiles(t, store, "mapdone", "---\nstatus: open\nkind: map\ncompleted_at: 2026-01-03T11:00:00Z\n---\n", done)
		for _, hidden := range []string{"one-offs", "follow-ups", "work-archived", "follow-ups-archived", "one-offs-archived"} {
			rigs.WriteEpicFiles(t, store, hidden, loop, open)
		}
	})

	if res, err := h.Client.Queue.Add(t.Context(), "proj:queued", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	list := rigs.GetEpicList(t, code, body)

	want := map[string]struct {
		state wire.EpicListState
		kind  string
	}{
		"proj:ready":            {wire.EpicListReady, wire.EpicKindLoop},
		"proj:draft":            {wire.EpicListDraft, wire.EpicKindLoop},
		"proj:queued":           {wire.EpicListQueued, wire.EpicKindLoop},
		"proj:awaiting":         {wire.EpicListAwaitingMerge, wire.EpicKindLoop},
		"proj:paused":           {wire.EpicListPaused, wire.EpicKindLoop},
		"proj:blocked":          {wire.EpicListMergeBlocked, wire.EpicKindLoop},
		"proj:merging":          {wire.EpicListMerging, wire.EpicKindLoop},
		"proj:unqueued-claimed": {wire.EpicListNoOpenTickets, wire.EpicKindLoop},
		"proj:merged":           {wire.EpicListFinished, wire.EpicKindLoop},
		"proj:noopen":           {wire.EpicListAwaitingMerge, wire.EpicKindLoop},
		"proj:designing":        {wire.EpicListDesigning, wire.EpicKindMap},
		"proj:designing-done":   {wire.EpicListDesigning, wire.EpicKindMap},
		"proj:waiting":          {wire.EpicListWaitingToDesign, wire.EpicKindMap},
		"proj:mapdone":          {wire.EpicListFinished, wire.EpicKindMap},
		"proj:follow-ups":       {wire.EpicListFollowUps, wire.EpicKindLoop},
	}
	got := map[string]wire.EpicListRow{}
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
		t.Errorf("listed %d epics, want %d (an unqueued one-off and archived epics are never listed): %s", len(got), len(want), body)
	}
}

func TestEpicList_RowFields(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "ready", "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n", map[string]string{
			"01-a.md": rigs.IssueMD("01", "done", "", "A"),
			"02-b.md": rigs.IssueMD("02", "open", "", "B"),
		})

		rigs.WriteEpicFiles(t, store, "merged", "---\nstatus: done\ncompleted_at: 2026-01-01T11:00:00Z\nmerged_at: 2026-01-02T11:00:00Z\n---\n",
			map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")})

	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	var byName = map[string]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
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
	t.Parallel()
	done := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	open := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "live", "---\nstatus: open\n---\n", open)
		rigs.WriteEpicFiles(t, store, "landed", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
		rigs.WriteEpicFiles(t, store, "waiter", "---\nstatus: open\nblocked_by: [live, \"proj:landed\", ghost]\n---\n", open)
		rigs.WriteEpicFiles(t, store, "free", "---\nstatus: open\nblocked_by: [landed]\n---\n", open)
	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	byName := map[string]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
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
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "busy", "---\nstatus: open\nkind: map\n---\n", map[string]string{
			"01-a.md": rigs.IssueMD("01", "done", "claimed_at: 2026-01-01T10:00:00Z\ndone_at: 2026-01-03T10:00:00Z\n", "A"),
			"02-b.md": rigs.IssueMD("02", "claimed", "claimed_at: 2026-01-02T10:00:00Z\n", "B"),
		})

		rigs.WriteEpicFiles(t, store, "idle", "---\nstatus: open\nkind: map\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	byName := map[string]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
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
	t.Parallel()
	loop := "---\nstatus: open\n---\n"
	followUps := map[string]string{
		"01-a.md": rigs.IssueMD("01", "draft", "", "A"),
		"02-b.md": rigs.IssueMD("02", "open", "", "B"),
		"03-c.md": rigs.IssueMD("03", "done", "", "C"),
		"04-d.md": rigs.IssueMD("04", "cancelled", "", "D"),
	}
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "follow-ups", loop, followUps)
		rigs.WriteEpicFiles(t, store, "work", loop, map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})

		for project, epics := range map[string]map[string]map[string]string{
			"other": {"follow-ups": {"01-a.md": rigs.IssueMD("01", "open", "", "A")}},
			"empty": {"follow-ups": {"01-a.md": rigs.IssueMD("01", "done", "", "A")}},
			"none":  {"work": {"01-a.md": rigs.IssueMD("01", "open", "", "A")}},
		} {
			for epic, issues := range epics {
				rigs.WriteProjectEpic(t, store, project, epic, loop, issues)
			}
		}
	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]int{}
	for _, f := range rigs.GetEpicList(t, code, body).FollowUps {
		got[f.Project] = f.Open
	}
	if want := map[string]int{"proj": 2, "other": 1}; !maps.Equal(got, want) {
		t.Errorf("follow_ups = %v, want %v: %s", got, want, body)
	}
}

func TestEpicList_FollowUpsCardCountsByCategoryAndComesFirst(t *testing.T) {
	t.Parallel()
	loop := "---\nstatus: open\n---\n"
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "follow-ups", loop, map[string]string{
			"01-a.md": rigs.IssueMD("01", "open", "category: bug\n", "A"),
			"02-b.md": rigs.IssueMD("02", "open", "category: bug\n", "B"),
			"03-c.md": rigs.IssueMD("03", "draft", "category: idea\n", "C"),
			"04-d.md": rigs.IssueMD("04", "done", "category: bug\n", "D"),
		})

		rigs.WriteEpicFiles(t, store, "aaa", "---\nstatus: draft\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
		rigs.WriteProjectEpic(t, store, "empty", "follow-ups", loop, map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")})
	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	var cards, design []wire.EpicListRow
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
		if r.Name == "follow-ups" {
			cards = append(cards, r)
		}
		if r.Section == wire.SectionReadyToDesign {
			design = append(design, r)
		}
	}
	if len(cards) != 1 {
		t.Fatalf("follow-ups cards = %+v, want one (none for the project with nothing open): %s", cards, body)
	}
	c := cards[0]
	if c.Address != "proj:follow-ups" || c.CardKind != wire.CardKindFollowUps || c.StatusWords != "2 bugs, 1 idea" {
		t.Errorf("card = %+v", c)
	}
	if len(c.Actions) != 1 || c.Actions[0].Kind != wire.CardActionTriage || c.Actions[0].Target != "proj" {
		t.Errorf("actions = %+v, want Triage on proj", c.Actions)
	}
	if len(design) != 2 || design[0].Address != "proj:follow-ups" {
		t.Errorf("Ready to design = %+v, want the follow-ups card first", design)
	}
}

func TestEpicList_OneOffCardsAndTheFinishedCap(t *testing.T) {
	t.Parallel()
	loop := "---\nstatus: open\n---\n"
	archived := func(id, at string) string {
		return rigs.IssueMD(id, "done", "completed_at: "+at+"\n", "Old "+id)
	}
	store := t.TempDir()
	rigs.WriteEpicFiles(t, store, "one-offs", loop, map[string]string{
		"01-queued.md": rigs.IssueMD("01", "open", "", "Queued job"),
		"02-paused.md": rigs.IssueMD("02", "paused", "pause_reason: self-reported\n", "Paused job"),
		"03-idle.md":   rigs.IssueMD("03", "open", "", "Idle job"),
	})

	rigs.WriteEpicFiles(t, store, "one-offs-archived", loop, map[string]string{
		"01-a.md": archived("01", "2026-03-01T10:00:00Z"),
		"02-b.md": archived("02", "2026-03-02T10:00:00Z"),
		"03-c.md": archived("03", "2026-03-03T10:00:00Z"),
	})

	done := func(at string) string { return "---\nstatus: done\nmerged_at: " + at + "\n---\n" }
	finished := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	rigs.WriteEpicFiles(t, store, "newer-epic", done("2026-01-02T10:00:00Z"), finished)
	rigs.WriteEpicFiles(t, store, "older-epic", done("2025-01-02T10:00:00Z"), finished)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0"; c.RecentlyFinished = 2 })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(t.Context(), "proj:one-offs/01", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	var addrs []string
	bySection := map[wire.CardSection][]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
		addrs = append(addrs, r.Address)
		bySection[r.Section] = append(bySection[r.Section], r)
	}
	if q := bySection[wire.SectionImplementing]; len(q) != 1 || q[0].Address != "proj:one-offs/01" || q[0].CardKind != wire.CardKindOneOff || q[0].State != wire.EpicListQueued {
		t.Errorf("Implementing = %+v, want the queued one-off", q)
	}
	if n := bySection[wire.SectionNeedsYou]; len(n) != 1 || n[0].Address != "proj:one-offs/02" || n[0].StatePill != epics.PillPaused || len(n[0].Actions) != 1 || n[0].Actions[0].Kind != wire.CardActionOpenTicket {
		t.Errorf("Needs you = %+v, want the paused one-off with its pill and action", n)
	}
	var got []string
	for _, r := range bySection[wire.SectionFinished] {
		got = append(got, r.Address)
	}
	// Every done epic stays; only the two newest Finished cards may be one-offs.
	want := []string{"proj:one-offs-archived/03", "proj:one-offs-archived/02", "proj:newer-epic", "proj:older-epic"}
	if !slices.Equal(got, want) {
		t.Errorf("Finished = %v, want %v (all cards: %v)", got, want, addrs)
	}
}

func TestEpicList_NeedsTheWebCookie(t *testing.T) {
	t.Parallel()
	_, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/epics", nil); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/epics without cookie = %d, want 401", code)
	}
}
