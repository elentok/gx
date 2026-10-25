package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// writeQuestionLog writes a ticket log holding one open question asked at ts.
func writeQuestionLog(t *testing.T, store, epic, file, ts, summary string) {
	t.Helper()
	line := `{"seq":1,"ts":"` + ts + `","type":"pause.question","by":"agent","summary":"` + summary + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(store, "proj", epic, "issues", file), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEpicList_NeedsYouReasons(t *testing.T) {
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	asking := map[string]string{"01-a.md": issueMD("01", "paused", "pause_reason: self-reported\n", "A")}
	repair := map[string]string{"01-a.md": issueMD("01", "paused", "pause_reason: zero-commit\n", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	cycle := map[string]string{
		"01-a.md": issueMD("01", "cancelled", "", "A"),
		"02-b.md": issueMD("02", "open", "parent: \"01\"\n", "B"),
	}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "asking", loop, asking)
		writeQuestionLog(t, store, "asking", "01-a.jsonl", "2026-03-01T10:00:00Z", "Which store?")
		writeEpicFiles(t, store, "repair", loop, repair)
		writeEpicFiles(t, store, "cycle", loop, cycle)
		writeEpicFiles(t, store, "blocked", "---\nstatus: open\ncompleted_at: 2026-01-01T11:00:00Z\nland_block_kind: self-reported\nland_block_reason: conflict\n---\n", done)
		writeEpicFiles(t, store, "awaiting", loop, done)
		writeEpicFiles(t, store, "ready", loop, map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
		writeProjectEpic(t, store, "auto", "awaiting", loop, done)
		if err := servertest.WriteProjectRepo(store, "auto", t.TempDir(), `,"auto-ff-merge":true`); err != nil {
			t.Fatal(err)
		}
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Address] = r
	}

	want := map[string]struct {
		section server.CardSection
		pill    string
		reason  string
		action  server.CardAction
	}{
		"proj:asking": {server.SectionNeedsYou, "Question", "01 A: Which store?",
			server.CardAction{Kind: server.CardActionAnswer, Label: "Answer", Target: "proj:asking/01"}},
		"proj:repair": {server.SectionNeedsYou, "Paused", "01 A: ",
			server.CardAction{Kind: server.CardActionOpenTicket, Label: "Open ticket", Target: "proj:repair/01"}},
		"proj:cycle": {server.SectionNeedsYou, "Deadlocked", "proj:cycle/02 waits on proj:cycle/01",
			server.CardAction{Kind: server.CardActionOpenEpic, Label: "Open epic", Target: "proj:cycle"}},
		"proj:blocked": {server.SectionNeedsYou, "Merge blocked", "conflict",
			server.CardAction{Kind: server.CardActionMerge, Label: "Merge", Target: "proj:blocked"}},
		"proj:awaiting": {server.SectionNeedsYou, "Awaiting merge", "Auto-merge is off for proj",
			server.CardAction{Kind: server.CardActionMerge, Label: "Merge", Target: "proj:awaiting"}},
		"auto:awaiting": {server.SectionInProgress, "", "", server.CardAction{}},
		"proj:ready":    {server.SectionReadyToQueue, "", "", server.CardAction{}},
	}
	for addr, w := range want {
		r, ok := got[addr]
		if !ok {
			t.Errorf("%s is missing from %s", addr, body)
			continue
		}
		if r.Section != w.section || r.StatePill != w.pill || !strings.Contains(r.Reason, w.reason) || (w.pill != "" && r.Reason == "") {
			t.Errorf("%s = section %q pill %q reason %q, want %q %q containing %q", addr, r.Section, r.StatePill, r.Reason, w.section, w.pill, w.reason)
		}
		if w.pill != "" && !slices.Equal(r.Actions, []server.CardAction{w.action}) {
			t.Errorf("%s actions = %v, want [%v]", addr, r.Actions, w.action)
		}
	}
	if r := got["proj:asking"]; r.WaitingSince.Format("2006-01-02") != "2026-03-01" {
		t.Errorf("asking waits since %v, want the question time", r.WaitingSince)
	}
	if r := got["proj:blocked"]; r.WaitingSince.Format("2006-01-02") != "2026-01-01" {
		t.Errorf("blocked waits since %v, want completed_at", r.WaitingSince)
	}
}

func TestEpicList_NeedsYouSortsOldestWaitFirst(t *testing.T) {
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	ask := map[string]string{"01-a.md": issueMD("01", "paused", "pause_reason: self-reported\n", "A")}
	h, base := startEpicDetail(t, func(store string) {
		for epic, ts := range map[string]string{
			"middle": "2026-02-01T10:00:00Z", "newest": "2026-03-01T10:00:00Z", "oldest": "2026-01-01T10:00:00Z",
		} {
			writeEpicFiles(t, store, epic, loop, ask)
			writeQuestionLog(t, store, epic, "01-a.jsonl", ts, "q")
		}
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	var names []string
	for _, r := range getEpicList(t, code, body).Epics {
		if r.Section == server.SectionNeedsYou {
			names = append(names, r.Name)
		}
	}
	if want := []string{"oldest", "middle", "newest"}; !slices.Equal(names, want) {
		t.Errorf("needs-you order = %v, want %v", names, want)
	}
}

func TestEpicList_SeveralWaitingTicketsNameTheOldest(t *testing.T) {
	loop := "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "many", loop, map[string]string{
			"01-a.md": issueMD("01", "paused", "pause_reason: self-reported\n", "A"),
			"02-b.md": issueMD("02", "paused", "pause_reason: self-reported\n", "B"),
		})
		writeQuestionLog(t, store, "many", "01-a.jsonl", "2026-03-01T10:00:00Z", "later")
		writeQuestionLog(t, store, "many", "02-b.jsonl", "2026-02-01T10:00:00Z", "earlier")
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	rows := getEpicList(t, code, body).Epics
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	r := rows[0]
	if r.Reason != "02 B: earlier (+1 more)" || len(r.Actions) != 1 || r.Actions[0].Target != "proj:many/02" {
		t.Errorf("row = reason %q actions %v, want the 02 question with one more", r.Reason, r.Actions)
	}
}

func TestEpicList_PermissionWaitingIsNeedsYou(t *testing.T) {
	h, _ := startBlockedClaimed(t, false, &agentrunner.Pending{
		ID: "req-1", Tool: "Bash", Input: json.RawMessage(`{"command":"ls"}`), Description: "List files",
	})
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	code, body := webDo(t, h, http.MethodGet, hs.WebURL+"/v1/epics")
	var r server.EpicListRow
	for _, row := range getEpicList(t, code, body).Epics {
		if row.Address == "proj:epic-a" {
			r = row
		}
	}
	want := server.CardAction{Kind: server.CardActionAnswer, Label: "Answer", Target: answerAddr}
	if r.Section != server.SectionNeedsYou || r.StatePill != "Permission" || !strings.Contains(r.Reason, "List files") || !slices.Equal(r.Actions, []server.CardAction{want}) {
		t.Errorf("row = section %q pill %q reason %q actions %v", r.Section, r.StatePill, r.Reason, r.Actions)
	}
}
