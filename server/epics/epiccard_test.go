package epics_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestEpicList_LoopCardFields(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": servertest.IssueMD("01", "done", "", "A")}
	half := map[string]string{
		"01-a.md": servertest.IssueMD("01", "done", "", "A"),
		"02-b.md": servertest.IssueMD("02", "open", "", "B"),
	}
	paused := map[string]string{"01-a.md": servertest.IssueMD("01", "paused", "pause_reason: self-reported\n", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	h, base := servertest.StartEpicDetail(t, func(store string) {
		servertest.WriteEpicFiles(t, store, "ready", loop, half)
		servertest.WriteEpicFiles(t, store, "queued", loop, open)
		servertest.WriteEpicFiles(t, store, "awaiting", loop, done)
		servertest.WriteEpicFiles(t, store, "paused", loop, paused)
		servertest.WriteEpicFiles(t, store, "blocked", "---\nstatus: open\ncompleted_at: 2026-01-01T11:00:00Z\nland_block_kind: self-reported\nland_block_reason: conflict\n---\n", done)
		servertest.WriteEpicFiles(t, store, "merging", loop, map[string]string{
			"01-a.md": servertest.IssueMD("01", "done", "", "A"),
			"02-m.md": strings.Replace(servertest.IssueMD("02", "open", "", "M"), "type: implement", "type: merge", 1),
		})

		servertest.WriteEpicFiles(t, store, "draft", "---\nstatus: draft\n---\n", open)
		servertest.WriteEpicFiles(t, store, "noopen", loop, map[string]string{"01-a.md": servertest.IssueMD("01", "claimed", "", "A")})
		servertest.WriteEpicFiles(t, store, "merged", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
		servertest.WriteEpicFiles(t, store, "empty", loop, map[string]string{})
	})

	if res, err := h.Client.Queue.Add(t.Context(), "proj:queued", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]wire.EpicListRow{}
	for _, r := range servertest.GetEpicList(t, code, body).Epics {
		got[r.Address] = r
	}

	queue := []wire.CardAction{{Kind: wire.CardActionQueue, Label: "Queue"}}
	merge := func(target string) []wire.CardAction {
		return []wire.CardAction{{Kind: wire.CardActionMerge, Label: "Merge", Target: target}}
	}
	want := map[string]struct {
		section wire.CardSection
		words   string
		ring    int
		actions []wire.CardAction
	}{
		"proj:ready":    {wire.SectionReadyToQueue, "Not started", 50, queue},
		"proj:queued":   {wire.SectionImplementing, "Queued #1", 0, nil},
		"proj:awaiting": {wire.SectionNeedsYou, "Implemented", 100, merge("proj:awaiting")},
		"proj:paused":   {wire.SectionNeedsYou, "Implementing...", 0, []wire.CardAction{{Kind: wire.CardActionOpenTicket, Label: "Open ticket", Target: "proj:paused/01"}}},
		"proj:blocked":  {wire.SectionNeedsYou, "Implemented", 100, merge("proj:blocked")},
		"proj:merging":  {wire.SectionImplementing, "Merging...", 50, nil},
		"proj:draft":    {wire.SectionReadyToDesign, "Draft", 0, nil},
		"proj:noopen":   {wire.SectionReadyToDesign, "No open tickets", 0, nil},
		"proj:merged":   {wire.SectionFinished, "Merged", 100, nil},
		// An empty epic counts as implemented (nothing is left to run).
		"proj:empty": {wire.SectionNeedsYou, "Implemented", 0, merge("proj:empty")},
	}
	for addr, w := range want {
		r, ok := got[addr]
		if !ok {
			t.Errorf("%s is missing from %s", addr, body)
			continue
		}
		if r.CardKind != wire.CardKindImplementation || r.Section != w.section || r.StatusWords != w.words || r.RingProgress != w.ring || !slices.Equal(r.Actions, w.actions) || !slices.Equal(r.Addresses, []string{addr}) {
			t.Errorf("%s = kind %q section %q words %q ring %d actions %v addresses %v, want section %q words %q ring %d actions %v",
				addr, r.CardKind, r.Section, r.StatusWords, r.RingProgress, r.Actions, r.Addresses, w.section, w.words, w.ring, w.actions)
		}
	}
}

func TestEpicList_QueuedOneOffSortsBySlotAndReadsQueuedN(t *testing.T) {
	t.Parallel()
	open := map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")}
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	h, base := servertest.StartEpicDetail(t, func(store string) {
		servertest.WriteEpicFiles(t, store, "q-first", loop, open)
		servertest.WriteEpicFiles(t, store, "q-last", loop, open)
		servertest.WriteEpicFiles(t, store, "one-offs", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": servertest.IssueMD("01", "open", "", "Queued one"),
			"02-b.md": servertest.IssueMD("02", "open", "", "Idle one"),
		})

	})

	for _, addr := range []string{"proj:q-first", "proj:one-offs/01", "proj:q-last"} {
		if res, err := h.Client.Queue.Add(t.Context(), addr, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", addr, res, err)
		}
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	var names, words []string
	for _, r := range servertest.GetEpicList(t, code, body).Epics {
		if r.Section == wire.SectionImplementing {
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
	open := map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")}
	loop := func(created string) string {
		return "---\nstatus: open\ncreated_at: " + created + "\n---\n"
	}
	h, base := servertest.StartEpicDetail(t, func(store string) {
		servertest.WriteEpicFiles(t, store, "old", loop("2026-01-01T10:00:00Z"), open)
		servertest.WriteEpicFiles(t, store, "new", loop("2026-03-01T10:00:00Z"), open)
		servertest.WriteEpicFiles(t, store, "q-second", loop("2026-01-01T10:00:00Z"), open)
		servertest.WriteEpicFiles(t, store, "q-first", loop("2026-01-01T10:00:00Z"), open)
		servertest.WriteEpicFiles(t, store, "zeta", "---\nstatus: draft\n---\n", open)
		servertest.WriteEpicFiles(t, store, "alpha", "---\nstatus: draft\n---\n", open)
	})

	for _, name := range []string{"q-first", "q-second"} {
		if res, err := h.Client.Queue.Add(t.Context(), "proj:"+name, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", name, res, err)
		}
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	bySection := map[wire.CardSection][]string{}
	for _, r := range servertest.GetEpicList(t, code, body).Epics {
		bySection[r.Section] = append(bySection[r.Section], r.Name)
	}
	for section, want := range map[wire.CardSection][]string{
		wire.SectionImplementing:  {"q-first", "q-second"},
		wire.SectionReadyToQueue:  {"new", "old"},
		wire.SectionReadyToDesign: {"alpha", "zeta"},
	} {
		if !slices.Equal(bySection[section], want) {
			t.Errorf("%s = %v, want %v", section, bySection[section], want)
		}
	}
}
