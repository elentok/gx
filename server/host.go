package server

import (
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// mergeDeps reads the fields the merge types copy; call it once they are set.
func (s *Server) mergeDeps() merge.Deps {
	return merge.Deps{
		Host: s, Log: s.log, Dirs: merge.Dirs{StateDir: s.cfg.StateDir, TicketStore: s.cfg.TicketStore},
		Idx: s.idx, Projects: s.projects, Queue: s.queue, OneOffs: s.oneOffs, Events: s.events,
		Writer: s.writer, Chat: s.chat, Registry: s.registry, Guards: s.guards, Settings: &s.settings,
		Harness: s.harness, Sessions: s.sessions,
	}
}

// Merger and Reconciler give tests the merge seams (SetMergeInto,
// HoldTrunkLock, SetMergedProbe, ...).
func (s *Server) Merger() *merge.Merger { return s.merger }

func (s *Server) Reconciler() *merge.Reconciler { return s.rec }

func (s *Server) Landing() *merge.Landing { return s.landing }

func (s *Server) RecoverAsync(f recovery.Failure) { s.recovery.RecoverAsync(f) }

func (s *Server) EpicState(root queue.RootRef, e tickets.Epic) wire.EpicState {
	return epicStateFor(root, e, s.registry.StartedAtOf)
}

func (s *Server) EpicMergeBlocked(e tickets.Epic) bool { return epicMergeBlocked(e) }
