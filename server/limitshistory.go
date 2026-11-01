package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/codexsession"
)

// limitHistoryFile keeps every limit sample, beside claude-limits.json.
const limitHistoryFile = "limit-history.json"

// Limit ids in the history.
const (
	LimitIDClaude5h     = "claude-5h"
	LimitIDClaudeWeekly = "claude-weekly"
	LimitIDCodex5h      = "codex-5h"
	LimitIDCodexWeekly  = "codex-weekly"
)

// limitConfirmEverySeconds is how often an unchanged limit still saves a
// confirming sample.
const limitConfirmEverySeconds = 3600

// LimitSample is one saved reading; times are epoch seconds. UsedPercentage is
// the raw number, not rounded.
type LimitSample struct {
	LimitID        string  `json:"limit_id"`
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
	ReceivedAt     int64   `json:"received_at"`
}

// limitHistoryState is the history file. Latest holds the newest reading seen
// per limit, which may be newer than its last sample.
type limitHistoryState struct {
	Samples map[string][]LimitSample `json:"samples"`
	Latest  map[string]LimitSample   `json:"latest"`
}

// limitHistory holds every limit sample and persists them in the state dir.
// Samples are never pruned.
type limitHistory struct {
	path string

	mu    sync.Mutex
	state limitHistoryState
	// codexSeen is the newest rollout time noted per Codex limit; not persisted.
	codexSeen map[string]int64
}

// openLimitHistory loads the history file; a missing or corrupt file means empty.
func openLimitHistory(stateDir string) *limitHistory {
	h := &limitHistory{path: filepath.Join(stateDir, limitHistoryFile), codexSeen: map[string]int64{}}
	if data, err := os.ReadFile(h.path); err == nil {
		if json.Unmarshal(data, &h.state) != nil {
			h.state = limitHistoryState{}
		}
	}
	if h.state.Samples == nil {
		h.state.Samples = map[string][]LimitSample{}
	}
	if h.state.Latest == nil {
		h.state.Latest = map[string]LimitSample{}
	}
	return h
}

// Samples returns a copy of one limit's samples, oldest first.
func (h *limitHistory) Samples(id string) []LimitSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]LimitSample(nil), h.state.Samples[id]...)
}

// FirstSampleAt is the receive time of a limit's oldest sample.
func (h *limitHistory) FirstSampleAt(id string) (int64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s := h.state.Samples[id]; len(s) > 0 {
		return s[0].ReceivedAt, true
	}
	return 0, false
}

// Latest is the newest reading seen for a limit.
func (h *limitHistory) Latest(id string) (LimitSample, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l, ok := h.state.Latest[id]
	return l, ok
}

// record notes a reading received at now. It saves a sample when the raw
// percentage or the reset time differs from the limit's last sample, or when
// the last sample is an hour old. The in-memory state only moves once the save
// succeeds.
func (h *limitHistory) record(id string, w LimitWindow, now int64) (saved bool, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sample := LimitSample{LimitID: id, UsedPercentage: w.UsedPercentage, ResetsAt: w.ResetsAt, ReceivedAt: now}
	samples := h.state.Samples[id]
	if n := len(samples); n > 0 {
		last := samples[n-1]
		unchanged := last.UsedPercentage == sample.UsedPercentage && last.ResetsAt == sample.ResetsAt
		if unchanged && now-last.ReceivedAt < limitConfirmEverySeconds {
			h.state.Latest[id] = sample
			return false, nil
		}
	}
	next := limitHistoryState{Samples: map[string][]LimitSample{}, Latest: map[string]LimitSample{}}
	for k, v := range h.state.Samples {
		next.Samples[k] = v
	}
	for k, v := range h.state.Latest {
		next.Latest[k] = v
	}
	next.Samples[id] = append(samples[:len(samples):len(samples)], sample)
	next.Latest[id] = sample
	data, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	if err := writeFileAtomic(h.path, data); err != nil {
		return false, err
	}
	h.state = next
	return true, nil
}

// newerCodexReading reports whether a reading seen at seen is newer than the
// last one noted for the limit, and notes it. The newest seen time lives in
// memory because the scan time stored as received-at can run ahead of a reading
// that landed in the same second; after a restart the stored received-at stands
// in for it.
func (h *limitHistory) newerCodexReading(id string, seen int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	last, ok := h.codexSeen[id]
	if !ok {
		last = h.state.Latest[id].ReceivedAt
	}
	if seen <= last {
		return false
	}
	h.codexSeen[id] = seen
	return true
}

// codexLimitIDs maps the two window lengths in minutes the forecast knows to
// their limit ids; a window of any other length is not saved.
var codexLimitIDs = map[int]string{300: LimitIDCodex5h, 10080: LimitIDCodexWeekly}

// recordCodex saves history from one scan's Codex samples, merged across
// agents per limit: the later reset wins, and among equal resets the highest
// percentage. A limit is only noted when a reading is newer than its latest
// received-at, so an idle Codex does not get fresher. It reports whether any
// sample was saved.
func (h *limitHistory) recordCodex(samples []codexsession.UsageSample, now int64) (bool, error) {
	type reading struct {
		w    LimitWindow
		seen int64
	}
	merged := map[string]reading{}
	for _, s := range samples {
		for _, cw := range s.Windows {
			id, ok := codexLimitIDs[cw.Minutes]
			if !ok {
				continue
			}
			r := reading{w: LimitWindow{UsedPercentage: cw.UsedPercent, ResetsAt: cw.ResetsAt.Unix()}, seen: s.SeenAt.Unix()}
			cur, have := merged[id]
			switch {
			case !have, r.w.ResetsAt > cur.w.ResetsAt:
				merged[id] = r
			case r.w.ResetsAt == cur.w.ResetsAt:
				cur.w.UsedPercentage = max(cur.w.UsedPercentage, r.w.UsedPercentage)
				cur.seen = max(cur.seen, r.seen)
				merged[id] = cur
			}
		}
	}
	var saved bool
	var errs []error
	for _, id := range []string{LimitIDCodex5h, LimitIDCodexWeekly} {
		r, ok := merged[id]
		if !ok {
			continue
		}
		if !h.newerCodexReading(id, r.seen) {
			continue
		}
		s, err := h.record(id, r.w, now)
		saved = saved || s
		if err != nil {
			errs = append(errs, err)
		}
	}
	return saved, errors.Join(errs...)
}

// recordClaude saves history for each window the merge accepted.
func (h *limitHistory) recordClaude(report LimitsReport, res LimitsApplyResult, now int64) error {
	for _, c := range []struct {
		id  string
		in  *LimitWindow
		res *LimitResult
	}{
		{LimitIDClaude5h, report.FiveHour, res.FiveHour},
		{LimitIDClaudeWeekly, report.SevenDay, res.SevenDay},
	} {
		if c.in == nil || c.res == nil || c.res.Outcome == LimitDropped {
			continue
		}
		if _, err := h.record(c.id, *c.in, now); err != nil {
			return err
		}
	}
	return nil
}
