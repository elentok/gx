package server_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// writeEpicFiles writes <store>/proj/<epic>/ticket.md and one issue file per
// entry of issues (file name to whole file content).
func writeEpicFiles(t *testing.T, store, epic, ticketMD string, issues map[string]string) {
	t.Helper()
	dir := filepath.Join(store, "proj", epic)
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte(ticketMD), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range issues {
		if err := os.WriteFile(filepath.Join(dir, "issues", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func issueMD(id, status, extra, title string) string {
	return "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: implement\n" + extra + "---\n\n# " + title + "\n"
}

func startEpicDetail(t *testing.T, setup func(store string)) (*servertest.Harness, string) {
	t.Helper()
	store := t.TempDir()
	setup(store)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return h, hs.WebURL
}

func getEpicDetail(t *testing.T, h *servertest.Harness, base, address string) (int, server.EpicDetail, string) {
	t.Helper()
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics/detail?address="+address)
	var d server.EpicDetail
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &d); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
	}
	return code, d, body
}

func rowOf(t *testing.T, d server.EpicDetail, address string) server.EpicDetailRow {
	t.Helper()
	for _, r := range d.Rows {
		if r.Address == address {
			return r
		}
	}
	t.Fatalf("no row %s in %+v", address, d.Rows)
	return server.EpicDetailRow{}
}

func TestEpicDetail_LoopEpic(t *testing.T) {
	var question string
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work",
			"---\nstatus: open\nstarted_at: 2026-01-01T10:00:00Z\ncompleted_at: 2026-01-01T11:00:00Z\n---\n\n## Summary\n\nDoes the thing.\n",
			map[string]string{
				"01-a.md": issueMD("01", "done", "elapsed_time: 60\nactual_cost: 1.5\n", "A"),
				"02-b.md": issueMD("02", "done", "parent: \"01\"\nelapsed_time: 30\nactual_cost: 0.5\n", "B"),
				"03-c.md": issueMD("03", "cancelled", "", "C"),
				"04-d.md": issueMD("04", "open", "blocked_by: [\"other/06\"]\n", "D"),
				"05-e.md": issueMD("05", "open", "blocked_by: [\"04\"]\n", "E"),
				"06-f.md": issueMD("06", "open", "", "F"),
			})
		writeEpicFiles(t, store, "other", "---\nstatus: open\n---\n", map[string]string{
			"06-x.md": issueMD("06", "open", "", "X"),
		})
		path := filepath.Join(store, "proj", "work", "issues", "06-f.md")
		if err := schema.UpdateTicket(path, func(tk *schema.Ticket) {
			tk.Status, tk.ParkKind = schema.StatusNeedsAnswer, schema.ParkKindSelfReported
		}); err != nil {
			t.Fatal(err)
		}
		question = "Which base?"
		if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventParkQuestion, By: "agent", Summary: question, Handoff: "h"}); err != nil {
			t.Fatal(err)
		}
	})
	code, d, body := getEpicDetail(t, h, base, "proj:work")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if d.Address != "proj:work" || d.Kind != server.EpicKindLoop || d.State != server.EpicStateDeadlocked || d.Summary != "Does the thing." {
		t.Errorf("header = %+v", d)
	}
	if d.Done != 3 || d.Total != 6 {
		t.Errorf("done/total = %d/%d, want 3/6 (cancelled counts as done)", d.Done, d.Total)
	}
	if d.WallClock != 3600 || d.AgentTime != 90 || d.CostTotal != 2 {
		t.Errorf("wall_clock/agent_time/cost_total = %d/%d/%v, want 3600/90/2", d.WallClock, d.AgentTime, d.CostTotal)
	}
	if want := []string{"proj:work/01", "proj:work/02", "proj:work/03", "proj:work/04", "proj:work/05", "proj:work/06"}; !slices.Equal(rowAddresses(d), want) {
		t.Errorf("rows = %v, want ticket order %v", rowAddresses(d), want)
	}
	if r := rowOf(t, d, "proj:work/02"); r.Parent != "proj:work/01" || r.Elapsed != 30 || r.Cost != 0.5 || r.Type != "implement" || r.RenderedStatus != "done" {
		t.Errorf("row 02 = %+v", r)
	}
	if r := rowOf(t, d, "proj:work/03"); r.RenderedStatus != "cancelled" {
		t.Errorf("row 03 = %+v", r)
	}
	if r := rowOf(t, d, "proj:work/04"); !slices.Equal(r.Blockers, []string{"other/06"}) {
		t.Errorf("cross-epic blockers = %v, want [other/06]", r.Blockers)
	}
	if r := rowOf(t, d, "proj:work/05"); !slices.Equal(r.Blockers, []string{"04"}) {
		t.Errorf("own-epic blockers = %v, want [04]", r.Blockers)
	}
	if r := rowOf(t, d, "proj:work/06"); !r.NeedsAnswer || r.Question != question {
		t.Errorf("row 06 = %+v, want needs_answer with the question", r)
	}
	if r := rowOf(t, d, "proj:work/04"); r.NeedsAnswer || r.Question != "" {
		t.Errorf("row 04 = %+v, want no answer flag", r)
	}
	if strings.Contains(strings.ToLower(body), "token") || strings.Contains(body, "context_window") {
		t.Errorf("payload has token counts: %s", body)
	}
}

func rowAddresses(d server.EpicDetail) []string {
	out := make([]string, len(d.Rows))
	for i, r := range d.Rows {
		out[i] = r.Address
	}
	return out
}

func TestEpicDetail_FinishedVersusAwaitingMerge(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
		writeEpicFiles(t, store, "unmerged", "---\nstatus: done\ncompleted_at: 2026-01-01T11:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "merged", "---\nstatus: done\ncompleted_at: 2026-01-01T11:00:00Z\nmerged_at: 2026-01-02T11:00:00Z\n---\n", done)
	})
	for epic, want := range map[string]server.EpicState{"unmerged": server.EpicStateAwaitingMerge, "merged": server.EpicStateFinished} {
		code, d, body := getEpicDetail(t, h, base, "proj:"+epic)
		if code != http.StatusOK || d.State != want {
			t.Errorf("%s = %d, state %q, want %q (%s)", epic, code, d.State, want, body)
		}
	}
}

func TestEpicDetail_MergeBlockedCarriesKindAndReason(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
		writeEpicFiles(t, store, "blocked", "---\nstatus: done\ncompleted_at: 2026-01-01T11:00:00Z\nland_block_kind: self-reported\nland_block_reason: conflict in main.go\n---\n", done)
		writeEpicFiles(t, store, "unmerged", "---\nstatus: done\ncompleted_at: 2026-01-01T11:00:00Z\n---\n", done)
	})
	code, d, body := getEpicDetail(t, h, base, "proj:blocked")
	if code != http.StatusOK || d.State != server.EpicStateMergeBlocked ||
		d.LandBlockKind != "self-reported" || d.LandBlockReason != "conflict in main.go" {
		t.Errorf("blocked = %d, %+v (%s)", code, d, body)
	}
	_, d, body = getEpicDetail(t, h, base, "proj:unmerged")
	if d.LandBlockKind != "" || d.LandBlockReason != "" {
		t.Errorf("an unblocked epic carries a land block: %s", body)
	}
}

func TestEpicDetail_Queueable(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
		done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
		writeEpicFiles(t, store, "ready", "---\nstatus: open\n---\n", open)
		writeEpicFiles(t, store, "queued", "---\nstatus: open\n---\n", open)
		writeEpicFiles(t, store, "merged", "---\nstatus: open\nmerged_at: 2026-01-02T11:00:00Z\n---\n", open)
		writeEpicFiles(t, store, "noopen", "---\nstatus: open\n---\n", done)
		writeEpicFiles(t, store, "atlas", "---\nstatus: open\nkind: map\n---\n", open)
	})
	if res, err := h.Client.QueueAdd(t.Context(), "proj:queued", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	for epic, want := range map[string]bool{"ready": true, "queued": false, "merged": false, "noopen": false, "atlas": false} {
		code, d, body := getEpicDetail(t, h, base, "proj:"+epic)
		if code != http.StatusOK || d.Queueable != want {
			t.Errorf("%s = %d, queueable %v, want %v (%s)", epic, code, d.Queueable, want, body)
		}
		if wantQueued := epic == "queued"; d.Queued != wantQueued {
			t.Errorf("%s queued = %v, want %v", epic, d.Queued, wantQueued)
		}
	}
}

func TestEpicDetail_MapEpic(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "atlas", "---\nstatus: open\nkind: map\nstarted_at: 2026-01-01T10:00:00Z\n---\n\n## Destination\n\nReach the sea.\n",
			map[string]string{
				"01-a.md": issueMD("01", "done", "elapsed_time: 60\nactual_cost: 1.5\n", "A"),
				"02-b.md": issueMD("02", "open", "blocked_by: [\"01\"]\n", "B"),
			})
	})
	code, d, body := getEpicDetail(t, h, base, "proj:atlas")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if d.Kind != server.EpicKindMap || d.Summary != "Reach the sea." || d.State != "" {
		t.Errorf("header = %+v", d)
	}
	if d.WallClock != 0 || d.AgentTime != 0 || d.CostTotal != 0 {
		t.Errorf("totals = %d/%d/%v, want empty", d.WallClock, d.AgentTime, d.CostTotal)
	}
	if r := rowOf(t, d, "proj:atlas/01"); r.Title != "A" || r.RenderedStatus != "done" || r.Type != "" || r.Elapsed != 0 || r.Cost != 0 {
		t.Errorf("row 01 = %+v, want title and status only", r)
	}
	if r := rowOf(t, d, "proj:atlas/02"); r.Blockers != nil || r.NeedsAnswer {
		t.Errorf("row 02 = %+v, want no blockers and no answer flag", r)
	}
}

func TestEpicDetail_LoopEpicWithoutCostKeepsZeroTotals(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	})
	code, _, body := getEpicDetail(t, h, base, "proj:work")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	for _, want := range []string{`"agent_time":0`, `"cost_total":0`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestEpicDetail_UnknownAddress(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	})
	for _, address := range []string{"proj:nope", "nope:work", "nocolon", ""} {
		if code, _, body := getEpicDetail(t, h, base, address); code != http.StatusNotFound {
			t.Errorf("%s = %d (%s), want 404", address, code, body)
		}
	}
}
