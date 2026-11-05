package sessions

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop/worktree"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const pruneInterval = 24 * time.Hour

// logPruner is a runner that keeps agent logs on disk (nativerunner.Headless).
type logPruner interface {
	Prune(maxAge time.Duration, now time.Time, paused func(label string) bool) ([]string, error)
}

// reattacher adopts the agent a previous server launched.
type Reattacher interface {
	Reattach(label string) (agentrunner.Session, nativerunner.Verdict, error)
}

// AgentLogs prunes the agent logs of every project.
type AgentLogs struct {
	ticketStore string
	log         *slog.Logger
	settings    *settings.Holder
	harness     *harnesschoice.Harness
}

func (al *AgentLogs) logRetention() time.Duration {
	if d := al.settings.Load().LogRetention; d > 0 {
		return d
	}
	return nativerunner.DefaultLogRetention
}

// KeepPruned prunes agent logs daily; Serve already pruned once at start.
func (al *AgentLogs) KeepPruned(ctx context.Context) {
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			al.Prune(now)
		}
	}
}

// Prune removes expired logs of every project's agent directories,
// whatever runner is configured: the headless runner walks the agents root and
// prunes each directory by the runner its meta.json names. A paused ticket's
// logs stay: a person may still need them.
func (al *AgentLogs) Prune(now time.Time) {
	dirs, err := tickets.ProjectDirs(al.ticketStore)
	if err != nil {
		al.log.Warn("prune agent logs", "err", err)
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		p, ok := al.harness.RunnerNamed(string(agentrunner.ChoiceHeadless), project).(logPruner)
		if !ok {
			continue
		}
		paused := pausedLabels(dir)
		pruned, err := p.Prune(al.logRetention(), now, func(label string) bool { return paused[label] })
		if err != nil {
			al.log.Warn("prune agent logs", "project", project, "err", err)
		}
		if len(pruned) > 0 {
			al.log.Info("pruned agent logs", "project", project, "labels", pruned)
		}
	}
}

// pausedLabels is the iteration labels of the project's stuck and
// asking tickets. An unreadable store keeps everything: nothing paused
// may lose its logs on a guess.
func pausedLabels(projectDir string) map[string]bool {
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, e := range epics {
		for _, t := range e.Tickets {
			if st := schema.Status(t.Status); st == schema.StatusPaused {
				label, _, _ := worktree.IterationIdentity(filepath.Base(e.Path), t.Identifier, "")
				out[label] = true
			}
		}
	}
	return out
}
