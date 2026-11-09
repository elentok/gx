package health

import (
	"context"
	"fmt"
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/procwatch"
	"github.com/elentok/gx/server/runs"
)

const stormBannerGroup = "gx-process-storm"

// stormWatch is the state the process-storm watchdog keeps between polls.
type stormWatch struct {
	mu      sync.Mutex
	byRoot  map[int]runs.TrackedRun // the roots the last poll asked for
	handled map[string]bool         // runs already killed, until they leave the registry
}

// StormRoots returns the owner pids of every run in list, and which run each
// belongs to. A run whose runner is missing or fails is skipped.
func StormRoots(list []runs.TrackedRun, runnerFor func(runs.Run) agentrunner.Runner) ([]int, map[int]runs.TrackedRun) {
	var roots []int
	byRoot := map[int]runs.TrackedRun{}
	for _, t := range list {
		runner := runnerFor(t.Run)
		if runner == nil {
			continue
		}
		pids, err := agentrunner.OwnerPIDs(runner, t.Session)
		if err != nil {
			continue
		}
		for _, p := range pids {
			if _, dup := byRoot[p.PID]; !dup {
				roots = append(roots, p.PID)
			}
			byRoot[p.PID] = t
		}
	}
	return roots, byRoot
}

// KeepStormWatched stops runs whose process tree outgrows watchdog.max-tree-processes
// until ctx ends. The limits are read before each poll, so a reload applies live.
func (s *Health) KeepStormWatched(ctx context.Context) {
	w := &procwatch.Watcher{
		Settings: func() procwatch.Config {
			wd := s.settings.Load().Watchdog
			return procwatch.Config{Threshold: wd.MaxTreeProcesses, Interval: wd.Interval}
		},
		Roots:  s.stormRoots,
		Report: s.stormFound,
	}
	w.Run(ctx)
}

func (s *Health) stormRoots() []int {
	list := s.registry.List()
	roots, byRoot := StormRoots(list, s.harness.RunnerForRun)
	s.storm.mu.Lock()
	defer s.storm.mu.Unlock()
	s.storm.byRoot = byRoot
	live := map[string]bool{}
	for _, t := range list {
		live[t.Address] = true
	}
	for addr := range s.storm.handled {
		if !live[addr] {
			delete(s.storm.handled, addr)
		}
	}
	return roots
}

// stormFound marks the run first, so its own settle cannot pause it as
// something else, then kills the tree and settles the pause.
func (s *Health) stormFound(st procwatch.Storm) {
	s.storm.mu.Lock()
	t, ok := s.storm.byRoot[st.Root]
	if !ok || s.storm.handled[t.Address] {
		s.storm.mu.Unlock()
		return
	}
	if s.storm.handled == nil {
		s.storm.handled = map[string]bool{}
	}
	s.storm.handled[t.Address] = true
	s.storm.mu.Unlock()

	threshold := s.settings.Load().Watchdog.MaxTreeProcesses
	stopper := s.stopper()
	stopper.MarkStorm(t.Address, st.Descendants, threshold)
	if err := (procwatch.Killer{}).Kill(st.Root); err != nil {
		s.log.Error("process storm: kill tree", "ticket", t.Address, "root", st.Root, "err", err)
	}
	if _, err := stopper.SettleStop(t); err != nil {
		s.log.Error("process storm: settle stop", "ticket", t.Address, "err", err)
	}
	s.log.Warn("process storm: run killed", "ticket", t.Address, "processes", st.Descendants, "threshold", threshold)
	note := osnotify.Notification{
		Title: "Process storm stopped " + t.Address,
		Body:  fmt.Sprintf("%s was killed: its process tree had %d processes (limit %d).", t.Address, st.Descendants, threshold),
		Group: stormBannerGroup,
	}
	if err := s.notifier.Notify(context.Background(), note); err != nil {
		s.log.Warn("process storm: banner", "err", err)
	}
}
