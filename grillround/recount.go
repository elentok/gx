package grillround

import (
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/transcript"
)

// Recount counts the Grilling rounds in the Claude transcript of conversation
// id (found under home, for the working directory dir) and replaces the items
// saved in root. A missing transcript keeps what was saved; a read error is
// returned and keeps it too. The caller skips conversations of other harnesses.
//
// It lives here, not in conversations, because tickets imports conversations
// and this package imports tickets.
func Recount(root, home, id, dir string) error {
	msgs, ok, err := transcript.ReadChat(transcript.PathIn(home, dir, id))
	if err != nil || !ok {
		return err
	}
	return conversations.SaveGrilling(root, id, Count(msgs))
}
