package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const (
	// defaultUsagePoll is slow on purpose: a record is one small file per agent
	// dir and a limit only matters to the next claim.
	defaultUsagePoll = time.Minute
	// Claude's files keep their names so an upgrade loads them; Codex's sit
	// beside them.
	usageSampleFile      = "usage-sample.json"
	usageLiftFile        = "usage-override.json"
	usageSampleFileCodex = "usage-sample-codex.json"
	usageLiftFileCodex   = "usage-override-codex.json"
)

// usageSamples keeps the newest usage record any PTY agent of one harness
// wrote, across all projects. It is persisted in the state dir so a restart
// keeps it.
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
	return openUsageFiles(stateDir, usageSampleFile, usageLiftFile)
}

func openUsageFiles(stateDir, sampleFile, liftFile string) *usageSamples {
	u := &usageSamples{path: filepath.Join(stateDir, sampleFile), liftPath: filepath.Join(stateDir, liftFile)}
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

// scan offers the record read of every agent dir under agentsRoot.
func (u *usageSamples) scan(agentsRoot string, read func(agentDir string) (nativerunner.UsageRecord, bool)) error {
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
		if rec, ok := read(filepath.Join(agentsRoot, e.Name())); ok {
			if _, err := u.offer(rec); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// limitWindow returns the unlifted window of the sample at the usage limit
// whose reset is still ahead of now, with its name as the gate wording uses
// it. A window past its reset is not at limit even if no newer sample arrived.
// The first window is preferred when both are at limit.
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
		if c.w != nil && c.w.UsedPercentage >= nativerunner.UsageLimitPercent && time.Unix(c.w.ResetsAt, 0).After(now) {
			if c.w.Name != "" {
				c.name = c.w.Name
			}
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

// usageFor is the sample and verdict prefix of a harness; nil for a harness
// with no usage gate.
func (s *Server) usageFor(agent ralphloop.AgentKind) (*usageSamples, string) {
	switch agent {
	case ralphloop.AgentClaude:
		return s.usage, VerdictUsagePrefixClaude
	case ralphloop.AgentCodex:
		return s.usageCodex, VerdictUsagePrefixCodex
	}
	return nil, ""
}

// usageGate holds a new claim of a harness while one of that harness's usage
// windows is at its limit. Extra usage does not lift it. The verdict is the
// whole phrase, like rateLimitVerdict.
func (s *Server) usageGate(agent ralphloop.AgentKind, now time.Time) (string, bool) {
	samples, prefix := s.usageFor(agent)
	if samples == nil {
		return "", false
	}
	name, w, ok := samples.limitWindow(now)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s%.0f%% of the %s limit, resets %s", prefix, w.UsedPercentage, name,
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

// UsageOverrideRequest names the harness whose gate to lift; empty means the
// Default harness.
type UsageOverrideRequest struct {
	Harness string `json:"harness,omitempty"`
}

// usageOverride lets new claims of a harness start until its gating windows
// reset. It lifts that harness only.
func (s *Server) usageOverride(now time.Time, agent ralphloop.AgentKind) (UsageOverrideResult, error) {
	samples, prefix := s.usageFor(agent)
	var names []string
	var until time.Time
	if samples != nil {
		var err error
		if names, until, err = samples.lift(now); err != nil {
			return UsageOverrideResult{}, err
		}
	}
	if len(names) == 0 {
		return UsageOverrideResult{Refused: true, Reason: ReasonNotGated,
			Message: fmt.Sprintf("no %s usage window is at its limit", strings.TrimSuffix(prefix, " usage: "))}, nil
	}
	s.events.publish(EventQueueChanged, "")
	s.kickRunner()
	return UsageOverrideResult{Windows: names, Until: until}, nil
}

func (s *Server) usageOverrideWrite(w http.ResponseWriter, r *http.Request) {
	var req UsageOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	agent := defaultOrClaude(s.settings.Load().AgentsDefaultHarness)
	if req.Harness != "" {
		agent = ralphloop.AgentKind(req.Harness)
		if samples, _ := s.usageFor(agent); samples == nil {
			http.Error(w, fmt.Sprintf("harness %q has no usage gate", req.Harness), http.StatusBadRequest)
			return
		}
	}
	res, err := s.usageOverride(time.Now(), agent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// scanUsage offers every project's PTY agent records to the samples of their
// harness.
func (s *Server) scanUsage() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Warn("scan usage", "err", err)
		return
	}
	for _, dir := range dirs {
		root := nativerunner.AgentsRoot(s.cfg.StateDir, tickets.ProjectName(dir))
		if err := s.usage.scan(root, readUsageRecord); err != nil {
			s.log.Warn("scan usage", "root", root, "err", err)
		}
		if err := s.usageCodex.scan(root, s.readCodexUsage); err != nil {
			s.log.Warn("scan codex usage", "root", root, "err", err)
		}
	}
}

// readCodexUsage is the usage record of a Codex agent dir, read from its
// rollout; Codex agents write no usage.json. A dir of another harness has none.
func (s *Server) readCodexUsage(agentDir string) (nativerunner.UsageRecord, bool) {
	d, err := nativerunner.OpenDir(agentDir)
	if err != nil || agentrunner.Kind(d.Meta.Harness) != harness.Codex {
		return nativerunner.UsageRecord{}, false
	}
	home, err := s.codexHome()
	if err != nil {
		return nativerunner.UsageRecord{}, false
	}
	sample, ok, err := codexsession.LatestUsageIn(home, d.Meta.Cwd, d.Meta.SessionID)
	if err != nil || !ok || sample.SeenAt.IsZero() {
		return nativerunner.UsageRecord{}, false
	}
	return codexUsageRecord(sample), true
}

// codexUsageRecord maps a sample's windows onto the two slots of the Claude
// gate: windows up to 5 hours into the first, longer ones into the second, the
// fuller window when two share a slot. Each keeps its own name.
func codexUsageRecord(sample codexsession.UsageSample) nativerunner.UsageRecord {
	rec := nativerunner.UsageRecord{SeenAt: sample.SeenAt}
	for _, w := range sample.Windows {
		slot := &rec.FiveHour
		if w.Minutes > 300 {
			slot = &rec.SevenDay
		}
		if *slot == nil || (*slot).UsedPercentage < w.UsedPercent {
			*slot = &nativerunner.UsageWindow{UsedPercentage: w.UsedPercent, ResetsAt: w.ResetsAt.Unix(), Name: codexWindowName(w.Minutes)}
		}
	}
	return rec
}

// codexWindowName names a window by its length: 300 minutes is "5-hour", 10080
// is "weekly", anything else by its days, hours or minutes.
func codexWindowName(minutes int) string {
	switch {
	case minutes == 300:
		return "5-hour"
	case minutes == 10080:
		return "weekly"
	case minutes%1440 == 0:
		return fmt.Sprintf("%d-day", minutes/1440)
	case minutes%60 == 0:
		return fmt.Sprintf("%d-hour", minutes/60)
	}
	return fmt.Sprintf("%d-minute", minutes)
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
