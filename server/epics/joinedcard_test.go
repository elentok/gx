package epics_test

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func joinedMapMD(step string) string {
	return "---\nstatus: open\nkind: map\ndesign_step: " + step + "\n---\n"
}

func TestEpicList_JoinedCardPhases(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	mapTickets := map[string]string{"01-a.md": servertest.IssueMD("01", "done", "", "A"), "02-b.md": servertest.IssueMD("02", "open", "", "B")}
	implTickets := map[string]string{"01-a.md": servertest.IssueMD("01", "done", "", "A"), "02-b.md": servertest.IssueMD("02", "open", "", "B"), "03-c.md": servertest.IssueMD("03", "open", "", "C")}
	implMD := "---\nstatus: open\n---\n"
	draftImplMD := "---\nstatus: draft\n---\n"

	servertest.WriteEpicFiles(t, store, "designing", joinedMapMD("spec"), mapTickets)
	servertest.WriteEpicFiles(t, store, "creating", joinedMapMD("create-tickets"), mapTickets)
	servertest.WriteEpicFiles(t, store, "creating-impl", draftImplMD, implTickets)
	servertest.WriteEpicFiles(t, store, "building", joinedMapMD("branch"), mapTickets)
	servertest.WriteEpicFiles(t, store, "building-impl", implMD, implTickets)
	servertest.WriteEpicFiles(t, store, "queued-done", joinedMapMD("done"), mapTickets)
	servertest.WriteEpicFiles(t, store, "queued-done-impl", implMD, implTickets)
	servertest.WriteEpicFiles(t, store, "orphan-impl", implMD, implTickets)
	servertest.WriteEpicFiles(t, store, "solo", joinedMapMD("spec"), mapTickets)

	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	got := map[string]wire.EpicListRow{}
	for _, r := range servertest.GetEpicList(t, code, body).Epics {
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
		kind      wire.CardKind
		words     string
		ring      int
	}{
		{"designing", []string{"proj:designing"}, wire.CardKindDesign, "Designing", 37},
		{"creating", []string{"proj:creating", "proj:creating-impl"}, wire.CardKindDesign, "Creating tickets (3 so far)...", 0},
		{"building", []string{"proj:building-impl", "proj:building"}, wire.CardKindImplementation, "Not started", 33},
		{"queued-done", []string{"proj:queued-done-impl", "proj:queued-done"}, wire.CardKindImplementation, "Not started", 33},
		{"orphan-impl", []string{"proj:orphan-impl"}, wire.CardKindImplementation, "Not started", 33},
		{"solo", []string{"proj:solo"}, wire.CardKindDesign, "Designing", 37},
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
	if s := got["queued-done"].Section; s != wire.SectionReadyToQueue {
		t.Errorf("queued-done section = %q, want ready-to-queue", s)
	}
}

func TestEpicList_JoinedCardDesignQuestionWinsNeedsYou(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	store := t.TempDir()
	tix := map[string]string{"01-a.md": servertest.IssueMD("01", "done", "", "A")}
	servertest.WriteEpicFiles(t, store, "m", joinedMapMD("done"), tix)
	servertest.WriteEpicFiles(t, store, "m-impl", "---\nstatus: open\n---\n", map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))

	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
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
	row := func() wire.EpicListRow {
		code, body := servertest.WebDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
		for _, r := range servertest.GetEpicList(t, code, body).Epics {
			if r.Name == "m" {
				return r
			}
		}
		t.Fatal("no row for m")
		return wire.EpicListRow{}
	}

	if r := row(); r.Section != wire.SectionReadyToQueue {
		t.Fatalf("before the question, section = %q, want ready-to-queue", r.Section)
	}
	designSection := func() wire.CardSection {
		_, d, _ := getEpicDetail(t, h, hs.WebURL, "proj:m")
		return d.DesignSection
	}
	if got := designSection(); got != wire.SectionFinished {
		t.Fatalf("before the question, design_section = %q, want finished", got)
	}

	// The seeded done step makes the first rescan stop a live design session, off
	// the rescan's goroutine; let it finish before the session exists.
	select {
	case <-h.Server.StartupDone():
	case <-ctx.Done():
		t.Fatal("startup did not finish")
	}
	if !h.Server.WaitSettled(10 * time.Second) {
		t.Fatal("startup design cleanup did not settle")
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:m"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	conv, _ := readMeta(t, h, res.Session.ID)["conversation_id"].(string)
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which store?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, res.Session.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, res.Session.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })

	r := row()
	if r.Section != wire.SectionNeedsYou || r.StatePill != epics.PillQuestion {
		t.Errorf("section %q pill %q, want needs-you Question", r.Section, r.StatePill)
	}
	if want := []string{"proj:m", "proj:m-impl"}; !slices.Equal(r.Addresses, want) {
		t.Errorf("addresses = %v, want %v", r.Addresses, want)
	}
	if r.CardKind != wire.CardKindImplementation {
		t.Errorf("card kind = %q, want implementation", r.CardKind)
	}
	if r.TimeKind != wire.CardTimeWaiting || !r.TimeStart.Equal(r.WaitingSince) {
		t.Errorf("time = %q %v, want waiting since %v", r.TimeKind, r.TimeStart, r.WaitingSince)
	}
	if got := designSection(); got != wire.SectionNeedsYou {
		t.Errorf("design_section = %q, want needs-you", got)
	}
	want := []wire.CardAction{{Kind: wire.CardActionAnswer, Label: "Answer", Target: "proj:m", Link: "#/d/proj:m"}}
	if !slices.Equal(r.Actions, want) {
		t.Errorf("actions = %+v, want %+v", r.Actions, want)
	}
}

func TestEpicList_DoneJoinedCardWithQueuedImplSitsInProgress(t *testing.T) {
	t.Parallel()
	tix := map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")}
	h, base := servertest.StartEpicDetail(t, func(store string) {
		servertest.WriteEpicFiles(t, store, "m", joinedMapMD("done"), tix)
		servertest.WriteEpicFiles(t, store, "m-impl", "---\nstatus: open\n---\n", tix)
	})

	if res, err := h.Client.Queue.Add(t.Context(), "proj:m-impl", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	var row *wire.EpicListRow
	for _, r := range servertest.GetEpicList(t, code, body).Epics {
		if r.Name == "m" {
			row = &r
		}
	}
	if row == nil {
		t.Fatal("no row for the joined card")
	}
	if row.Section != wire.SectionImplementing || row.QueueSlot != 1 {
		t.Errorf("section = %q, slot = %d, want implementing and slot 1", row.Section, row.QueueSlot)
	}
}

func TestEpicList_JoinedCardCarriesImplQueueSlot(t *testing.T) {
	t.Parallel()
	tix := map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")}
	h, base := servertest.StartEpicDetail(t, func(store string) {
		servertest.WriteEpicFiles(t, store, "other", "---\nstatus: open\n---\n", tix)
		servertest.WriteEpicFiles(t, store, "queued", joinedMapMD("done"), tix)
		servertest.WriteEpicFiles(t, store, "queued-impl", "---\nstatus: open\n---\n", tix)
		servertest.WriteEpicFiles(t, store, "idle", joinedMapMD("done"), tix)
		servertest.WriteEpicFiles(t, store, "idle-impl", "---\nstatus: open\n---\n", tix)
	})

	for _, name := range []string{"other", "queued-impl"} {
		if res, err := h.Client.Queue.Add(t.Context(), "proj:"+name, ""); err != nil || res.Refused {
			t.Fatalf("queue add %s = %+v, %v", name, res, err)
		}
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]wire.EpicListRow{}
	for _, r := range servertest.GetEpicList(t, code, body).Epics {
		got[r.Name] = r
	}
	if s := got["queued"].QueueSlot; s != 2 {
		t.Errorf("queued card slot = %d, want its -impl epic's slot 2", s)
	}
	if s := got["idle"].QueueSlot; s != 0 {
		t.Errorf("card with no queued impl slot = %d, want 0", s)
	}
}
