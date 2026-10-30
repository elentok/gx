package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	usageLiftFile    = "usage-override.json"
)

// usageSamples keeps the newest usage record any PTY agent wrote, across all
// projects. It is persisted in the state dir so a restart keeps it.
type usageSamples struct {
	path     string
	liftPath string

	mu     sync.Mutex
	sample *nativerunner.UsageRecord
	// lifted maps a window name to the resets_at the override covers. A window
	// is keyed by its reset, so the next window gates again with no cleanup.
	lifted map[string]int64
}

// openUsageSamples loads the persisted sample and override; a missing or
// unreadable file means none.
func openUsageSamples(stateDir string) *usageSamples {
	u := &usageSamples{path: filepath.Join(stateDir, usageSampleFile), liftPath: filepath.Join(stateDir, usageLiftFile)}
	if data, err := os.ReadFile(u.path); err == nil {
		var rec nativerunner.UsageRecord
		if json.Unmarshal(data, &rec) == nil {
			u.sample = &rec
		}
	}
	if data, err := os.ReadFile(u.liftPath); err == nil {
		_ = json.Unmarshal(data, &u.lifted)
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
	for _, c := range u.atLimitLocked(now) {
		if u.lifted[c.name] != c.w.ResetsAt {
			return c.name, c.w, true
		}
	}
	return "", nil, false
}

type namedWindow struct {
	name string
	w    *nativerunner.UsageWindow
}

// atLimitLocked lists the windows at their limit with a reset still ahead,
// lifted or not, five-hour first.
func (u *usageSamples) atLimitLocked(now time.Time) []namedWindow {
	if u.sample == nil {
		return nil
	}
	var out []namedWindow
	for _, c := range []namedWindow{{"5-hour", u.sample.FiveHour}, {"weekly", u.sample.SevenDay}} {
		if c.w != nil && c.w.UsedPercentage >= usageLimitPercent && time.Unix(c.w.ResetsAt, 0).After(now) {
			out = append(out, c)
		}
	}
	return out
}

// lift covers every window now gating until its reset and persists that. It
// returns the lifted window names and the latest reset among them; none means
// nothing was gating.
func (u *usageSamples) lift(now time.Time) ([]string, time.Time, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	next := map[string]int64{}
	for k, v := range u.lifted {
		next[k] = v
	}
	var names []string
	var until int64
	for _, c := range u.atLimitLocked(now) {
		if next[c.name] == c.w.ResetsAt {
			continue
		}
		next[c.name] = c.w.ResetsAt
		names = append(names, c.name)
		until = max(until, c.w.ResetsAt)
	}
	if len(names) == 0 {
		return nil, time.Time{}, nil
	}
	data, err := json.Marshal(next)
	if err != nil {
		return nil, time.Time{}, err
	}
	if err := writeFileAtomic(u.liftPath, data); err != nil {
		return nil, time.Time{}, err
	}
	u.lifted = next
	return names, time.Unix(until, 0), nil
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

// UsageOverrideResult is the outcome of lifting the usage gate: the windows
// lifted and when the lift ends, or a refusal.
type UsageOverrideResult struct {
	Windows []string  `json:"windows,omitempty"`
	Until   time.Time `json:"until,omitzero"`
	Refused bool      `json:"refused,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message,omitempty"`
}

// ReasonNotGated refuses a usage override when no window is gating.
const ReasonNotGated = "not-gated"

// usageOverride lets new claims start until the gating windows reset.
func (s *Server) usageOverride(now time.Time) (UsageOverrideResult, error) {
	names, until, err := s.usage.lift(now)
	if err != nil {
		return UsageOverrideResult{}, err
	}
	if len(names) == 0 {
		return UsageOverrideResult{Refused: true, Reason: ReasonNotGated, Message: "no usage window is at its limit"}, nil
	}
	s.events.publish(EventQueueChanged, "")
	s.kickRunner()
	return UsageOverrideResult{Windows: names, Until: until}, nil
}

func (s *Server) usageOverrideWrite(w http.ResponseWriter, _ *http.Request) {
	res, err := s.usageOverride(time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
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
