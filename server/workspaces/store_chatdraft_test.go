package workspaces

import (
	"testing"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
)

func TestStoreJoin_ChatDraft(t *testing.T) {
	const id = "0123456789abcdef"
	d := fakeDesigns{designs: map[string]bool{"p:nav": true}, chats: map[string]bool{id: true}}
	s, err := Open(t.TempDir(), broker.New(0), func() Directory { return d })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	key := chatdrafts.MemberKey(id)
	if err := s.Join(key, "Bugs"); err != nil {
		t.Fatalf("join existing chat draft = %v", err)
	}
	if got := s.Workspace(key); got != "Bugs" {
		t.Errorf("chat draft workspace = %q, want Bugs", got)
	}
	if err := s.Join(chatdrafts.MemberKey("ffffffffffffffff"), "Bugs"); err != ReasonUnknownKey {
		t.Errorf("join unknown chat draft = %v, want ReasonUnknownKey", err)
	}
	if err := s.Join(drafts.MemberKey(id), "Bugs"); err != ReasonUnknownKey {
		t.Errorf("a chat draft id as a Design draft = %v, want ReasonUnknownKey", err)
	}
}
