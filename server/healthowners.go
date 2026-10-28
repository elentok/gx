package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/syshealth"
)

// ownerCache holds the holder pings of live sessions, one entry per session
// run, so a process read does not ping every holder each time.
type ownerCache struct {
	mu      sync.Mutex
	entries map[string][]syshealth.OwnerKey
}

// pingHolder names the pids and start times a session's holder reports. It
// fails when the holder is gone or predates the extended ping.
type pingHolder func(dir string) ([]syshealth.OwnerKey, error)

func dialHolderOwners(dir string) ([]syshealth.OwnerKey, error) {
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return nil, err
	}
	var keys []syshealth.OwnerKey
	if start, ok := c.HolderStart(); ok {
		keys = append(keys, syshealth.OwnerKey{PID: c.HolderPID(), StartUnix: start})
	}
	if start, ok := c.ChildStart(); ok {
		keys = append(keys, syshealth.OwnerKey{PID: c.ChildPID(), StartUnix: start})
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("holder sends no start times")
	}
	return keys, nil
}

// healthOwners is the owner map for one process read: tracked runs, live
// sessions and the gx server itself.
func (s *Server) healthOwners(t syshealth.ProcessTable) map[syshealth.OwnerKey]syshealth.Owner {
	owners := serverOwners(t)
	if owners == nil {
		owners = map[syshealth.OwnerKey]syshealth.Owner{}
	}
	s.addRunOwners(owners)
	s.addSessionOwners(owners, dialHolderOwners)
	return owners
}

func (s *Server) addRunOwners(owners map[syshealth.OwnerKey]syshealth.Owner) {
	for _, t := range s.registry.list() {
		runner := s.runnerForRun(t.Run)
		if runner == nil {
			s.log.Warn("health owners: no runner for run", "address", t.Address, "runner", t.Runner)
			continue
		}
		pids, err := agentrunner.OwnerPIDs(runner, t.Session)
		if err != nil {
			s.log.Warn("health owners: run pids", "address", t.Address, "err", err)
			continue
		}
		for _, p := range pids {
			owners[syshealth.OwnerKey{PID: p.PID, StartUnix: p.Start}] = syshealth.Owner{Kind: syshealth.OwnerTicket, Name: t.Address}
		}
	}
}

func (s *Server) addSessionOwners(owners map[syshealth.OwnerKey]syshealth.Owner, ping pingHolder) {
	entries, err := os.ReadDir(s.cfg.SessionsDir)
	if err != nil {
		return
	}
	c := &s.ownerCache
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(s.cfg.SessionsDir, e.Name())
		meta, err := readSessionMeta(dir)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("%s#%d", e.Name(), meta.Run)
		seen[key] = true
		keys, ok := c.entries[key]
		if !ok {
			if keys, err = ping(dir); err != nil {
				continue
			}
			if c.entries == nil {
				c.entries = map[string][]syshealth.OwnerKey{}
			}
			c.entries[key] = keys
		}
		owner := syshealth.Owner{Kind: syshealth.OwnerSession, Name: meta.Name}
		if meta.Ticket != "" {
			owner = syshealth.Owner{Kind: syshealth.OwnerTicket, Name: meta.Ticket}
		}
		for _, k := range keys {
			owners[k] = owner
		}
	}
	for key := range c.entries {
		if !seen[key] {
			delete(c.entries, key)
		}
	}
}
