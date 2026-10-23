package server_test

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestEpicList_MapPickerSection(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	const day = 24 * time.Hour

	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	mapMD := func(front string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" }
	unconfirmed := "destination_unconfirmed: true\ncreated_at: " + ts(30*day) + "\n"
	implMD := func(front string) string {
		return "---\nstatus: open\ncreated_at: " + ts(20*day) + "\n" + front + "---\n"
	}

	store := t.TempDir()
	// Not started: unconfirmed Destination, no ticket resolved.
	writeEpicFiles(t, store, "m-idle", mapMD(unconfirmed), open)
	writeEpicFiles(t, store, "m-live", mapMD(unconfirmed), open)
	writeEpicFiles(t, store, "m-wait", mapMD(unconfirmed), open)
	// In progress: past Chart.
	writeEpicFiles(t, store, "m-resolving", mapMD("created_at: "+ts(30*day)+"\n"), open)
	// Impl epic created, not queued.
	writeEpicFiles(t, store, "m-ready", mapMD("completed_at: "+ts(20*day)+"\n"), done)
	writeEpicFiles(t, store, "m-ready-impl", implMD(""), open)
	// Impl queued inside and outside the window.
	writeEpicFiles(t, store, "m-recent", mapMD("completed_at: "+ts(20*day)+"\n"), done)
	writeEpicFiles(t, store, "m-recent-impl", implMD("queued_at: "+ts(6*day+23*time.Hour)+"\n"), open)
	writeEpicFiles(t, store, "m-old", mapMD("completed_at: "+ts(20*day)+"\n"), done)
	writeEpicFiles(t, store, "m-old-impl", implMD("queued_at: "+ts(8*day)+"\n"), open)
	// Finished by hand (no impl epic) inside and outside the window.
	writeEpicFiles(t, store, "m-hand-new", mapMD("completed_at: "+ts(2*day)+"\n"), done)
	writeEpicFiles(t, store, "m-hand-old", mapMD("completed_at: "+ts(8*day)+"\n"), done)

	// The Branch step is done once the impl branch exists.
	repo := testutil.TempRepo(t)
	for _, b := range []string{"m-ready-impl", "m-recent-impl", "m-old-impl"} {
		runGitIn(t, repo, "branch", b)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)

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
	startDesign := func(ticket string) (id, conv string) {
		res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		conv, _ = readMeta(t, h, res.Session.ID)["conversation_id"].(string)
		return res.Session.ID, conv
	}
	// A live session on one of the map's tickets.
	startDesign("proj:m-live/01")
	// A waiting grilling round.
	id, conv := startDesign("proj:m-wait")
	writeAssistantMessage(t, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), "❓ **Q1** Which store?")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })

	code, body := webDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Name] = r
	}

	within := func(a, b time.Time) bool { return a.Sub(b).Abs() < time.Minute }
	want := []struct {
		name    string
		section server.PickerSection
		sortAgo time.Duration
		impl    string
	}{
		{"m-idle", server.PickerNotStarted, 30 * day, ""},
		{"m-live", server.PickerInProgress, 30 * day, ""},
		{"m-wait", server.PickerInProgress, 30 * day, ""},
		{"m-resolving", server.PickerInProgress, 30 * day, ""},
		{"m-ready", server.PickerReadyToQueue, 20 * day, "proj:m-ready-impl"},
		{"m-recent", server.PickerRecentlyFinished, 6*day + 23*time.Hour, "proj:m-recent-impl"},
		{"m-old", server.PickerNone, 8 * day, "proj:m-old-impl"},
		{"m-hand-new", server.PickerRecentlyFinished, 2 * day, ""},
		{"m-hand-old", server.PickerNone, 8 * day, ""},
	}
	for _, w := range want {
		r, ok := got[w.name]
		if !ok {
			t.Errorf("%s is missing", w.name)
			continue
		}
		if r.PickerSection != w.section {
			t.Errorf("%s: section = %q, want %q", w.name, r.PickerSection, w.section)
		}
		// In progress sorts by latest activity, which falls back to creation here.
		if !within(r.PickerSortAt, now.Add(-w.sortAgo)) {
			t.Errorf("%s: sort time = %v, want about %v ago", w.name, r.PickerSortAt, w.sortAgo)
		}
		if r.ImplAddress != w.impl {
			t.Errorf("%s: impl address = %q, want %q", w.name, r.ImplAddress, w.impl)
		}
	}
}
