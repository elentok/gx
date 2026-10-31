package grillround

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// Homes are the per-harness home directories conversations are found under.
type Homes struct {
	Claude string
	Codex  string
}

// Recount counts the Grilling rounds in the conversation id of harness kind
// (found under homes, for the working directory dir) and replaces the items
// saved in root. A missing conversation file keeps what was saved; a read error
// is returned and keeps it too. Every kind but Codex reads as Claude.
//
// It lives here, not in conversations, because tickets imports conversations
// and this package imports tickets.
func Recount(root string, homes Homes, kind agentrunner.Kind, id, dir string) error {
	path, ok, err := conversationPath(homes, kind, id, dir)
	if err != nil || !ok {
		return err
	}
	res, ok, err := chatreader.ReadFile(kind, path)
	if err != nil || !ok {
		return err
	}
	return conversations.SaveGrilling(root, id, Count(res.Messages))
}

func conversationPath(homes Homes, kind agentrunner.Kind, id, dir string) (string, bool, error) {
	if kind == harness.Codex {
		return codexsession.RolloutPathIn(homes.Codex, id)
	}
	return transcript.PathIn(homes.Claude, dir, id), true, nil
}
