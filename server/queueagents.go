package server

import (
	"cmp"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/tickets"
)

// runnerPTY is the Runner of a run hosted in a gx agent-holder pty.
const runnerPTY = "pty"

// queueAgentIDPrefix keeps a Queue agent row's id apart from sessions-dir ids.
const queueAgentIDPrefix = "queue-"

// listSessionsWithQueue is listSessions plus a row per PTY Queue agent. Only
// the snapshot and the sessions route show Queue rows; every other caller
// (worktree removal, design lookup, resume) reads sessions-dir sessions only.
func (s *Server) listSessionsWithQueue() ([]SessionInfo, error) {
	list, err := s.listSessions()
	return append(list, s.queueAgentRows()...), err
}

// queueAgentRows lists the tracked runs hosted on the PTY runner. A row lives
// as long as its run's registry entry, which a finish or a stop drops with the
// session; a paused ticket's live agent keeps its run.
func (s *Server) queueAgentRows() []SessionInfo {
	var rows []SessionInfo
	for _, t := range s.registry.list() {
		if t.Runner != runnerPTY || t.Watched {
			continue
		}
		if row, ok := s.queueAgentRow(t); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func (s *Server) queueAgentRow(t trackedRun) (SessionInfo, bool) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return SessionInfo{}, false
	}
	runner := s.runnerForRun(t.Run)
	if runner == nil {
		return SessionInfo{}, false
	}
	st, err := runner.Status(t.Session)
	if err != nil {
		return SessionInfo{}, false
	}
	label := iterationLabel(t.Address)
	info := SessionInfo{
		ID:              queueAgentIDPrefix + label,
		Name:            label,
		Project:         addr.Project,
		Dir:             t.Cwd,
		Harness:         t.Agent,
		Ticket:          t.Address,
		Queue:           true,
		Holder:          filepath.Join(nativerunner.AgentsRoot(s.cfg.StateDir, addr.Project), label),
		Status:          SessionStatus{State: queueAgentState(st.State, t.RateLimited)},
		StatusChangedAt: t.StartedAt,
		RunStartedAt:    t.StartedAt,
		Model:           t.Model,
		Effort:          t.Effort,
	}
	if st.State == agentrunner.StateBlocked {
		info.Status.Message = st.BlockedReason
	}
	return info, true
}

// readableSession resolves a session read's id: a sessions-dir session, else a
// live Queue agent. Only the chat and screen reads use it; the write routes
// call sessionDir and so refuse a Queue agent id as an unknown session.
func (s *Server) readableSession(id string) (string, sessionMeta, SessionResult, error) {
	dir, meta, res, err := s.sessionDir(id)
	if !res.Refused || res.Reason != ReasonSessionUnknown || !strings.HasPrefix(id, queueAgentIDPrefix) {
		return dir, meta, res, err
	}
	for _, row := range s.queueAgentRows() {
		if row.ID != id {
			continue
		}
		d, err := nativerunner.OpenDir(row.Holder)
		if err != nil {
			break
		}
		// No Ticket: it would make the chat read the agent as a design step.
		return row.Holder, sessionMeta{
			ID:             id,
			Harness:        row.Harness,
			Project:        row.Project,
			Dir:            cmp.Or(d.Meta.Cwd, row.Dir),
			ConversationID: d.Meta.SessionID,
			Status:         row.Status,
		}, SessionResult{}, nil
	}
	return dir, meta, res, nil
}

// queueAgentState maps a run's live state to a session state. The hold lives
// in the run, so it wins over whatever the session shows.
func queueAgentState(state agentrunner.State, held bool) string {
	switch {
	case held:
		return SessionRateLimited
	case state == agentrunner.StateWorking:
		return SessionWorking
	case state == agentrunner.StateIdle:
		return SessionIdle
	case state == agentrunner.StateBlocked:
		return SessionBlocked
	default:
		return SessionExited
	}
}
