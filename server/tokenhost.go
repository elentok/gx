package server

import (
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/wire"
)

// tokenHost is limits.TokenHost: what the token poll reads from the root.
type tokenHost struct{ s *Server }

func (h tokenHost) Snapshot() wire.Snapshot { return h.s.idx.Snapshot() }

func (h tokenHost) OnMapEpic(address string) bool { return h.s.onMapEpic(address) }

func (h tokenHost) BudgetIteration(address string) (wire.IterationInfo, bool) {
	return h.s.budget.Iteration(address)
}

func (h tokenHost) MigrationsDone() <-chan struct{} { return h.s.migrator.Done() }

func (h tokenHost) Sessions() ([]limits.InteractiveSession, error) {
	metas, err := h.s.sessions.listSessionMetas()
	if err != nil {
		return nil, err
	}
	out := make([]limits.InteractiveSession, len(metas))
	for i, m := range metas {
		out[i] = limits.InteractiveSession{
			ID: m.ID, Harness: m.Harness, Project: m.Project, Dir: m.Dir, Ticket: m.Ticket,
			ConversationID: m.ConversationID, Stopped: m.Stopped, Exit: m.Exit,
		}
	}
	return out, nil
}
