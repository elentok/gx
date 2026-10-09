package server

import (
	"context"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const pruneInterval = 24 * time.Hour

// logPruner is a runner that keeps agent logs on disk (nativerunner.Headless).
type logPruner interface {
	Prune(maxAge time.Duration, now time.Time, parked func(label string) bool) ([]string, error)
}

// reattacher adopts the agent a previous server launched.
type reattacher interface {
	Reattach(label string) (agentrunner.Session, nativerunner.Verdict, error)
}

func (s *Server) logRetention() time.Duration {
	if s.cfg.LogRetention > 0 {
		return s.cfg.LogRetention
	}
	return nativerunner.DefaultLogRetention
}

// keepPruned prunes agent logs daily; Serve already pruned once at start.
func (s *Server) keepPruned(ctx context.Context) {
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.pruneAgentLogs(now)
		}
	}
}

// pruneAgentLogs removes expired logs of every project whose runner keeps
// them. A parked ticket's logs stay: a person may still need them.
func (s *Server) pruneAgentLogs(now time.Time) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Warn("prune agent logs", "err", err)
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		p, ok := s.runnerFor(project).(logPruner)
		if !ok {
			continue
		}
		parked := parkedLabels(dir)
		pruned, err := p.Prune(s.logRetention(), now, func(label string) bool { return parked[label] })
		if err != nil {
			s.log.Warn("prune agent logs", "project", project, "err", err)
		}
		if len(pruned) > 0 {
			s.log.Info("pruned agent logs", "project", project, "labels", pruned)
		}
	}
}

// parkedLabels is the iteration labels of the project's needs-repair and
// needs-answer tickets. An unreadable store keeps everything: nothing parked
// may lose its logs on a guess.
func parkedLabels(projectDir string) map[string]bool {
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, e := range epics {
		for _, t := range e.Tickets {
			if st := schema.Status(t.Status); st == schema.StatusNeedsRepair || st == schema.StatusNeedsAnswer {
				label, _, _ := ralphloop.IterationIdentity(filepath.Base(e.Path), t.Identifier, "")
				out[label] = true
			}
		}
	}
	return out
}
