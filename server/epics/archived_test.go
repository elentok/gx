package epics_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func archivedLoopFile(extra string) string {
	return "---\nstatus: open\ncreated_at: 2026-01-01T10:00:00Z\nstarted_at: 2026-01-01T10:00:00Z\ncompleted_at: 2026-01-02T10:00:00Z\nmerged_at: 2026-01-02T10:00:00Z\n" + extra + "---\n\n# Title of it\n\n## Summary\n\nDid it.\n"
}

func archivedDesignFile(extra string) string {
	return "---\nstatus: open\nkind: map\ndesign_step: done\ncompleted_at: 2026-01-02T10:00:00Z\n" + extra + "---\n\n## Destination\n\nGo.\n"
}

func getArchivedList(t *testing.T, h *servertest.Harness, base string) wire.ArchivedEpicList {
	t.Helper()
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics/archived")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var l wire.ArchivedEpicList
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return l
}

func TestEpicDetail_ArchivedListOrdersNewestFirstAndJoinsAPair(t *testing.T) {
	t.Parallel()
	done := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, ".archive/old", archivedLoopFile("archived_at: 2026-02-01T10:00:00Z\n"), done)
		rigs.WriteEpicFiles(t, store, ".archive/nostamp", archivedLoopFile(""), done)
		rigs.WriteEpicFiles(t, store, ".archive/newer", archivedLoopFile("archived_at: 2026-03-01T10:00:00Z\n"), done)
		rigs.WriteEpicFiles(t, store, ".archive/design", archivedDesignFile("archived_at: 2026-02-15T10:00:00Z\n"), done)
		rigs.WriteEpicFiles(t, store, ".archive/design-impl", archivedLoopFile("archived_at: 2026-02-15T10:00:00Z\n"), done)
		rigs.WriteEpicFiles(t, store, "active", "---\nstatus: open\n---\n", done)
	})

	got := getArchivedList(t, h, base)
	var addrs []string
	for _, r := range got.Epics {
		addrs = append(addrs, r.Address)
	}
	if want := []string{"proj:newer", "proj:design", "proj:old", "proj:nostamp"}; !slices.Equal(addrs, want) {
		t.Fatalf("addresses = %v, want %v", addrs, want)
	}
	pair := got.Epics[1]
	if pair.Kind != wire.EpicKindMap || !slices.Equal(pair.Addresses, []string{"proj:design", "proj:design-impl"}) {
		t.Errorf("pair row = %+v, want a map row with the Design first", pair)
	}
	if r := got.Epics[0]; r.Title != "Title of it" || r.Project != "proj" || r.Name != "newer" || r.Kind != wire.EpicKindLoop || r.CompletedAt.IsZero() {
		t.Errorf("loop row = %+v", r)
	}
}

func TestEpicDetail_ArchivedListWorkspaceFallsBackToDefault(t *testing.T) {
	t.Parallel()
	done := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, ".archive/kept", archivedLoopFile("archived_workspace: Bugs\n"), done)
		rigs.WriteEpicFiles(t, store, ".archive/gone", archivedLoopFile("archived_workspace: Vanished\n"), done)
		rigs.WriteEpicFiles(t, store, ".archive/plain", archivedLoopFile(""), done)
	})
	if res, err := h.Client.Admin.CreateWorkspace(t.Context(), "Bugs"); err != nil || res.Refused {
		t.Fatalf("create workspace: %v %+v", err, res)
	}

	ws := map[string]string{}
	for _, r := range getArchivedList(t, h, base).Epics {
		ws[r.Name] = r.Workspace
	}
	want := map[string]string{"kept": "Bugs", "gone": "Default workspace", "plain": "Default workspace"}
	for name, w := range want {
		if ws[name] != w {
			t.Errorf("workspace of %s = %q, want %q", name, ws[name], w)
		}
	}
}

func TestEpicDetail_ArchivedLoopEpicHasArchivedAtAndNoLiveDecoration(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, ".archive/work", archivedLoopFile("archived_at: 2026-02-01T10:00:00Z\n"), map[string]string{
			"01-a.md": rigs.IssueMD("01", "done", "elapsed_time: 60\n", "A"),
		})
		// An active epic with the same name and an open ticket must not leak into the archived page.
		rigs.WriteEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics/archived/detail?address=proj:work")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var d wire.EpicDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.ArchivedAt.IsZero() || d.Summary != "Did it." || d.Done != 1 || d.Total != 1 {
		t.Errorf("detail = %+v, want the archived epic with archived_at", d)
	}
	if d.Queueable || d.Queued || len(d.Actions) != 0 || d.PR != nil || len(d.Rows) != 1 || d.Rows[0].RenderedStatus != "done" {
		t.Errorf("live decoration on an archived epic: %+v", d)
	}
}

func TestEpicDetail_ArchivedDesignAnswersToEitherAddressOfThePair(t *testing.T) {
	t.Parallel()
	done := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, ".archive/design", archivedDesignFile("archived_at: 2026-02-15T10:00:00Z\n"), done)
		rigs.WriteEpicFiles(t, store, ".archive/design-impl", archivedLoopFile("archived_at: 2026-02-15T10:00:00Z\n"), done)
	})

	code, d, body := getArchivedDetail(t, h, base, "proj:design")
	if code != http.StatusOK || d.Kind != wire.EpicKindMap || d.ImplAddress != "proj:design-impl" || d.ArchivedAt.IsZero() {
		t.Fatalf("design detail = %d %+v (%s)", code, d, body)
	}
	if d.Deletable || d.Queueable || d.Queued {
		t.Errorf("live decoration on an archived Design: %+v", d)
	}
	code, d, body = getArchivedDetail(t, h, base, "proj:design-impl")
	if code != http.StatusOK || d.Kind != wire.EpicKindLoop || d.ArchivedAt.IsZero() {
		t.Errorf("impl detail = %d %+v (%s)", code, d, body)
	}
}

func TestEpicDetail_ArchivedDetailUnknownAddressIs404(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "active", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	})
	for _, addr := range []string{"proj:active", "proj:missing", "nope:x", "bad"} {
		if code, _, body := getArchivedDetail(t, h, base, addr); code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404 (%s)", addr, code, body)
		}
	}
}

func getArchivedDetail(t *testing.T, h *servertest.Harness, base, address string) (int, wire.EpicDetail, string) {
	t.Helper()
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics/archived/detail?address="+address)
	var d wire.EpicDetail
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &d); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
	}
	return code, d, body
}
