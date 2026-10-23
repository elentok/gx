package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

const (
	queuedAtLoop   = "---\nstatus: open\n---\n"
	queuedAtOpenMD = "01-a.md"
)

func epicTicketMD(t *testing.T, store, epic string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(store, "proj", epic, "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func queuedAtLine(md string) string {
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "queued_at:") {
			return line
		}
	}
	return ""
}

func TestQueueAdd_StampsQueuedAtOnceAndKeepsIt(t *testing.T) {
	open := map[string]string{queuedAtOpenMD: issueMD("01", "open", "", "A")}
	var storeDir string
	h, _ := startEpicDetail(t, func(store string) {
		storeDir = store
		writeEpicFiles(t, store, "e", queuedAtLoop, open)
	})
	if got := queuedAtLine(epicTicketMD(t, storeDir, "e")); got != "" {
		t.Fatalf("queued_at before enqueue = %q", got)
	}
	if res, err := h.Client.QueueAdd(t.Context(), "proj:e", ""); err != nil || res.Refused {
		t.Fatalf("add = %+v, %v", res, err)
	}
	first := queuedAtLine(epicTicketMD(t, storeDir, "e"))
	if first == "" {
		t.Fatal("first enqueue did not stamp queued_at")
	}
	if res, err := h.Client.QueueRemove(t.Context(), "proj:e"); err != nil || res.Refused {
		t.Fatalf("remove = %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(t.Context(), "proj:e", ""); err != nil || res.Refused {
		t.Fatalf("re-add = %+v, %v", res, err)
	}
	if got := queuedAtLine(epicTicketMD(t, storeDir, "e")); got != first {
		t.Errorf("queued_at after re-add = %q, want the first %q", got, first)
	}
}

func TestEpicList_QueuedAtFallsBackToStartedThenMerged(t *testing.T) {
	open := map[string]string{queuedAtOpenMD: issueMD("01", "open", "", "A")}
	done := map[string]string{queuedAtOpenMD: issueMD("01", "done", "", "A")}
	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "all", "---\nstatus: open\nqueued_at: 2026-01-01T00:00:00Z\nstarted_at: 2026-01-02T00:00:00Z\nmerged_at: 2026-01-03T00:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "started", "---\nstatus: open\nstarted_at: 2026-01-02T00:00:00Z\nmerged_at: 2026-01-03T00:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "merged", "---\nstatus: done\nmerged_at: 2026-01-03T00:00:00Z\n---\n", done)
		writeEpicFiles(t, store, "none", queuedAtLoop, open)
	})
	_ = h
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	list := getEpicList(t, code, body)
	want := map[string]string{
		"proj:all":     "2026-01-01T00:00:00Z",
		"proj:started": "2026-01-02T00:00:00Z",
		"proj:merged":  "2026-01-03T00:00:00Z",
		"proj:none":    "0001-01-01T00:00:00Z",
	}
	for _, row := range list.Epics {
		if got := row.QueuedAt.UTC().Format("2006-01-02T15:04:05Z"); got != want[row.Address] {
			t.Errorf("%s queued_at = %s, want %s", row.Address, got, want[row.Address])
		}
	}
}

func TestQueueAdd_DraftImplEpicRefusesEpicDraftAndWritesNothing(t *testing.T) {
	open := map[string]string{queuedAtOpenMD: issueMD("01", "open", "", "A")}
	var storeDir string
	h, _ := startEpicDetail(t, func(store string) {
		storeDir = store
		writeEpicFiles(t, store, "feature-impl", "---\nstatus: draft\n---\n", open)
	})
	res, err := h.Client.QueueAdd(t.Context(), "proj:feature-impl", "")
	if err != nil || !res.Refused || res.Reason != server.ReasonEpicDraft {
		t.Fatalf("add = %+v, %v; want refusal %s", res, err, server.ReasonEpicDraft)
	}
	if got := queuedAtLine(epicTicketMD(t, storeDir, "feature-impl")); got != "" {
		t.Errorf("refused add stamped %q", got)
	}
	if got := queueAddresses(t, h); len(got) != 0 {
		t.Errorf("queue = %v, want empty", got)
	}
}
