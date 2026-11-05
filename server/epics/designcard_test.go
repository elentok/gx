package epics_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func epicRows(t *testing.T, h *servertest.Harness, f designsFixture) map[string]wire.EpicListRow {
	t.Helper()
	list, err := h.Client.Epics(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]wire.EpicListRow{}
	for _, r := range list.Epics {
		got[r.Name] = r
	}
	return got
}

// waitForEpicRow polls for the row of name: the epic index follows the store
// through a file watcher, so a map written just now shows up a moment later.
func waitForEpicRow(t *testing.T, f designsFixture, name string, ok func(wire.EpicListRow) bool) wire.EpicListRow {
	t.Helper()
	var r wire.EpicListRow
	if !servertest.Poll(5*time.Second, func() bool {
		var found bool
		r, found = epicRows(t, f.H, f)[name]
		return found && ok(r)
	}) {
		t.Fatalf("row %q = %+v, never matched", name, r)
	}
	return r
}

func actionKinds(r wire.EpicListRow) []wire.CardActionKind {
	var out []wire.CardActionKind
	for _, a := range r.Actions {
		out = append(out, a.Kind)
	}
	return out
}

func TestEpicList_DesignCards(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	open := map[string]string{"01-a.md": servertest.IssueMD("01", "open", "", "A")}
	four := map[string]string{
		"01-a.md": servertest.IssueMD("01", "done", "", "A"), "02-b.md": servertest.IssueMD("02", "done", "", "B"),
		"03-c.md": servertest.IssueMD("03", "open", "", "C"), "04-d.md": servertest.IssueMD("04", "open", "", "D"),
	}
	mapMD := func(front string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" }
	unconfirmed := "design_step: chart\ndestination_unconfirmed: true\n"

	store := t.TempDir()
	servertest.WriteEpicFiles(t, store, "d-idle", mapMD(unconfirmed), open)
	servertest.WriteEpicFiles(t, store, "d-live", mapMD(unconfirmed), open)
	servertest.WriteEpicFiles(t, store, "d-wait", mapMD(unconfirmed), open)
	servertest.WriteEpicFiles(t, store, "d-stopped", mapMD("design_step: spec\ncreated_at: 2026-01-01T10:00:00Z\n"), open)
	servertest.WriteEpicFiles(t, store, "d-pct", mapMD("design_step: resolve-tickets\n"), four)
	servertest.WriteEpicFiles(t, store, "d-done", "---\nstatus: done\nkind: map\ndesign_step: done\n---\n", map[string]string{"01-a.md": servertest.IssueMD("01", "done", "", "A")})

	f := startDesignsIn(t, store, servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg))
	startDesign := func(ticket string) (id, conv string) {
		res, err := f.H.Client.CreateSession(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		conv, _ = readMeta(t, f.H, res.Session.ID)["conversation_id"].(string)
		return res.Session.ID, conv
	}
	startDesign("proj:d-live/01")
	id, conv := startDesign("proj:d-wait")
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which store?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })

	got := epicRows(t, f.H, f)
	for _, w := range []struct {
		name    string
		section wire.CardSection
		status  string
		pill    string
		actions []wire.CardActionKind
	}{
		{"d-idle", wire.SectionReadyToDesign, "Not started", "", []wire.CardActionKind{wire.CardActionDesign}},
		{"d-live", wire.SectionDesigning, "Designing", "", []wire.CardActionKind{wire.CardActionOpen}},
		{"d-wait", wire.SectionNeedsYou, "Designing", epics.PillQuestion, []wire.CardActionKind{wire.CardActionAnswer}},
		{"d-stopped", wire.SectionDesigning, "Designing", "", []wire.CardActionKind{wire.CardActionContinue}},
		{"d-done", wire.SectionFinished, "Designed", "", nil},
	} {
		r := got[w.name]
		if r.CardKind != wire.CardKindDesign || r.Section != w.section || r.StatusWords != w.status || r.StatePill != w.pill {
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
	if r := got["d-wait"]; r.TimeKind != wire.CardTimeWaiting || !r.TimeStart.Equal(r.WaitingSince) {
		t.Errorf("d-wait time = %q %v, want waiting since %v", r.TimeKind, r.TimeStart, r.WaitingSince)
	}
	// The stamp reaches the epic index through the file watcher, a moment after the session starts.
	waitForEpicRow(t, f, "d-live", func(r wire.EpicListRow) bool {
		return r.TimeKind == wire.CardTimeElapsed && !r.TimeStart.IsZero()
	})
	if r := got["d-pct"]; r.RingProgress != 30 {
		t.Errorf("d-pct ring = %d, want 30 (3 of 10)", r.RingProgress)
	}
}

func TestEpicList_DesignCountdownCarriesItsDeadline(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	list, err := f.H.Client.Epics(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list.Epics {
		if r.Name != "map" {
			continue
		}
		if r.Section != wire.SectionDesigning || len(r.Actions) != 0 || r.CountdownEndsAt.IsZero() {
			t.Errorf("map row = section %q actions %v countdown %v, want Designing, no button, a deadline", r.Section, r.Actions, r.CountdownEndsAt)
		}
		return
	}
	t.Error("map row is missing")
}

func TestEpicList_NewDesignLifecycle(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	f := startDesigns(t, servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg))
	res := f.OpenNew(t, "fresh-idea", "an idea")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	id := res.Session.ID

	r := waitForEpicRow(t, f, "fresh-idea", func(r wire.EpicListRow) bool { return r.Section == wire.SectionDesigning })
	if r.Address != "proj:fresh-idea" || r.StatusWords != "Designing" {
		t.Fatalf("running row = %+v, want a Designing card", r)
	}

	conv, _ := readMeta(t, f.H, id)["conversation_id"].(string)
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which name?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	if r := epicRows(t, f.H, f)["fresh-idea"]; r.Section != wire.SectionNeedsYou || r.StatePill != epics.PillQuestion {
		t.Errorf("waiting row = section %q pill %q, want Needs you / Question", r.Section, r.StatePill)
	}

}

func TestEpicList_NewDesignGoesToReadyToDesignWhenItsSessionEndsUncharted(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	res := f.OpenNew(t, "fresh-idea", "an idea")
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if _, err := f.H.Client.Sessions.Stop(f.Ctx, res.Session.ID); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.Ctx, t, f.H, f.Evs, res.Session.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if r := waitForEpicRow(t, f, "fresh-idea", func(r wire.EpicListRow) bool { return r.Section == wire.SectionReadyToDesign }); r.StatusWords != "Not started" {
		t.Errorf("row after the session ended = %+v, want Ready to design / Not started", r)
	}
}
