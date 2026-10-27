package server_test

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func joinedMapMD(step string) string {
	return "---\nstatus: open\nkind: map\ndesign_step: " + step + "\n---\n"
}

func TestEpicList_JoinedCardPhases(t *testing.T) {
	store := t.TempDir()
	mapTickets := map[string]string{"01-a.md": issueMD("01", "done", "", "A"), "02-b.md": issueMD("02", "open", "", "B")}
	implTickets := map[string]string{"01-a.md": issueMD("01", "done", "", "A"), "02-b.md": issueMD("02", "open", "", "B"), "03-c.md": issueMD("03", "open", "", "C")}
	implMD := "---\nstatus: open\n---\n"
	draftImplMD := "---\nstatus: draft\n---\n"

	writeEpicFiles(t, store, "designing", joinedMapMD("spec"), mapTickets)
	writeEpicFiles(t, store, "creating", joinedMapMD("create-tickets"), mapTickets)
	writeEpicFiles(t, store, "creating-impl", draftImplMD, implTickets)
	writeEpicFiles(t, store, "building", joinedMapMD("branch"), mapTickets)
	writeEpicFiles(t, store, "building-impl", implMD, implTickets)
	writeEpicFiles(t, store, "queued-done", joinedMapMD("done"), mapTickets)
	writeEpicFiles(t, store, "queued-done-impl", implMD, implTickets)
	writeEpicFiles(t, store, "orphan-impl", implMD, implTickets)
	writeEpicFiles(t, store, "solo", joinedMapMD("spec"), mapTickets)

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

	for _, impl := range []string{"creating-impl", "building-impl", "queued-done-impl"} {
		if _, ok := got[impl]; ok {
			t.Errorf("%s has its own row, want it joined into its Design's card", impl)
		}
	}

	cases := []struct {
		name      string
		addresses []string
		kind      server.CardKind
		words     string
		ring      int
	}{
		{"designing", []string{"proj:designing"}, server.CardKindDesign, "Designing", 37},
		{"creating", []string{"proj:creating", "proj:creating-impl"}, server.CardKindDesign, "Creating tickets (3 so far)...", 0},
		{"building", []string{"proj:building-impl", "proj:building"}, server.CardKindImplementation, "Not started", 33},
		{"queued-done", []string{"proj:queued-done-impl", "proj:queued-done"}, server.CardKindImplementation, "Not started", 33},
		{"orphan-impl", []string{"proj:orphan-impl"}, server.CardKindImplementation, "Not started", 33},
		{"solo", []string{"proj:solo"}, server.CardKindDesign, "Designing", 37},
	}
	for _, c := range cases {
		r, ok := got[c.name]
		if !ok {
			t.Errorf("%s: no row", c.name)
			continue
		}
		if !slices.Equal(r.Addresses, c.addresses) || r.CardKind != c.kind || r.StatusWords != c.words || r.RingProgress != c.ring {
			t.Errorf("%s = addresses %v kind %q words %q ring %d, want %v %q %q %d",
				c.name, r.Addresses, r.CardKind, r.StatusWords, r.RingProgress, c.addresses, c.kind, c.words, c.ring)
		}
	}
	if s := got["queued-done"].Section; s != server.SectionReadyToQueue {
		t.Errorf("queued-done section = %q, want ready-to-queue", s)
	}
}

func TestEpicList_JoinedCardDesignQuestionWinsNeedsYou(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	store := t.TempDir()
	tix := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	writeEpicFiles(t, store, "m", joinedMapMD("done"), tix)
	writeEpicFiles(t, store, "m-impl", "---\nstatus: open\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))

	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	row := func() server.EpicListRow {
		code, body := webDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
		for _, r := range getEpicList(t, code, body).Epics {
			if r.Name == "m" {
				return r
			}
		}
		t.Fatal("no row for m")
		return server.EpicListRow{}
	}

	if r := row(); r.Section != server.SectionReadyToQueue {
		t.Fatalf("before the question, section = %q, want ready-to-queue", r.Section)
	}
	designSection := func() server.CardSection {
		_, d, _ := getEpicDetail(t, h, hs.WebURL, "proj:m")
		return d.DesignSection
	}
	if got := designSection(); got != server.SectionFinished {
		t.Fatalf("before the question, design_section = %q, want finished", got)
	}

	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:m"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	conv, _ := readMeta(t, h, res.Session.ID)["conversation_id"].(string)
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which store?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, res.Session.ID, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, res.Session.ID, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })

	r := row()
	if r.Section != server.SectionNeedsYou || r.StatePill != server.PillQuestion {
		t.Errorf("section %q pill %q, want needs-you Question", r.Section, r.StatePill)
	}
	if want := []string{"proj:m", "proj:m-impl"}; !slices.Equal(r.Addresses, want) {
		t.Errorf("addresses = %v, want %v", r.Addresses, want)
	}
	if r.CardKind != server.CardKindImplementation {
		t.Errorf("card kind = %q, want implementation", r.CardKind)
	}
	if got := designSection(); got != server.SectionNeedsYou {
		t.Errorf("design_section = %q, want needs-you", got)
	}
	want := []server.CardAction{{Kind: server.CardActionAnswer, Label: "Answer", Target: "proj:m", Link: "#/d/proj:m"}}
	if !slices.Equal(r.Actions, want) {
		t.Errorf("actions = %+v, want %+v", r.Actions, want)
	}
}

func TestEpicList_DoneJoinedCardWithQueuedImplSitsInProgress(t *testing.T) {
	tix := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "m", joinedMapMD("done"), tix)
		writeEpicFiles(t, store, "m-impl", "---\nstatus: open\n---\n", tix)
	})
	if res, err := h.Client.QueueAdd(t.Context(), "proj:m-impl", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	var row *server.EpicListRow
	for _, r := range getEpicList(t, code, body).Epics {
		if r.Name == "m" {
			row = &r
		}
	}
	if row == nil {
		t.Fatal("no row for the joined card")
	}
	if row.Section != server.SectionInProgress || row.QueueSlot != 1 {
		t.Errorf("section = %q, slot = %d, want in-progress and slot 1", row.Section, row.QueueSlot)
	}
}

func TestEpicList_JoinedCardCarriesImplQueueSlot(t *testing.T) {
	tix := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "other", "---\nstatus: open\n---\n", tix)
		writeEpicFiles(t, store, "queued", joinedMapMD("done"), tix)
		writeEpicFiles(t, store, "queued-impl", "---\nstatus: open\n---\n", tix)
		writeEpicFiles(t, store, "idle", joinedMapMD("done"), tix)
		writeEpicFiles(t, store, "idle-impl", "---\nstatus: open\n---\n", tix)
	})
	for _, name := range []string{"other", "queued-impl"} {
		if res, err := h.Client.QueueAdd(t.Context(), "proj:"+name, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", name, res, err)
		}
	}
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Name] = r
	}
	if s := got["queued"].QueueSlot; s != 2 {
		t.Errorf("queued card slot = %d, want its -impl epic's slot 2", s)
	}
	if s := got["idle"].QueueSlot; s != 0 {
		t.Errorf("card with no queued impl slot = %d, want 0", s)
	}
}
