package nativerunner

import (
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/syshealth"
)

// OwnerPIDs returns the claude and holder pids from the agent's meta.json.
func (p *Pty) OwnerPIDs(s agentrunner.Session) ([]agentrunner.OwnerPID, error) {
	m, err := readOwnerMeta(filepath.Join(p.Root, s.Label))
	if err != nil || m == nil {
		return nil, err
	}
	return appendOwners(nil, s.Label,
		ownerSpec{"claude", m.PID, m.PIDStart},
		ownerSpec{"holder", m.HolderPID, m.HolderPIDStart},
	), nil
}

// OwnerPIDs returns the claude pid from the agent's meta.json.
func (h *Headless) OwnerPIDs(s agentrunner.Session) ([]agentrunner.OwnerPID, error) {
	m, err := readOwnerMeta(filepath.Join(h.Root, s.Label))
	if err != nil || m == nil {
		return nil, err
	}
	return appendOwners(nil, s.Label, ownerSpec{"claude", m.PID, m.PIDStart}), nil
}

type ownerSpec struct {
	role  string
	pid   int
	start string
}

// readOwnerMeta is nil, nil when the agent's directory is gone: the agent
// ended, which is not an error for the caller.
func readOwnerMeta(dir string) (*Meta, error) {
	m, err := readMeta(dir)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// appendOwners skips unset pids and pids whose start time is missing or
// unparsable.
func appendOwners(out []agentrunner.OwnerPID, label string, specs ...ownerSpec) []agentrunner.OwnerPID {
	for _, sp := range specs {
		if sp.pid <= 0 {
			continue
		}
		start, err := syshealth.ParseLstart(sp.start)
		if err != nil {
			slog.Warn("owner pid left out: no usable start time", "label", label, "role", sp.role, "pid", sp.pid, "err", err)
			continue
		}
		out = append(out, agentrunner.OwnerPID{PID: sp.pid, Start: start})
	}
	return out
}
