package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const (
	// usageLimitPercent is the used percentage at which a window counts as at
	// its limit. Fixed on purpose: no config key.
	usageLimitPercent = 98
	// defaultUsagePoll is slow on purpose: a record is one small file per agent
	// dir and a limit only matters to the next claim.
	defaultUsagePoll = time.Minute
	usageSampleFile  = "usage-sample.json"
)

// usageSamples keeps the newest usage record any PTY agent wrote, across all
// projects. It is persisted in the state dir so a restart keeps it.
type usageSamples struct {
	path string

	mu     sync.Mutex
	sample *nativerunner.UsageRecord
}

// openUsageSamples loads the persisted sample; a missing or unreadable file
// means no sample.
func openUsageSamples(stateDir string) *usageSamples {
	u := &usageSamples{path: filepath.Join(stateDir, usageSampleFile)}
	if data, err := os.ReadFile(u.path); err == nil {
		var rec nativerunner.UsageRecord
		if json.Unmarshal(data, &rec) == nil {
			u.sample = &rec
		}
	}
	return u
}

// offer replaces the sample when rec is newer, and persists it.
func (u *usageSamples) offer(rec nativerunner.UsageRecord) (bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.sample != nil && !rec.SeenAt.After(u.sample.SeenAt) {
		return false, nil
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return false, err
	}
	if err := writeFileAtomic(u.path, data); err != nil {
		return false, err
	}
	u.sample = &rec
	return true, nil
}

// readUsageRecord reads the usage record in an agent dir; ok is false when the
// agent wrote none or it is unreadable.
func readUsageRecord(agentDir string) (nativerunner.UsageRecord, bool) {
	var rec nativerunner.UsageRecord
	data, err := os.ReadFile(filepath.Join(agentDir, nativerunner.UsageFile))
	if err != nil || json.Unmarshal(data, &rec) != nil || rec.SeenAt.IsZero() {
		return rec, false
	}
	return rec, true
}

// scan offers the record of every agent dir under agentsRoot.
func (u *usageSamples) scan(agentsRoot string) error {
	entries, err := os.ReadDir(agentsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if rec, ok := readUsageRecord(filepath.Join(agentsRoot, e.Name())); ok {
			if _, err := u.offer(rec); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// atLimit returns the window of the sample at 98% or more whose reset is still
// ahead of now. A window past its reset is not at limit even if no newer
// sample arrived. The five-hour window is preferred when both are at limit.
func (u *usageSamples) atLimit(now time.Time) (*nativerunner.UsageWindow, bool) {
	_, w, ok := u.limitWindow(now)
	return w, ok
}

// limitWindow is atLimit plus the window's name as the gate wording uses it.
func (u *usageSamples) limitWindow(now time.Time) (string, *nativerunner.UsageWindow, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.sample == nil {
		return "", nil, false
	}
	windows := []struct {
		name string
		w    *nativerunner.UsageWindow
	}{{"5-hour", u.sample.FiveHour}, {"weekly", u.sample.SevenDay}}
	for _, c := range windows {
		if c.w != nil && c.w.UsedPercentage >= usageLimitPercent && time.Unix(c.w.ResetsAt, 0).After(now) {
			return c.name, c.w, true
		}
	}
	return "", nil, false
}

// usageGate holds a new Claude claim while a usage window is at its limit.
// Extra usage does not lift it. The verdict is the whole phrase, like
// rateLimitVerdict.
func (s *Server) usageGate(agent ralphloop.AgentKind, now time.Time) (string, bool) {
	if agent != ralphloop.AgentClaude {
		return "", false
	}
	name, w, ok := s.usage.limitWindow(now)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("usage at %.0f%% of the %s limit, resets %s", w.UsedPercentage, name,
		time.Unix(w.ResetsAt, 0).UTC().Format(time.RFC3339)), true
}

// usageWindowAtLimit is the window at its limit now, if any.
func (s *Server) usageWindowAtLimit(now time.Time) (*nativerunner.UsageWindow, bool) {
	return s.usage.atLimit(now)
}

// scanUsage offers every project's PTY agent records to the sample.
func (s *Server) scanUsage() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Warn("scan usage", "err", err)
		return
	}
	for _, dir := range dirs {
		root := nativerunner.AgentsRoot(s.cfg.StateDir, tickets.ProjectName(dir))
		if err := s.usage.scan(root); err != nil {
			s.log.Warn("scan usage", "root", root, "err", err)
		}
	}
}

// keepUsageScanned reads the agent records on a slow timer until ctx ends.
func (s *Server) keepUsageScanned(ctx context.Context) {
	interval := s.cfg.UsagePollInterval
	if interval <= 0 {
		interval = defaultUsagePoll
	}
	s.scanUsage()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.scanUsage()
		}
	}
}
