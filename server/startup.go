package server

import (
	"context"
	"time"
)

// migrationsEnded reports whether the start-up migrations have ended, pass or
// fail. Until then no code may read or change tickets.
func (s *Server) migrationsEnded() bool {
	select {
	case <-s.migrator.done:
		return true
	default:
		return false
	}
}

// Rescan rescans the store now, as a ping or the watch would, once the
// migrations have ended: a rescan reads nothing until then.
func (s *Server) Rescan() {
	<-s.migrator.done
	s.idx.Rescan()
}

// startAfterMigrations waits for the migrations to end, then runs the steps that
// read tickets. It reports false when ctx ended first. A project whose migration
// failed is scanned and recovered like the rest.
func (s *Server) startAfterMigrations(ctx context.Context) bool {
	defer close(s.migrator.startupDone)
	select {
	case <-s.migrator.done:
	case <-ctx.Done():
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	// The first scan comes first: it fills the index the steps below read, and
	// it also raises the parent defects.
	s.startupStep("first-rescan", s.idx.Rescan)
	s.startupStep("recover-merges", s.recoverMerges)
	// Running agents stay invisible to a restart, only a little later.
	s.startupStep("reclaim-runs", s.reclaimRuns)
	s.logFlushingRunners()
	s.pruneAgentLogs(time.Now())
	return true
}

func (s *Server) startupStep(name string, run func()) {
	s.log.Info("startup step", "step", name)
	run()
}
