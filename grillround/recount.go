package grillround

import (
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// Recount counts the Grilling rounds in the Claude transcript of record r
// (found under home) and replaces the items saved in root. A missing
// transcript keeps what was saved; a read error is returned and keeps it too.
// A record of another harness is skipped and gets no counts.
//
// It lives here, not in conversations, because tickets imports conversations
// and this package imports tickets.
func Recount(root, home string, r conversations.Record) error {
	if r.Harness != string(harness.Claude) {
		return nil
	}
	msgs, ok, err := transcript.ReadChat(transcript.PathIn(home, r.Dir, r.ID))
	if err != nil || !ok {
		return err
	}
	counted := Count(msgs)
	items := make([]conversations.GrillingItem, len(counted))
	for i, it := range counted {
		items[i] = conversations.GrillingItem{MessageID: it.MessageID, At: it.At, Questions: it.Questions, Words: it.Words}
	}
	return conversations.SaveGrilling(root, r.ID, items)
}
