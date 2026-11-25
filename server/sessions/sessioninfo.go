package sessions

import (
	"cmp"
	"strings"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop/agent"
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
	var tokens int
	var turn codexsession.TurnModel
	isCodex := m.Harness == string(harness.Codex)
	if isCodex {
		turn, tokens = mg.chat.ReportedCodex(cm)
		info.Model, info.Effort = turn.Model, turn.Effort
	} else {
		turn, tokens = mg.chat.ReportedClaudeTurn(cm)
		info.Model = turn.Model
	}
	if !m.SwitchedAt.IsZero() {
		shown := chat.ShownModel(cm.SavedTurn(), turn, m.SwitchedAt, isCodex)
		info.Model, info.Effort = shown.Model, shown.Effort
	}
	if tokens > 0 {
		info.ContextTokens, info.ContextLevel = &tokens, agent.LevelForTokens(tokens)
		info.ContextCeiling = agent.SmartZoneCeiling
		info.Usage = mg.chat.ConversationUsage(cm)
	}
	info.HasHistory = tokens > 0
	if sw, ok := mg.switches.get(m.ID); ok {
		info.SwitchingModel, info.SwitchingEffort, info.SwitchingBack = sw.model, sw.effort, sw.back
	}
	if f, ok := mg.switches.failure(m.ID); ok {
		info.SwitchFailedModel, info.SwitchFailedOld = f.attempted, f.old
	}
	setLatestModel(mg, &info, m.Harness)
	info.ModelSwitched = m.SwitchedByHand || chat.ModelSwitched(m.Model, info.Model)
	if at := mg.prompter.AutoAdvanceDeadline(m.ID); !at.IsZero() {
		info.AutoAdvanceAt = &at
	}
	if at := mg.prompter.AutoQueueDeadline(m.ID); !at.IsZero() {
		info.AutoQueueAt = &at
	}
	info.Failure = mg.chat.Failure(cm)
	if m.Flow == "" {
		info.ShowDiffDir = mg.diffDirs.Qualifies(m.Project, m.Dir)
	}
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
			RunTicket:      row.Ticket,
		}, wire.SessionResult{}, nil
	}
	return dir, meta, res, nil
}
