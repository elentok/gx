package workspaces_test

import (
	"testing"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/workspaces"
)

func TestClassifyDraftKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		kind workspaces.DraftKind
		id   string
	}{
		{"design draft", drafts.MemberKey("d1"), workspaces.DesignDraft, "d1"},
		{"chat draft", chatdrafts.MemberKey("c1"), workspaces.ChatDraft, "c1"},
		{"epic", "proj/epic", workspaces.NotDraft, ""},
		{"blank", "", workspaces.NotDraft, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, id := workspaces.ClassifyDraftKey(c.key)
			if kind != c.kind || id != c.id {
				t.Fatalf("ClassifyDraftKey(%q) = %v, %q; want %v, %q", c.key, kind, id, c.kind, c.id)
			}
		})
	}
}
