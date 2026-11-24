package server_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire/ops"
)

func TestPruneEpicMembers_Drafts(t *testing.T) {
	h := servertest.Start(t)
	store := drafts.New(h.StateDir)
	kept, err := store.Create("proj", "kept", "an idea")
	if err != nil {
		t.Fatal(err)
	}
	gone, err := store.Create("proj", "gone", "an idea")
	if err != nil {
		t.Fatal(err)
	}
	ws := h.Server.Workspaces()
	if err := ws.Create("A"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{drafts.MemberKey(kept.ID), drafts.MemberKey(gone.ID), "a-session-id", "nowhere:epic"} {
		if err := ws.SetMember(key, "A"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Delete(gone.ID); err != nil {
		t.Fatal(err)
	}

	// A new ticket makes the index rescan, which prunes.
	servertest.WriteTicket(t, h.TicketStore, "proj", "rescan", "01", "first", "")
	servertest.WaitFor(t, 10*time.Second, "the dead draft member to be pruned", func() bool {
		return ws.Workspace(drafts.MemberKey(gone.ID)) != "A"
	})
	for _, key := range []string{drafts.MemberKey(kept.ID), "a-session-id", "nowhere:epic"} {
		if got := ws.Workspace(key); got != "A" {
			t.Errorf("%s workspace = %q, want A (kept)", key, got)
		}
	}
}

func TestPruneEpicMembers_ChatDrafts(t *testing.T) {
	h := servertest.Start(t)
	store := chatdrafts.New(h.StateDir)
	mk := func() string {
		id, err := chatdrafts.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Create(&ops.ChatDraft{ID: id, Project: "proj", Brief: "x", Folder: ops.ChatDraftFolderTrunk}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	kept, gone := mk(), mk()
	ws := h.Server.Workspaces()
	if err := ws.Create("A"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{chatdrafts.MemberKey(kept), chatdrafts.MemberKey(gone), "a-session-id"} {
		if err := ws.SetMember(key, "A"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Delete(gone); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicket(t, h.TicketStore, "proj", "rescan", "01", "first", "")
	servertest.WaitFor(t, 10*time.Second, "the dead chat draft member to be pruned", func() bool {
		return ws.Workspace(chatdrafts.MemberKey(gone)) != "A"
	})
	for _, key := range []string{chatdrafts.MemberKey(kept), "a-session-id"} {
		if got := ws.Workspace(key); got != "A" {
			t.Errorf("%s workspace = %q, want A (kept)", key, got)
		}
	}
}
