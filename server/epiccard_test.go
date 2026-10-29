package server_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

func TestEpicList_LoopCardFields(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	half := map[string]string{
		"01-a.md": issueMD("01", "done", "", "A"),
		"02-b.md": issueMD("02", "open", "", "B"),
	}
	paused := map[string]string{"01-a.md": issueMD("01", "paused", "pause_reason: self-reported\n", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "ready", loop, half)
		writeEpicFiles(t, store, "queued", loop, open)
		writeEpicFiles(t, store, "awaiting", loop, done)
		writeEpicFiles(t, store, "paused", loop, paused)
		writeEpicFiles(t, store, "blocked", "---\nstatus: open\ncompleted_at: 2026-01-01T11:00:00Z\nland_block_kind: self-reported\nland_block_reason: conflict\n---\n", done)
		writeEpicFiles(t, store, "merging", loop, map[string]string{
			"01-a.md": issueMD("01", "done", "", "A"),
			"02-m.md": strings.Replace(issueMD("02", "open", "", "M"), "type: implement", "type: merge", 1),
		})
		writeEpicFiles(t, store, "draft", "---\nstatus: draft\n---\n", open)
		writeEpicFiles(t, store, "noopen", loop, map[string]string{"01-a.md": issueMD("01", "claimed", "", "A")})
		writeEpicFiles(t, store, "merged", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
	})
	if res, err := h.Client.QueueAdd(t.Context(), "proj:queued", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Address] = r
	}

	queue := []server.CardAction{{Kind: server.CardActionQueue, Label: "Queue"}}
	merge := func(target string) []server.CardAction {
		return []server.CardAction{{Kind: server.CardActionMerge, Label: "Merge", Target: target}}
	}
	want := map[string]struct {
		section server.CardSection
		words   string
		ring    int
		actions []server.CardAction
	}{
		"proj:ready":    {server.SectionReadyToQueue, "Not started", 50, queue},
		"proj:queued":   {server.SectionImplementing, "Queued #1", 0, nil},
		"proj:awaiting": {server.SectionNeedsYou, "Implemented", 100, merge("proj:awaiting")},
		"proj:paused":   {server.SectionNeedsYou, "Implementing...", 0, []server.CardAction{{Kind: server.CardActionOpenTicket, Label: "Open ticket", Target: "proj:paused/01"}}},
		"proj:blocked":  {server.SectionNeedsYou, "Implemented", 100, merge("proj:blocked")},
		"proj:merging":  {server.SectionImplementing, "Merging...", 50, nil},
		"proj:draft":    {server.SectionReadyToDesign, "Draft", 0, nil},
		"proj:noopen":   {server.SectionReadyToDesign, "No open tickets", 0, nil},
		"proj:merged":   {server.SectionFinished, "Merged", 100, nil},
	}
	for addr, w := range want {
		r, ok := got[addr]
		if !ok {
			t.Errorf("%s is missing from %s", addr, body)
			continue
		}
		if r.CardKind != server.CardKindImplementation || r.Section != w.section || r.StatusWords != w.words || r.RingProgress != w.ring || !slices.Equal(r.Actions, w.actions) || !slices.Equal(r.Addresses, []string{addr}) {
			t.Errorf("%s = kind %q section %q words %q ring %d actions %v addresses %v, want section %q words %q ring %d actions %v",
				addr, r.CardKind, r.Section, r.StatusWords, r.RingProgress, r.Actions, r.Addresses, w.section, w.words, w.ring, w.actions)
		}
	}
}

func TestEpicList_QueuedOneOffSortsBySlotAndReadsQueuedN(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "q-first", loop, open)
		writeEpicFiles(t, store, "q-last", loop, open)
		writeEpicFiles(t, store, "one-offs", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "open", "", "Queued one"),
			"02-b.md": issueMD("02", "open", "", "Idle one"),
		})
	})
	for _, addr := range []string{"proj:q-first", "proj:one-offs/01", "proj:q-last"} {
		if res, err := h.Client.QueueAdd(t.Context(), addr, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", addr, res, err)
		}
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	var names, words []string
	for _, r := range getEpicList(t, code, body).Epics {
		if r.Section == server.SectionImplementing {
			names, words = append(names, r.Name), append(words, r.StatusWords)
		}
	}
	if want := []string{"q-first", "Queued one", "q-last"}; !slices.Equal(names, want) {
		t.Errorf("Implementing = %v, want %v", names, want)
	}
	if want := []string{"Queued #1", "Queued #2", "Queued #3"}; !slices.Equal(words, want) {
		t.Errorf("status words = %v, want %v", words, want)
	}
}

func TestEpicList_SectionSortOrders(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	loop := func(created string) string {
		return "---\nstatus: open\ncreated_at: " + created + "\n---\n"
	}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "old", loop("2026-01-01T10:00:00Z"), open)
		writeEpicFiles(t, store, "new", loop("2026-03-01T10:00:00Z"), open)
		writeEpicFiles(t, store, "q-second", loop("2026-01-01T10:00:00Z"), open)
		writeEpicFiles(t, store, "q-first", loop("2026-01-01T10:00:00Z"), open)
		writeEpicFiles(t, store, "zeta", "---\nstatus: draft\n---\n", open)
		writeEpicFiles(t, store, "alpha", "---\nstatus: draft\n---\n", open)
	})
	for _, name := range []string{"q-first", "q-second"} {
		if res, err := h.Client.QueueAdd(t.Context(), "proj:"+name, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", name, res, err)
		}
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	bySection := map[server.CardSection][]string{}
	for _, r := range getEpicList(t, code, body).Epics {
		bySection[r.Section] = append(bySection[r.Section], r.Name)
	}
	for section, want := range map[server.CardSection][]string{
		server.SectionImplementing:  {"q-first", "q-second"},
		server.SectionReadyToQueue:  {"new", "old"},
		server.SectionReadyToDesign: {"alpha", "zeta"},
	} {
		if !slices.Equal(bySection[section], want) {
			t.Errorf("%s = %v, want %v", section, bySection[section], want)
		}
	}
}
