package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// seedLeftoverRuns claims each LeftoverRun's ticket and writes its worktree and
// runs.json entry, so the server's start-up recovery finds a run whose runner it
// cannot build. It runs after prepareStore wrote the store and the repo.
func seedLeftoverRuns(f *fixture, sc scenario) error {
	var queue []map[string]string
	for _, lr := range sc.LeftoverRuns {
		tg, err := leftoverTarget(lr.Address)
		if err != nil {
			return err
		}
		var work func(string) error
		if lr.Commit {
			work = func(wt string) error {
				if err := os.WriteFile(filepath.Join(wt, "agent.txt"), []byte("work"), 0o644); err != nil {
					return err
				}
				if err := f.git(wt, "add", "-A"); err != nil {
					return err
				}
				return f.git(wt, "commit", "-q", "-m", "agent work")
			}
		}
		runner := lr.Runner
		_, _, err = servertest.StageLeftover(f.stateD, f.store, filepath.Join(f.root, "repo"), tg, work, func(servertest.Run) map[string]any {
			return map[string]any{"runner": runner}
		})
		if err != nil {
			return fmt.Errorf("stage leftover run %s: %w", lr.Address, err)
		}
		queue = append(queue, map[string]string{"address": tg.Project + ":" + tg.Epic, "agent": "claude"})
	}
	if len(queue) == 0 {
		return nil
	}
	data, err := json.Marshal(queue)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.stateD, projects.QueueFileName), data, 0o600)
}

// appendRawRunLog appends lines to the run log at path without runlog.AppendEvent,
// which refuses the removed types and kinds these lines exist to carry.
func appendRawRunLog(path string, lines []spec.RunLogLine) error {
	if len(lines) == 0 {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, l := range lines {
		data, err := json.Marshal(runlog.Event{Time: time.Now(), Type: l.Type, Kind: l.Kind, Reason: l.Reason})
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// leftoverTarget splits project:epic/id.
func leftoverTarget(addr string) (servertest.Target, error) {
	proj, rest, ok := strings.Cut(addr, ":")
	epic, id, ok2 := strings.Cut(rest, "/")
	if !ok || !ok2 {
		return servertest.Target{}, fmt.Errorf("leftover run address %q is not project:epic/id", addr)
	}
	return servertest.Target{Project: proj, Epic: epic, Ticket: id}, nil
}
