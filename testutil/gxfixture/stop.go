package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// pidFileName sits in the fixture's root dir while it runs.
const pidFileName = "fixture.pid"

// pidRecord is the pid file's content: enough for `stop` to match a fixture by
// scenario name or pid, and to tell whose worktree it belongs to.
type pidRecord struct {
	PID      int    `json:"pid"`
	Scenario string `json:"scenario"`
	Worktree string `json:"worktree"`
}

// writePidFile records the fixture in its root dir and returns the remover.
func writePidFile(root, scenario string) (remove func(), err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(pidRecord{PID: os.Getpid(), Scenario: scenario, Worktree: cwd})
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, pidFileName)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

// stopFixture sends SIGTERM (via signal) to every fixture under the temp roots
// matching target (a scenario name or a pid) that was started from worktree. It
// refuses when the only matches belong to another worktree, so one session never
// kills a sibling's fixture.
func stopFixture(target, worktree string, roots []string, signal func(pid int) error) error {
	var ours, others []pidRecord
	for _, root := range roots {
		files, _ := filepath.Glob(filepath.Join(root, "gxf*", pidFileName))
		for _, file := range files {
			b, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			var r pidRecord
			if json.Unmarshal(b, &r) != nil || r.PID == 0 {
				continue
			}
			if r.Scenario != target && strconv.Itoa(r.PID) != target {
				continue
			}
			if r.Worktree == worktree {
				ours = append(ours, r)
			} else {
				others = append(others, r)
			}
		}
	}
	if len(ours) == 0 {
		if len(others) > 0 {
			return fmt.Errorf("refusing to stop %q (pid %d): it belongs to the worktree %s, not %s", target, others[0].PID, others[0].Worktree, worktree)
		}
		return fmt.Errorf("no running fixture matches %q", target)
	}
	for _, r := range ours {
		if err := signal(r.PID); err != nil {
			return fmt.Errorf("stop pid %d: %w", r.PID, err)
		}
		fmt.Printf("stopped %s (pid %d)\n", r.Scenario, r.PID)
	}
	return nil
}

// runStop is the `stop` subcommand: it looks for fixtures under the temp dirs
// ShortTempDir may have used.
func runStop(target string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	roots := []string{"/tmp"}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		roots = append(roots, tmp)
	}
	return stopFixture(target, cwd, roots, func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) })
}
