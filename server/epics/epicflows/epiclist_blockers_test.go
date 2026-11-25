package epicflows_test

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

const (
	openEpic = "---\nstatus: open\n---\n"
	mergedAt = "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n"
)

func blockedEpic(refs string) string {
	return "---\nstatus: open\nblocked_by: [" + refs + "]\n---\n"
}

func oneTicket(status string) map[string]string {
	return map[string]string{"01-a.md": rigs.IssueMD("01", status, "", "A")}
}

func listRows(t *testing.T, setup func(store string)) map[string]wire.EpicListRow {
	t.Helper()
	return listRowsAfter(t, setup, nil)
}

// listRowsAfter runs after on the started server before it lists the epics.
func listRowsAfter(t *testing.T, setup func(store string), after func(h *servertest.Harness)) map[string]wire.EpicListRow {
	t.Helper()
	h, base := rigs.StartEpicDetail(t, setup)
	if after != nil {
		after(h)
	}
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	byName := map[string]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
		byName[r.Name] = r
	}
	return byName
}

func blockerNames(bs []wire.EpicBlocker) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Name+b.Ref)
	}
	return out
}

func TestEpicList_BlockersListUnreleasedAndInvalid(t *testing.T) {
	t.Parallel()
	rows := listRows(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "live", openEpic, oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "landed", mergedAt, oneTicket("done"))
		rigs.WriteEpicFiles(t, store, "waiter", blockedEpic(`live, "proj:landed", ghost`), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "free", blockedEpic("landed"), oneTicket("open"))
	})

	got := rows["waiter"].Blockers
	if len(got) != 2 || !got[0].Invalid || got[0].Ref != "ghost" || got[0].Address != "" || got[0].State != "" {
		t.Fatalf("waiter blockers = %+v, want the invalid ghost first", got)
	}
	if got[1].Address != "proj:live" || got[1].Name != "live" || got[1].State != wire.EpicListReady {
		t.Errorf("waiter blockers[1] = %+v, want live in state ready", got[1])
	}
	if rows["waiter"].WaitingOnEpic {
		t.Error("waiter has an invalid ref, so it is in error, not Waiting")
	}
	if len(rows["free"].Blockers) != 0 || rows["free"].WaitingOnEpic {
		t.Errorf("free = %+v, want no blockers", rows["free"])
	}
}

func TestEpicList_BlockersIgnoreAnOnDoneReleasedBlocker(t *testing.T) {
	t.Parallel()
	rows := listRows(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "dep", "---\nstatus: done\ncompleted_at: 2026-01-02T11:00:00Z\n---\n", oneTicket("done"))
		rigs.WriteEpicFiles(t, store, "waiter", blockedEpic("dep"), oneTicket("open"))
		if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj","landing":"on-done"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if w := rows["waiter"]; len(w.Blockers) != 0 || w.WaitingOnEpic {
		t.Errorf("waiter = %+v, want released by an on-done blocker", w)
	}
}

func TestEpicList_BlockersSortNotMovingFirstThenByName(t *testing.T) {
	t.Parallel()
	rows := listRows(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "zed", openEpic, oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "live", openEpic, oneTicket("open"))
		// Implemented, awaiting its merge: moving.
		rigs.WriteEpicFiles(t, store, "implemented", "---\nstatus: done\ncompleted_at: 2026-01-02T11:00:00Z\n---\n", oneTicket("done"))
		rigs.WriteEpicFiles(t, store, "waiter", blockedEpic("implemented, zed, live"), oneTicket("open"))
	})
	got := blockerNames(rows["waiter"].Blockers)
	if want := []string{"live", "zed", "implemented"}; !slices.Equal(got, want) {
		t.Errorf("blockers = %v, want %v", got, want)
	}
}

func TestEpicList_BlockersHintNamesTheDeepestNotMovingEpic(t *testing.T) {
	t.Parallel()
	rows := listRowsAfter(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "stuck", openEpic, oneTicket("draft"))
		rigs.WriteEpicFiles(t, store, "mid", blockedEpic("stuck"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "top", blockedEpic("mid"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "implemented", "---\nstatus: done\ncompleted_at: 2026-01-02T11:00:00Z\n---\n", oneTicket("done"))
		rigs.WriteEpicFiles(t, store, "mid2", blockedEpic("implemented"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "top2", blockedEpic("mid2"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "top3", blockedEpic("implemented, mid2"), oneTicket("open"))
	}, func(h *servertest.Harness) {
		// Only a queued Waiting epic hands the hint on; mid2 stays unqueued.
		if res, err := h.Client.Queue.Add(t.Context(), "proj:mid/01", "claude"); err != nil || res.Refused {
			t.Fatalf("queue add: %+v, %v", res, err)
		}
	})

	top := rows["top"]
	if !top.WaitingOnEpic || len(top.Blockers) != 1 {
		t.Fatalf("top = %+v, want Waiting on one blocker", top)
	}
	h := top.Blockers[0].Hint
	if h == nil || h.Name != "stuck" || h.Address != "proj:stuck" || h.State != wire.EpicListPaused || h.Deadlocked {
		t.Errorf("top hint = %+v, want stuck in state paused", h)
	}
	// mid2 is Waiting but not queued, so it is itself the stuck epic: no hint.
	if got := rows["top2"].Blockers; len(got) != 1 || got[0].Hint != nil || got[0].State != wire.EpicListReady {
		t.Errorf("top2 blockers = %+v, want unqueued mid2 itself, no hint", got)
	}
	if got, want := blockerNames(rows["top3"].Blockers), []string{"mid2", "implemented"}; !slices.Equal(got, want) {
		t.Errorf("top3 blockers = %v, want the unqueued blocker sorted as not moving: %v", got, want)
	}
	if got := rows["mid"].Blockers; len(got) != 1 || got[0].Hint != nil || got[0].State != wire.EpicListPaused {
		t.Errorf("mid blockers = %+v, want stuck itself, no hint", got)
	}
}

func TestEpicList_BlockersQueuedWaitingBlockerOverMovingEpicStaysMoving(t *testing.T) {
	t.Parallel()
	rows := listRowsAfter(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "implemented", "---\nstatus: done\ncompleted_at: 2026-01-02T11:00:00Z\n---\n", oneTicket("done"))
		rigs.WriteEpicFiles(t, store, "mid", blockedEpic("implemented"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "top", blockedEpic("mid"), oneTicket("open"))
	}, func(h *servertest.Harness) {
		if res, err := h.Client.Queue.Add(t.Context(), "proj:mid/01", "claude"); err != nil || res.Refused {
			t.Fatalf("queue add: %+v, %v", res, err)
		}
	})
	got := rows["top"].Blockers
	if len(got) != 1 || got[0].Hint != nil || got[0].State != wire.EpicListQueued {
		t.Errorf("top blockers = %+v, want queued waiting mid, moving, no hint", got)
	}
}

func TestEpicList_BlockersEpicCycleEndsTheWalk(t *testing.T) {
	t.Parallel()
	rows := listRows(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "ca", blockedEpic("cb"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "cb", blockedEpic("ca"), oneTicket("open"))
	})
	if len(rows["ca"].Blockers) != 1 || len(rows["cb"].Blockers) != 1 {
		t.Errorf("rows = %+v / %+v, want one blocker each", rows["ca"], rows["cb"])
	}
}

func TestEpicList_WaitingOnEpicIsFalseForAPausedEpic(t *testing.T) {
	t.Parallel()
	rows := listRows(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "live", openEpic, oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "paused", blockedEpic("live"), oneTicket("draft"))
		rigs.WriteEpicFiles(t, store, "waiting", blockedEpic("live"), oneTicket("open"))
	})
	if p := rows["paused"]; p.WaitingOnEpic || len(p.Blockers) != 1 {
		t.Errorf("paused = %+v, want one blocker and not Waiting", p)
	}
	if w := rows["waiting"]; !w.WaitingOnEpic || len(w.Blockers) != 1 {
		t.Errorf("waiting = %+v, want Waiting on one blocker", w)
	}
}

func TestEpicList_WaitingStatusWordsNameTheFirstBlocker(t *testing.T) {
	t.Parallel()
	rows := listRows(t, func(store string) {
		for _, name := range []string{"aa", "bb", "cc"} {
			rigs.WriteEpicFiles(t, store, name, openEpic, oneTicket("open"))
		}
		rigs.WriteEpicFiles(t, store, "one", blockedEpic("aa"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "two", blockedEpic("aa, bb"), oneTicket("open"))
		rigs.WriteEpicFiles(t, store, "three", blockedEpic("aa, bb, cc"), oneTicket("open"))
	})
	for name, want := range map[string]string{
		"one": "Waiting on aa", "two": "Waiting on aa and 1 more", "three": "Waiting on aa and 2 more",
	} {
		if got := rows[name].StatusWords; got != want {
			t.Errorf("%s status_words = %q, want %q", name, got, want)
		}
	}
}
