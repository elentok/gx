package queue

import (
	"path/filepath"

	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/tickets"
)

// RunnerPTY is the Runner of a run hosted in a gx agent-holder pty.
const RunnerPTY = "pty"

// AgentIDPrefix keeps a Queue agent row's id apart from sessions-dir ids.
const AgentIDPrefix = "queue-"

// listSessionsWithQueue is listSessions plus a row per PTY Queue agent. Only
// the snapshot and the sessions route show Queue rows; every other caller
// (worktree removal, design lookup, resume) reads sessions-dir sessions only.
func (q *Queue) ListSessionsWithQueue() ([]wire.SessionInfo, error) {
	list, err := q.host.ListSessions()
	return append(list, q.AgentRows()...), err
}

// AgentRows lists the tracked runs hosted on the PTY runner. A row lives
// as long as its run's registry entry, which a finish or a stop drops with the
// session; a paused ticket's live agent keeps its run.
func (q *Queue) AgentRows() []wire.SessionInfo {
	var rows []wire.SessionInfo
	for _, t := range q.registry.List() {
		if t.Runner != RunnerPTY || t.Watched {
			continue
		}
		if row, ok := q.queueAgentRow(t); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func (q *Queue) queueAgentRow(t runs.TrackedRun) (wire.SessionInfo, bool) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return wire.SessionInfo{}, false
	}
	runner := q.host.RunnerForRun(t.Run)
	if runner == nil {
		return wire.SessionInfo{}, false
	}
	st, err := runner.Status(t.Session)
	if err != nil {
		return wire.SessionInfo{}, false
	}
	label := runs.IterationLabel(t.Address)
	info := wire.SessionInfo{
		ID:              AgentIDPrefix + label,
		Name:            label,
		Project:         addr.Project,
		Dir:             t.Cwd,
		Harness:         t.Agent,
		Ticket:          t.Address,
		Queue:           true,
		Holder:          filepath.Join(nativerunner.AgentsRoot(q.stateDir, addr.Project), label),
		WaitingLabel:    q.host.CheckWaitingLabel(t.Address),
		Status:          wire.SessionStatus{State: queueAgentState(st.State, t.RateLimited)},
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

// queueAgentState maps a run's live state to a session state. The hold lives
// in the run, so it wins over whatever the session shows.
func queueAgentState(state agentrunner.State, held bool) string {
	switch {
	case held:
		return wire.SessionRateLimited
	case state == agentrunner.StateWorking:
		return wire.SessionWorking
	case state == agentrunner.StateIdle:
		return wire.SessionIdle
	case state == agentrunner.StateBlocked:
		return wire.SessionBlocked
	default:
		return wire.SessionExited
	}
}
