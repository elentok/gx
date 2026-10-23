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
)

func fetchDetail(t *testing.T, h *servertest.Harness, base, address string) server.TicketDetail {
	t.Helper()
	code, body := webDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address="+address)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var d server.TicketDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return d
}

// withRepo gives the project a repo; findTicket refuses a project without one.
func withRepo(t *testing.T, store string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj","repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func issueWithBody(id, status, extra, title, body string) string {
	return issueMD(id, status, extra, title) + "\n" + body
}

func TestTicketDetail_LoopTicketPayload(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "", "Own blocker"),
			"02-b.md": issueMD("02", "done", "", "The parent"),
			"03-c.md": issueWithBody("03", "open", "parent: \"02\"\nblocked_by: [\"01\", \"other/06\"]\n", "The ticket",
				"## Description\n\nDo it.\n\n## Comments\n\nA note.\n"),
			"04-d.md": issueMD("04", "open", "parent: \"03\"\n", "A fork"),
		})
		writeEpicFiles(t, store, "other", "---\nstatus: open\n---\n", map[string]string{
			"06-x.md": issueMD("06", "claimed", "", "Cross blocker"),
		})
		withRepo(t, store)
	})
	d := fetchDetail(t, h, base, "proj:work/03")
	if d.Type != "implement" || d.Epic != "proj:work" || d.MapEpic {
		t.Fatalf("header = %q %q %v", d.Type, d.Epic, d.MapEpic)
	}
	for _, want := range []string{"## Description", "## Comments", "A note."} {
		if !strings.Contains(d.Body, want) {
			t.Fatalf("body lacks %q: %q", want, d.Body)
		}
	}
	want := []server.RelatedTicket{
		{Relation: server.RelationBlocker, Address: "proj:work/01", Linkable: true, Title: "Own blocker", Status: "done"},
		{Relation: server.RelationBlocker, Address: "proj:other/06", Linkable: true, Title: "Cross blocker", Status: "claimed"},
		{Relation: server.RelationParent, Address: "proj:work/02", Linkable: true, Title: "The parent", Status: "waiting-for-children"},
		{Relation: server.RelationFork, Address: "proj:work/04", Linkable: true, Title: "A fork", Status: "blocked"},
	}
	if !slices.Equal(d.Related, want) {
		t.Fatalf("related = %+v, want %+v", d.Related, want)
	}
	if d.Stats == nil {
		t.Fatal("loop ticket has no stats")
	}
}

func TestTicketDetail_PausedTicketCarriesReason(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueWithBody("01", "paused", "pause_reason: merge-conflict\n", "A", "## Paused\n\nConflict in main.go.\n"),
		})
		withRepo(t, store)
	})
	d := fetchDetail(t, h, base, "proj:work/01")
	if d.PauseReason != "merge-conflict" || d.PauseLabel != "Merge conflict" || d.PauseText != "Conflict in main.go." {
		t.Fatalf("pause = %q %q %q", d.PauseReason, d.PauseLabel, d.PauseText)
	}
}

func TestTicketDetail_MapTicketHasNoStats(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "atlas", "---\nstatus: open\nkind: map\n---\n", map[string]string{
			"01-q.md": issueWithBody("01", "done", "", "Which way?", "## Resolution\n\nWest.\n\n## Question\n\nWhere to?\n"),
		})
		withRepo(t, store)
	})
	d := fetchDetail(t, h, base, "proj:atlas/01")
	if !d.MapEpic || d.Stats != nil {
		t.Fatalf("map = %v, stats = %+v", d.MapEpic, d.Stats)
	}
	if !strings.Contains(d.Body, "## Resolution") || !strings.Contains(d.Body, "## Question") {
		t.Fatalf("body = %q", d.Body)
	}
}

func TestTicketDetail_CostAndContextAppearOnceMerged(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": issueMD("01", "done", "elapsed_time: 60\nactual_cost: 1.5\nactual_context_window: 90000\n", "Merged"),
			"02-b.md": issueMD("02", "open", "", "Unmerged"),
		})
		withRepo(t, store)
	})
	merged := fetchDetail(t, h, base, "proj:work/01").Stats
	if merged == nil || merged.Elapsed != 60 || merged.Cost != 1.5 || merged.ContextWindow != 90000 {
		t.Fatalf("merged stats = %+v", merged)
	}
	open := fetchDetail(t, h, base, "proj:work/02").Stats
	if open == nil || open.Cost != 0 || open.ContextWindow != 0 {
		t.Fatalf("unmerged stats = %+v", open)
	}
}
