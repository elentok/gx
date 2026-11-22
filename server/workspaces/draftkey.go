package workspaces

import (
	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
)

// DraftKind says which kind of draft a member key names.
type DraftKind int

const (
	NotDraft DraftKind = iota
	DesignDraft
	ChatDraft
)

// ClassifyDraftKey answers which draft kind a member key is and its draft id.
// A key that is neither is NotDraft with an empty id.
func ClassifyDraftKey(key string) (DraftKind, string) {
	if id, ok := drafts.ParseMemberKey(key); ok {
		return DesignDraft, id
	}
	if id, ok := chatdrafts.ParseMemberKey(key); ok {
		return ChatDraft, id
	}
	return NotDraft, ""
}
