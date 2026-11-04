package server

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/server/wire"
)

// limitsFile keeps the last Claude rate limits, beside but apart from the
// usage-gate files. Shape (a missing window is omitted):
//
//	{"five_hour": {"used_percentage": 34, "resets_at": 1790000000, "received_at": 1789990000},
//	 "seven_day": {...}}
const limitsFile = "claude-limits.json"

// limitsStore holds the last Claude limits and persists them in the state dir.
type limitsStore struct {
	path string

	mu    sync.Mutex
	state wire.LimitsReport
}

// openLimitsStore loads the state file; a missing or corrupt file means empty.
func openLimitsStore(stateDir string) *limitsStore {
	s := &limitsStore{path: filepath.Join(stateDir, limitsFile)}
	if data, err := os.ReadFile(s.path); err == nil {
		if json.Unmarshal(data, &s.state) != nil {
			s.state = wire.LimitsReport{}
		}
	}
	return s
}

// read returns a copy of the stored windows.
func (s *limitsStore) read() wire.LimitsReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyLimits(s.state)
}

func copyLimits(r wire.LimitsReport) wire.LimitsReport {
	var out wire.LimitsReport
	if r.FiveHour != nil {
		w := *r.FiveHour
		out.FiveHour = &w
	}
	if r.SevenDay != nil {
		w := *r.SevenDay
		out.SevenDay = &w
	}
	return out
}

// apply merges a report window by window and saves when anything changed. now
// is epoch seconds. The state is only updated once the save succeeds.
func (s *limitsStore) apply(report wire.LimitsReport, now int64) (wire.LimitsApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := copyLimits(s.state)
	var res wire.LimitsApplyResult
	touched := false
	for _, c := range []struct {
		in   *wire.LimitWindow
		cur  **wire.LimitWindow
		into **wire.LimitResult
	}{
		{report.FiveHour, &next.FiveHour, &res.FiveHour},
		{report.SevenDay, &next.SevenDay, &res.SevenDay},
	} {
		if c.in == nil {
			continue
		}
		r := mergeLimitWindow(c.cur, *c.in, now)
		*c.into = &r
		touched = touched || r.Outcome != wire.LimitDropped
	}
	if !touched {
		return res, nil
	}
	data, err := json.Marshal(next)
	if err != nil {
		return res, err
	}
	if err := fsutil.WriteFilePrivateAtomic(s.path, data); err != nil {
		return res, err
	}
	s.state = next
	return res, nil
}

// mergeLimitWindow applies the order-safe rule to one window.
func mergeLimitWindow(cur **wire.LimitWindow, in wire.LimitWindow, now int64) wire.LimitResult {
	stored := *cur
	in.ReceivedAt = now
	replace := func(outcome wire.LimitOutcome, prev int64) wire.LimitResult {
		w := in
		*cur = &w
		return wire.LimitResult{Outcome: outcome, PrevReceivedAt: prev}
	}
	if stored == nil {
		return replace(wire.LimitNew, 0)
	}
	prev := stored.ReceivedAt
	switch {
	case in.ResetsAt == stored.ResetsAt:
		if in.UsedPercentage < stored.UsedPercentage {
			return wire.LimitResult{Outcome: wire.LimitDropped, PrevReceivedAt: prev}
		}
		outcome := wire.LimitConfirmed
		if math.Round(in.UsedPercentage) != math.Round(stored.UsedPercentage) {
			outcome = wire.LimitChanged
		}
		return replace(outcome, prev)
	case in.ResetsAt > stored.ResetsAt || stored.ResetsAt <= now:
		return replace(wire.LimitChanged, prev)
	}
	return wire.LimitResult{Outcome: wire.LimitDropped, PrevReceivedAt: prev}
}
