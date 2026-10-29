package herdrrunner

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/syshealth"
)

var _ agentrunner.ProcessOwner = (*Runner)(nil)

// OwnerPIDs returns the pane's shell pid with its start time. A pane's shell
// pid never changes, so each pane costs one herdr exec and one start-time
// read; a failure caches nothing and is retried on the next call.
func (r *Runner) OwnerPIDs(s agentrunner.Session) ([]agentrunner.OwnerPID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if owner, ok := r.shells[s.ID]; ok {
		return []agentrunner.OwnerPID{owner}, nil
	}
	pid, err := r.CLI.PaneShellPID(s.ID)
	if err != nil {
		return nil, err
	}
	start := r.PIDStart
	if start == nil {
		start = syshealth.ProcessStart
	}
	startUnix, err := start(pid)
	if err != nil {
		return nil, err
	}
	owner := agentrunner.OwnerPID{PID: pid, Start: startUnix}
	if r.shells == nil {
		r.shells = map[string]agentrunner.OwnerPID{}
	}
	r.shells[s.ID] = owner
	return []agentrunner.OwnerPID{owner}, nil
}
