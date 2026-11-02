package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/elentok/gx/check"
)

// checkHolderRun is the run id the waiting file names as the lock holder.
const checkHolderRun = "run-holding-the-lock"

// writeCheckWaiting writes the live waiting state `gx check --full` leaves while
// it waits for the check lock, for the held fake agent's worktree. The file is
// keyed by the agent's pid, which lives as long as the hold, so the server keeps
// reading it until the flow removes the file. It returns the file's path.
func (f *fixture) writeCheckWaiting(ctx context.Context) (string, error) {
	m, err := f.waitAgentMeta(ctx)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(f.stateD, "check-waiting")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.Marshal(check.Waiting{PID: m.PID, HeldBy: checkHolderRun, Since: time.Now().UTC().Format(time.RFC3339), Root: m.Cwd})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, strconv.Itoa(m.PID)+".json")
	return path, os.WriteFile(path, data, 0o644)
}
