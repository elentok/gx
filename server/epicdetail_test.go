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
			tk.Status, tk.PauseKind = schema.StatusLegacyAnswer, schema.PauseKindSelfReported
		}); err != nil {
			t.Fatal(err)
		}
		question = "Which base?"
		if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "agent", Summary: question, Handoff: "h"}); err != nil {
			t.Fatal(err)
		}
	})
	code, d, body := getEpicDetail(t, h, base, "proj:work")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if d.Address != "proj:work" || d.Kind != server.EpicKindLoop || d.State != server.EpicStatePaused || d.Summary != "Does the thing." {
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
	if r := rowOf(t, d, "proj:work/06"); !r.Asking || r.Question != question {
		t.Errorf("row 06 = %+v, want Asking with the question", r)
	}
	if r := rowOf(t, d, "proj:work/04"); r.Asking || r.Question != "" {
		t.Errorf("row 04 = %+v, want no answer flag", r)
	}
	if r := rowOf(t, d, "proj:work/06"); r.Reason == "" {
		t.Errorf("row 06 = %+v, want a reason label on a paused ticket", r)
	}
	if r := rowOf(t, d, "proj:work/04"); r.Reason != "" {
		t.Errorf("row 04 reason = %q, want none on an unpaused ticket", r.Reason)
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
	if code != http.StatusOK || d.State != server.EpicStatePaused ||
		d.MergeBlockKind != "self-reported" || d.MergeBlockReason != "conflict in main.go" {
		t.Errorf("blocked = %d, %+v (%s)", code, d, body)
	}
	_, d, body = getEpicDetail(t, h, base, "proj:unmerged")
	if d.MergeBlockKind != "" || d.MergeBlockReason != "" {
		t.Errorf("an unblocked epic carries a merge block: %s", body)
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
		writeEpicFiles(t, store, "follow-ups", "---\nstatus: open\n---\n", open)
		writeEpicFiles(t, store, "follow-ups-archived", "---\nstatus: open\n---\n", open)
		writeEpicFiles(t, store, "drafting", "---\nstatus: draft\n---\n", open)
		writeEpicFiles(t, store, "cancelled", "---\nstatus: cancelled\n---\n", open)
	})
	if res, err := h.Client.QueueAdd(t.Context(), "proj:queued", ""); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	for epic, want := range map[string]bool{
		"ready": true, "queued": false, "merged": false, "noopen": false, "atlas": false,
		"follow-ups": false, "follow-ups-archived": false, "drafting": false, "cancelled": false,
	} {
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
				"03-c.md": issueMD("03", "open", "", "C"),
			})
		path := filepath.Join(store, "proj", "atlas", "issues", "03-c.md")
		if err := schema.UpdateTicket(path, func(tk *schema.Ticket) {
			tk.Status, tk.PauseKind = schema.StatusLegacyAnswer, schema.PauseKindSelfReported
		}); err != nil {
			t.Fatal(err)
		}
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
	if r := rowOf(t, d, "proj:atlas/01"); r.Title != "A" || r.RenderedStatus != "done" || r.Type != "implement" || r.Elapsed != 0 || r.Cost != 0 {
		t.Errorf("row 01 = %+v, want title, status and type only", r)
	}
	if r := rowOf(t, d, "proj:atlas/02"); r.Blockers != nil || r.Asking {
		t.Errorf("row 02 = %+v, want no blockers (01 is done) and no answer flag", r)
	}
	if r := rowOf(t, d, "proj:atlas/03"); r.Reason != "" {
		t.Errorf("row 03 reason = %q, want none on a map epic row", r.Reason)
	}
}

func TestEpicDetail_MapRowsCarryRedesignFields(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "atlas", "---\nstatus: open\nkind: map\n---\n\n## Destination\n\nReach the sea.\n",
			map[string]string{
				"01-a.md": issueMD("01", "done", "claimed_at: 2026-01-01T10:00:00Z\ndone_at: 2026-01-01T10:05:00Z\n", "A"),
				"02-b.md": issueMD("02", "done", "done_at: 2026-01-01T11:00:00Z\n", "B"),
				"03-c.md": issueMD("03", "claimed", "claimed_at: 2026-01-01T12:00:00Z\n", "C"),
				"04-d.md": issueMD("04", "open", "blocked_by: [\"03\"]\n", "D"),
				"05-e.md": issueMD("05", "cancelled", "cancelled_at: 2026-01-01T13:00:00Z\n", "E"),
			})
	})
	_, d, body := getEpicDetail(t, h, base, "proj:atlas")
	r1 := rowOf(t, d, "proj:atlas/01")
	if r1.Elapsed != 300 || r1.ClosedAt.Format("15:04") != "10:05" {
		t.Errorf("row 01 elapsed/closed = %d/%v, want 300 and 10:05: %s", r1.Elapsed, r1.ClosedAt, body)
	}
	if r2 := rowOf(t, d, "proj:atlas/02"); r2.Elapsed != 0 || r2.ClosedAt.IsZero() {
		t.Errorf("row 02 elapsed/closed = %d/%v, want no elapsed and a close time", r2.Elapsed, r2.ClosedAt)
	}
	r3 := rowOf(t, d, "proj:atlas/03")
	if r3.ClaimedAt.IsZero() || !r3.ClosedAt.IsZero() {
		t.Errorf("row 03 = %+v, want claimed_at and no close time", r3)
	}
	if r4 := rowOf(t, d, "proj:atlas/04"); !slices.Equal(r4.Blockers, []string{"03"}) || !r4.ClaimedAt.IsZero() {
		t.Errorf("row 04 = %+v, want blocker 03 and no claimed_at", r4)
	}
	if r5 := rowOf(t, d, "proj:atlas/05"); r5.ClosedAt.Format("15:04") != "13:00" {
		t.Errorf("row 05 closed = %v, want 13:00", r5.ClosedAt)
	}
}

func TestEpicDetail_LoopRowsCarryCloseTime(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "done_at: 2026-01-01T10:05:00Z\n", "A"),
			"02-b.md": issueMD("02", "cancelled", "cancelled_at: 2026-01-01T13:00:00Z\n", "B"),
			"03-c.md": issueMD("03", "open", "", "C"),
		})
	})
	_, d, _ := getEpicDetail(t, h, base, "proj:work")
	if got := rowOf(t, d, "proj:work/01").ClosedAt.Format("15:04"); got != "10:05" {
		t.Errorf("done closed = %s, want 10:05", got)
	}
	if got := rowOf(t, d, "proj:work/02").ClosedAt.Format("15:04"); got != "13:00" {
		t.Errorf("cancelled closed = %s, want 13:00", got)
	}
	if !rowOf(t, d, "proj:work/03").ClosedAt.IsZero() {
		t.Error("open row has a close time")
	}
}

func TestEpicDetail_MapRingProgress(t *testing.T) {
	cases := []struct {
		name, epic string
		check      func(t *testing.T, ring int)
	}{
		{"mid-way", "---\nstatus: open\nkind: map\n---\n", func(t *testing.T, ring int) {
			if ring <= 0 || ring >= 100 {
				t.Errorf("ring = %d, want strictly between 0 and 100", ring)
			}
		}},
		{"done", "---\nstatus: done\nkind: map\ncompleted_at: 2026-01-01T00:00:00Z\n---\n", func(t *testing.T, ring int) {
			if ring != 100 {
				t.Errorf("ring = %d, want 100", ring)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, base := startEpicDetail(t, func(store string) {
				writeEpicFiles(t, store, "atlas", c.epic, map[string]string{
					"01-a.md": issueMD("01", "done", "", "A"),
					"02-b.md": issueMD("02", "open", "", "B"),
				})
			})
			_, d, body := getEpicDetail(t, h, base, "proj:atlas")
			c.check(t, d.RingProgress)
			if !strings.Contains(body, `"ring_progress"`) {
				t.Errorf("body has no ring_progress: %s", body)
			}
		})
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

const stampedEpicFrontmatter = `status: open
created_at: 2026-01-01T00:00:00Z
merged_at: 2026-01-01T01:00:00Z
lines_added: 120
lines_removed: 30
total_tokens: 4500
actual_cost: 1.25
friction:
  pauses_by_reason:
    ask: 2
  run_events:
    stall: 3
  pause_endings:
    web: 1
    tui: 3
  total_paused_secs: 90
`

func TestEpicDetail_MergedLoopEpicCarriesEveryStamp(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\n"+stampedEpicFrontmatter+"---\n", map[string]string{"01-a.md": issueMD("01", "done", "", "A")})
	})
	code, d, body := getEpicDetail(t, h, base, "proj:work")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	st := d.Stats
	if st.LinesAdded == nil || *st.LinesAdded != 120 || st.LinesRemoved == nil || *st.LinesRemoved != 30 {
		t.Errorf("lines = %v/%v, want 120/30", st.LinesAdded, st.LinesRemoved)
	}
	if st.TotalTokens == nil || *st.TotalTokens != 4500 || st.Cost == nil || *st.Cost != 1.25 {
		t.Errorf("tokens/cost = %v/%v, want 4500/1.25", st.TotalTokens, st.Cost)
	}
	if st.LeadTime == nil || *st.LeadTime != 3600 || st.LeadTimeSoFar {
		t.Errorf("lead time = %v so far %v, want 3600 final", st.LeadTime, st.LeadTimeSoFar)
	}
	f := st.Friction
	if f == nil || f.PausesByReason["ask"] != 2 || f.RunEvents["stall"] != 3 || f.PausedSecs != 90 {
		t.Fatalf("friction = %+v", f)
	}
	if f.WebShare == nil || *f.WebShare != 0.25 {
		t.Errorf("web share = %v, want 0.25", f.WebShare)
	}
}

func TestEpicDetail_WebShareCountsPersonEndingsOnly(t *testing.T) {
	cases := []struct {
		name    string
		endings string
		want    float64
		hasWant bool
	}{
		{"only unknown and gx", "    unknown: 4\n    gx: 2\n", 0, false},
		{"web and tui ignore gx", "    web: 1\n    tui: 3\n    gx: 5\n    unknown: 7\n", 0.25, true},
		{"cli counts", "    web: 1\n    cli: 1\n", 0.5, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fm := "status: open\ncreated_at: 2026-01-01T00:00:00Z\nmerged_at: 2026-01-01T01:00:00Z\nfriction:\n  total_paused_secs: 1\n  pause_endings:\n" + c.endings
			h, base := startEpicDetail(t, func(store string) {
				writeEpicFiles(t, store, "work", "---\n"+fm+"---\n", map[string]string{"01-a.md": issueMD("01", "done", "", "A")})
			})
			code, d, body := getEpicDetail(t, h, base, "proj:work")
			if code != http.StatusOK {
				t.Fatalf("status %d: %s", code, body)
			}
			f := d.Stats.Friction
			if f == nil {
				t.Fatalf("no friction: %s", body)
			}
			switch {
			case !c.hasWant && f.WebShare != nil:
				t.Errorf("web share = %v, want none", *f.WebShare)
			case c.hasWant && (f.WebShare == nil || *f.WebShare != c.want):
				t.Errorf("web share = %v, want %v", f.WebShare, c.want)
			}
		})
	}
}

func TestEpicDetail_UnmergedLoopEpicShowsOnlyLeadTimeSoFar(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\ncreated_at: 2026-01-01T00:00:00Z\n---\n", map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	})
	_, d, _ := getEpicDetail(t, h, base, "proj:work")
	st := d.Stats
	if st.LeadTime == nil || *st.LeadTime <= 0 || !st.LeadTimeSoFar {
		t.Errorf("lead time = %v so far %v, want running", st.LeadTime, st.LeadTimeSoFar)
	}
	if st.LinesAdded != nil || st.LinesRemoved != nil || st.TotalTokens != nil || st.Cost != nil || st.Friction != nil {
		t.Errorf("stats = %+v, want only lead time", st)
	}
}

func TestEpicDetail_CodexDropsCost(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\n"+stampedEpicFrontmatter+"had_codex: true\n---\n", map[string]string{"01-a.md": issueMD("01", "done", "", "A")})
	})
	_, d, _ := getEpicDetail(t, h, base, "proj:work")
	if d.Stats.Cost != nil || d.Stats.TotalTokens == nil {
		t.Errorf("cost/tokens = %v/%v, want no cost and tokens kept", d.Stats.Cost, d.Stats.TotalTokens)
	}
}

func TestEpicDetail_MapEpicStats(t *testing.T) {
	stamped := "---\nstatus: open\nkind: map\ncreated_at: 2026-01-01T00:00:00Z\ncompleted_at: 2026-01-01T02:00:00Z\ntotal_tokens: 900\nactual_cost: 0.5\n---\n"
	unstamped := "---\nstatus: open\nkind: map\ncreated_at: 2026-01-01T00:00:00Z\n---\n"
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "atlas", stamped, map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
		writeEpicFiles(t, store, "bare", unstamped, map[string]string{"01-a.md": issueMD("01", "open", "", "A")})
	})
	_, d, _ := getEpicDetail(t, h, base, "proj:atlas")
	st := d.Stats
	if st.TotalTokens == nil || *st.TotalTokens != 900 || st.Cost == nil || *st.Cost != 0.5 {
		t.Errorf("tokens/cost = %v/%v, want 900/0.5", st.TotalTokens, st.Cost)
	}
	if st.LeadTime == nil || *st.LeadTime != 7200 || st.LeadTimeSoFar || st.Friction != nil {
		t.Errorf("stats = %+v, want 7200 final and no friction", st)
	}
	_, d, _ = getEpicDetail(t, h, base, "proj:bare")
	st = d.Stats
	if st.TotalTokens != nil || st.Cost != nil || st.LeadTime == nil || !st.LeadTimeSoFar {
		t.Errorf("stats = %+v, want only lead time so far", st)
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

func TestEpicDetail_NeedsRepairRowCarriesTheStateToken(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		servertest.WriteTicketWith(t, store, "proj", "work", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
		servertest.WriteTicketWith(t, store, "proj", "work", "02", "fine", servertest.TicketOpts{})
	})
	_, d, body := getEpicDetail(t, h, base, "proj:work")
	if len(d.Rows) != 2 {
		t.Fatalf("rows = %s", body)
	}
	for _, r := range d.Rows {
		want := ""
		if strings.HasSuffix(r.Address, "/01") {
			want = repairToken
		}
		if r.State != want {
			t.Errorf("%s state = %q, want %q", r.Address, r.State, want)
		}
	}
}
