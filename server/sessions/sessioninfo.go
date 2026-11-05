package sessions

import (
	"cmp"
	"strings"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// agentRows is the Queue's live agents, which the chat and screen reads show
// as sessions.
type agentRows interface {
	AgentRows() []wire.SessionInfo
}

// SessionInfo is m.Info() plus the failure that needs a transcript read, the
// reported model and the running countdown.
func (mg *Manager) SessionInfo(m Meta) wire.SessionInfo {
	cm := m.ChatMeta()
	info := m.Info()
	info.Model = mg.chat.ReportedModel(cm)
	info.ModelSwitched = chat.ModelSwitched(m.Model, info.Model)
	if m.Harness == string(harness.Codex) {
		info.Effort = mg.chat.ReportedCodexTurn(cm).Effort
	}
	if at := mg.prompter.AutoAdvanceDeadline(m.ID); !at.IsZero() {
		info.AutoAdvanceAt = &at
	}
	info.Failure = mg.chat.Failure(cm)
	return info
}

// ReadableSession resolves a session read's id: a sessions-dir session, else a
// live Queue agent. Only the chat and screen reads use it; the write routes
// call SessionDir and so refuse a Queue agent id as an unknown session.
func (mg *Manager) ReadableSession(id string) (string, Meta, wire.SessionResult, error) {
	dir, meta, res, err := mg.SessionDir(id)
	if !res.Refused || res.Reason != wire.ReasonSessionUnknown || !strings.HasPrefix(id, queue.AgentIDPrefix) {
		return dir, meta, res, err
	}
	for _, row := range mg.queue.AgentRows() {
		if row.ID != id {
			continue
		}
		d, err := nativerunner.OpenDir(row.Holder)
		if err != nil {
			break
		}
		// No Ticket: it would make the chat read the agent as a design step.
		return row.Holder, Meta{
			ID:             id,
			Harness:        row.Harness,
			Project:        row.Project,
			Dir:            cmp.Or(d.Meta.Cwd, row.Dir),
			ConversationID: d.Meta.SessionID,
			Status:         row.Status,
		}, wire.SessionResult{}, nil
	}
	return dir, meta, res, nil
}
