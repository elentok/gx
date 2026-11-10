package cleanup_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func archiveReq(epic string) wire.CleanupWriteRequest {
	return wire.CleanupWriteRequest{Project: "proj", Epic: epic}
}

func TestCleanupArchive_DoneMergedEpicLeavesTheActiveList(t *testing.T) {
	repo, store := testutil.TempRepo(t), t.TempDir()
	servertest.WriteCompleteEpic(t, store, "landed")
	servertest.SetProjectRepo(t, store, "proj", repo)
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", "landed"), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Reads.ArchiveCleanupEpic(context.Background(), archiveReq("landed"))
	if err != nil || res.Refused {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	archived, err := tickets.ArchivedEpicNames(filepath.Join(store, "proj"))
	if err != nil || !archived["landed"] {
		t.Errorf("archived = %v, err = %v", archived, err)
	}
	active, err := tickets.Load(filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range active {
		if e.Name == "landed" {
			t.Errorf("epic still in the active list")
		}
	}
}

func TestCleanupArchive_ArchivesADoneEpicThatHasNotLanded(t *testing.T) {
	repo, store := testutil.TempRepo(t), t.TempDir()
	servertest.WriteCompleteEpic(t, store, "unlanded")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Reads.ArchiveCleanupEpic(context.Background(), archiveReq("unlanded"))
	if err != nil || res.Refused {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if archived, _ := tickets.ArchivedEpicNames(filepath.Join(store, "proj")); !archived["unlanded"] {
		t.Errorf("unmerged epic not archived")
	}
}

func TestCleanupArchive_RefusesAQueuedEpic(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "queued", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.BudgetSoftLimit = 5 })
	servertest.RegisterTurn(h)
	h.Server.Budget().RecordSpend("spent", 6) // holds the launch, so the root stays queued
	ctx := context.Background()
	if res, err := h.Client.Queue.Add(ctx, "proj:queued/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	res, err := h.Client.Reads.ArchiveCleanupEpic(ctx, archiveReq("queued"))
	if err != nil || !res.Refused || res.Reason != wire.ReasonCleanupQueued || res.Message == "" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if archived, _ := tickets.ArchivedEpicNames(filepath.Join(store, "proj")); archived["queued"] {
		t.Errorf("queued epic archived")
	}
}
