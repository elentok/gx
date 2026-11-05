package chat

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// TurnMark is a session's transcript as it stood at an idle edge.
type TurnMark struct {
	Path string
	Size int64
	Kind agentrunner.Kind
}

// TurnEnded says whether the transcript at Path holds its turn's end line, by
// the format kind of the session.
func (m TurnMark) TurnEnded() (bool, error) {
	if m.Kind == harness.Codex {
		return codexsession.TurnEnded(m.Path)
	}
	return transcript.TurnEnded(m.Path)
}

// DesignRef names the conversation and ticket of a Design session that just
// went idle. Live marks a turn that ended while this server watched it, the
// only kind that auto-advances.
type DesignRef struct {
	Conversation, Ticket string
	Live                 bool
}
