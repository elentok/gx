package workspaces_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func bugsMembers(ctx context.Context, f rigs.DesignsFixture) int {
	l, _ := f.H.Client.Reads.Workspaces(ctx)
	for _, w := range l.Workspaces {
		if w.Name == "Bugs" {
			return w.Members
		}
	}
	return -1
}

func moveToBugs(t *testing.T, f rigs.DesignsFixture, key string) {
	t.Helper()
	if res, err := f.H.Client.Admin.MoveToWorkspace(f.Ctx, "Bugs", key); err != nil || res.Results[0].Refused() {
		t.Fatalf("move %s = %+v, %v", key, res, err)
	}
}

func newChat(t *testing.T, f rigs.DesignsFixture) string {
	t.Helper()
	chat, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir()})
	if err != nil || chat.Session == nil {
		t.Fatalf("create chat = %+v, %v", chat, err)
	}
	return chat.Session.ID
}

func TestWorkspacesCleanup_DeletingAnUnchartedDesignDropsItsMembership(t *testing.T) {
	f, _ := startMembership(t)
	opened := f.OpenNew(t, "fresh-idea", "an idea")
	if opened.Session == nil {
		t.Fatalf("open = %+v", opened)
	}
	moveToBugs(t, f, "proj:fresh-idea")
	if res, err := f.H.Client.Sessions.Stop(f.Ctx, opened.Session.ID); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	if res, err := f.H.Client.Sessions.DeleteDesign(f.Ctx, wire.DesignDeleteRequest{Epic: "proj:fresh-idea"}); err != nil || res.Refused {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "membership dropped", func() bool { return bugsMembers(f.Ctx, f) == 0 })
}

func TestWorkspacesCleanup_StoppedAndRemovedChatsDropTheirMembership(t *testing.T) {
	f, _ := startMembership(t)
	stopped, removed := newChat(t, f), newChat(t, f)
	moveToBugs(t, f, stopped)
	moveToBugs(t, f, removed)
	if got := bugsMembers(f.Ctx, f); got != 2 {
		t.Fatalf("members = %d, want 2", got)
	}
	if res, err := f.H.Client.Sessions.Stop(f.Ctx, stopped); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "stopped chat dropped", func() bool { return bugsMembers(f.Ctx, f) == 1 })
	if _, err := f.H.Client.Sessions.Stop(f.Ctx, removed); err != nil {
		t.Fatal(err)
	}
	if res, err := f.H.Client.Sessions.Remove(f.Ctx, removed); err != nil || res.Refused {
		t.Fatalf("remove = %+v, %v", res, err)
	}
	servertest.WaitFor(t, 20*time.Second, "removed chat dropped", func() bool { return bugsMembers(f.Ctx, f) == 0 })
}

func TestWorkspacesCleanup_AFinishedEpicKeepsItsMembership(t *testing.T) {
	store := t.TempDir()
	servertest.WriteCompleteEpic(t, store, "done-epic")
	servertest.WriteTicket(t, store, "proj", "gone", "01", "first", "")
	f := rigs.StartDesignsIn(t, store)
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, "Bugs"); err != nil {
		t.Fatal(err)
	}
	moveToBugs(t, f, "proj:done-epic")
	moveToBugs(t, f, "proj:gone")
	if err := os.RemoveAll(filepath.Join(store, "proj", "gone")); err != nil {
		t.Fatal(err)
	}
	// Once the deleted epic is pruned a rescan has run, and the finished one must have survived it.
	servertest.WaitFor(t, 20*time.Second, "deleted epic pruned", func() bool { return bugsMembers(f.Ctx, f) == 1 })
	if got := bugsMembers(f.Ctx, f); got != 1 {
		t.Errorf("members = %d, want the finished epic only", got)
	}
}
