package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestTrunkConflict_MarkersLeftPauseTheTicketAndKeepTheBackup(t *testing.T) {
	var f *schedFixture
	f = stashMergeFixtureWith(t, func(_ *schedFixture, _ servertest.Prompt) {
		reportIteration(t, f.ticketFile(t, "02"), "finished") // the markers are still there
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.subscribe(ctx, t)
	if res, err := f.h.Client.EpicStashMerge(ctx, "proj:"+schedEpic); err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}

	servertest.WaitForEvent(ctx, t, evs, server.EventIterationPaused, "proj:"+schedEpic+"/02")

	md := readFile(t, f.ticketFile(t, "02"))
	if !strings.Contains(md, "status: paused") || !strings.Contains(md, "pause_reason: conflict-markers") {
		t.Errorf("ticket not paused conflict-markers:\n%s", md)
	}
	if refs := f.backupRefs(t); len(refs) != 1 {
		t.Errorf("backup refs = %v, want the ref kept while the ticket is open", refs)
	}
	if merged := tickets.LoadEpicTicketMD(filepath.Join(f.store, "proj", schedEpic)).MergedAt; !merged.IsZero() {
		t.Errorf("root merged_at = %v while markers are left", merged)
	}
}

func TestTrunkConflict_NoMarkersLeftMakesTheTicketDoneDeletesTheBackupAndLandsTheEpic(t *testing.T) {
	var f *schedFixture
	f = stashMergeFixtureWith(t, func(_ *schedFixture, p servertest.Prompt) {
		testutil.WriteFile(t, p.Cwd, "same.txt", "resolved")
		reportIteration(t, f.ticketFile(t, "02"), "finished")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.subscribe(ctx, t)
	if res, err := f.h.Client.EpicStashMerge(ctx, "proj:"+schedEpic); err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}

	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)

	if md := readFile(t, f.ticketFile(t, "02")); !strings.Contains(md, "status: done") {
		t.Errorf("ticket not done:\n%s", md)
	}
	if refs := f.backupRefs(t); len(refs) != 0 {
		t.Errorf("backup refs = %v, want the ref deleted once the ticket is done", refs)
	}
	if merged := tickets.LoadEpicTicketMD(filepath.Join(f.store, "proj", schedEpic)).MergedAt; merged.IsZero() {
		t.Error("root has no merged_at")
	}
}

func TestTrunkConflict_StartOverRestoresTheBackupMarksDoneAndLandsTheEpic(t *testing.T) {
	// The agent reports nothing, so the ticket stays open on its pause.
	f := stashMergeFixtureWith(t, func(*schedFixture, servertest.Prompt) {})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.subscribe(ctx, t)
	if res, err := f.h.Client.EpicStashMerge(ctx, "proj:"+schedEpic); err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationPaused, "proj:"+schedEpic+"/02")
	if got := f.read(t, "same.txt"); !strings.Contains(got, "<<<<<<<") {
		t.Fatalf("same.txt = %q, want conflict markers", got)
	}

	res, err := f.h.Client.Repair(ctx, "start-over", server.RepairRequest{Address: "proj:" + schedEpic + "/02", Reason: "give up"})
	if err != nil || res.Refused {
		t.Fatalf("start-over = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)

	if got := f.read(t, "same.txt"); got != "local" {
		t.Errorf("same.txt = %q, want the backed-up local edit", got)
	}
	if md := readFile(t, f.ticketFile(t, "02")); !strings.Contains(md, "status: done") {
		t.Errorf("ticket not done:\n%s", md)
	}
	if refs := f.backupRefs(t); len(refs) != 0 {
		t.Errorf("backup refs = %v, want the ref deleted", refs)
	}
	if merged := tickets.LoadEpicTicketMD(filepath.Join(f.store, "proj", schedEpic)).MergedAt; merged.IsZero() {
		t.Error("root has no merged_at")
	}
}

func TestTrunkConflict_PruneKeepsAnOldRefOfAnOpenTicket(t *testing.T) {
	// The agent reports nothing, so the ticket stays open.
	f := stashMergeFixtureWith(t, func(*schedFixture, servertest.Prompt) {})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.subscribe(ctx, t)
	if res, err := f.h.Client.EpicStashMerge(ctx, "proj:"+schedEpic); err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationPaused, "proj:"+schedEpic+"/02")
	late := time.Now().Add(90 * 24 * time.Hour)

	f.h.Server.PruneBackups(f.repo, late)
	if refs := f.backupRefs(t); len(refs) != 1 {
		t.Fatalf("backup refs = %v, want the open ticket's ref kept", refs)
	}

	path := f.ticketFile(t, "02")
	body := strings.Replace(readFile(t, path), "status: paused\n", "status: done\n", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f.h.Server.PruneBackups(f.repo, late)
	if refs := f.backupRefs(t); len(refs) != 0 {
		t.Errorf("backup refs = %v, want the old ref pruned once the ticket is done", refs)
	}
}
