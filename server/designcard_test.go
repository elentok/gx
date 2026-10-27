package server_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func epicRows(t *testing.T, h *servertest.Harness, f designsFixture) map[string]server.EpicListRow {
	t.Helper()
	list, err := h.Client.Epics(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]server.EpicListRow{}
	for _, r := range list.Epics {
		got[r.Name] = r
	}
	return got
}

// waitForEpicRow polls for the row of name: the epic index follows the store
// through a file watcher, so a map written just now shows up a moment later.
func waitForEpicRow(t *testing.T, f designsFixture, name string, ok func(server.EpicListRow) bool) server.EpicListRow {
	t.Helper()
	var r server.EpicListRow
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		var found bool
		if r, found = epicRows(t, f.h, f)[name]; found && ok(r) {
			return r
		}
	}
	t.Fatalf("row %q = %+v, never matched", name, r)
	return r
}

func actionKinds(r server.EpicListRow) []server.CardActionKind {
	var out []server.CardActionKind
	for _, a := range r.Actions {
		out = append(out, a.Kind)
	}
	return out
}

func TestEpicList_DesignCards(t *testing.T) {
	cfg := t.TempDir()
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	four := map[string]string{
		"01-a.md": issueMD("01", "done", "", "A"), "02-b.md": issueMD("02", "done", "", "B"),
		"03-c.md": issueMD("03", "open", "", "C"), "04-d.md": issueMD("04", "open", "", "D"),
	}
	mapMD := func(front string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" }
	unconfirmed := "design_step: chart\ndestination_unconfirmed: true\n"

	store := t.TempDir()
	writeEpicFiles(t, store, "d-idle", mapMD(unconfirmed), open)
	writeEpicFiles(t, store, "d-live", mapMD(unconfirmed), open)
	writeEpicFiles(t, store, "d-wait", mapMD(unconfirmed), open)
	writeEpicFiles(t, store, "d-stopped", mapMD("design_step: spec\ncreated_at: 2026-01-01T10:00:00Z\n"), open)
	writeEpicFiles(t, store, "d-pct", mapMD("design_step: resolve-tickets\n"), four)
	writeEpicFiles(t, store, "d-done", "---\nstatus: done\nkind: map\ndesign_step: done\n---\n", map[string]string{"01-a.md": issueMD("01", "done", "", "A")})

	f := startDesignsIn(t, store, servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg))
	startDesign := func(ticket string) (id, conv string) {
		res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		conv, _ = readMeta(t, f.h, res.Session.ID)["conversation_id"].(string)
		return res.Session.ID, conv
	}
	startDesign("proj:d-live/01")
	id, conv := startDesign("proj:d-wait")
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which store?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })

	got := epicRows(t, f.h, f)
	for _, w := range []struct {
		name    string
		section server.CardSection
		status  string
		pill    string
		actions []server.CardActionKind
	}{
		{"d-idle", server.SectionReadyToDesign, "Not started", "", []server.CardActionKind{server.CardActionDesign}},
		{"d-live", server.SectionInProgress, "Designing", "", []server.CardActionKind{server.CardActionOpen}},
		{"d-wait", server.SectionNeedsYou, "Designing", server.PillQuestion, []server.CardActionKind{server.CardActionAnswer}},
		{"d-stopped", server.SectionInProgress, "Designing", "", []server.CardActionKind{server.CardActionContinue}},
		{"d-done", server.SectionFinished, "Designed", "", nil},
	} {
		r := got[w.name]
		if r.CardKind != server.CardKindDesign || r.Section != w.section || r.StatusWords != w.status || r.StatePill != w.pill {
			t.Errorf("%s: kind %q section %q status %q pill %q, want design/%q/%q/%q", w.name, r.CardKind, r.Section, r.StatusWords, r.StatePill, w.section, w.status, w.pill)
		}
		if k := actionKinds(r); len(k) != len(w.actions) || (len(k) == 1 && k[0] != w.actions[0]) {
			t.Errorf("%s: actions = %v, want %v", w.name, k, w.actions)
		}
		if len(r.Actions) == 1 && r.Actions[0].Target != "proj:"+w.name {
			t.Errorf("%s: action target = %q", w.name, r.Actions[0].Target)
		}
	}
	if r := got["d-wait"]; r.WaitingSince.IsZero() || r.WaitingSince.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("d-wait waiting since = %v, want the question time", r.WaitingSince)
	}
	if r := got["d-wait"]; r.TimeKind != server.CardTimeWaiting || !r.TimeStart.Equal(r.WaitingSince) {
		t.Errorf("d-wait time = %q %v, want waiting since %v", r.TimeKind, r.TimeStart, r.WaitingSince)
	}
	// The stamp reaches the epic index through the file watcher, a moment after the session starts.
	waitForEpicRow(t, f, "d-live", func(r server.EpicListRow) bool {
		return r.TimeKind == server.CardTimeElapsed && !r.TimeStart.IsZero()
	})
	if r := got["d-pct"]; r.RingProgress != 30 {
		t.Errorf("d-pct ring = %d, want 30 (3 of 10)", r.RingProgress)
	}
}

func TestEpicList_DesignCountdownCarriesItsDeadline(t *testing.T) {
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	writeAssistantMessage(t, f.path, runEndText)
	endTurn(t, f)
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	list, err := f.h.Client.Epics(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list.Epics {
		if r.Name != "map" {
			continue
		}
		if r.Section != server.SectionInProgress || len(r.Actions) != 0 || r.CountdownEndsAt.IsZero() {
			t.Errorf("map row = section %q actions %v countdown %v, want In progress, no button, a deadline", r.Section, r.Actions, r.CountdownEndsAt)
		}
		return
	}
	t.Error("map row is missing")
}

func TestEpicList_NewDesignLifecycle(t *testing.T) {
	cfg := t.TempDir()
	f := startDesigns(t, servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg))
	res := f.openNew(t, "fresh-idea", "an idea")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	id := res.Session.ID

	r := waitForEpicRow(t, f, "fresh-idea", func(r server.EpicListRow) bool { return r.Section == server.SectionInProgress })
	if r.Address != "proj:fresh-idea" || r.StatusWords != "Designing" {
		t.Fatalf("running row = %+v, want an In progress Designing card", r)
	}

	conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which name?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })
	if r := epicRows(t, f.h, f)["fresh-idea"]; r.Section != server.SectionNeedsYou || r.StatePill != server.PillQuestion {
		t.Errorf("waiting row = section %q pill %q, want Needs you / Question", r.Section, r.StatePill)
	}

}

func TestEpicList_NewDesignGoesToReadyToDesignWhenItsSessionEndsUncharted(t *testing.T) {
	f := startDesigns(t)
	res := f.openNew(t, "fresh-idea", "an idea")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if _, err := f.h.Client.StopSession(f.ctx, res.Session.ID); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, res.Session.ID, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if r := waitForEpicRow(t, f, "fresh-idea", func(r server.EpicListRow) bool { return r.Section == server.SectionReadyToDesign }); r.StatusWords != "Not started" {
		t.Errorf("row after the session ended = %+v, want Ready to design / Not started", r)
	}
}
