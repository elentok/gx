package server

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// LimitsStaleAfterSeconds is how old a window's received-at may get before the
// web client shows it as stale.
const LimitsStaleAfterSeconds = 600

// limitsFile keeps the last Claude rate limits, beside but apart from the
// usage-gate files. Shape (a missing window is omitted):
//
//	{"five_hour": {"used_percentage": 34, "resets_at": 1790000000, "received_at": 1789990000},
//	 "seven_day": {...}}
const limitsFile = "claude-limits.json"

// LimitWindow is one rate-limit window; all times are epoch seconds.
// ReceivedAt is set by the server, never taken from a report.
type LimitWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
	ReceivedAt     int64   `json:"received_at"`
}

// LimitsReport is a report or the stored state: the windows it carries.
type LimitsReport struct {
	FiveHour *LimitWindow `json:"five_hour,omitempty"`
	SevenDay *LimitWindow `json:"seven_day,omitempty"`
}

// LimitOutcome says what a report did to one window.
type LimitOutcome string

const (
	// LimitNew filled a window that was empty.
	LimitNew LimitOutcome = "new"
	// LimitChanged moved the whole-number percentage or the reset time.
	LimitChanged LimitOutcome = "changed"
	// LimitConfirmed refreshed received-at without a visible change.
	LimitConfirmed LimitOutcome = "confirmed"
	// LimitDropped left the window alone.
	LimitDropped LimitOutcome = "dropped"
)

// LimitResult is the outcome for one window the report carried.
type LimitResult struct {
	Outcome LimitOutcome
	// PrevReceivedAt is the stored received-at before the report; 0 when none.
	PrevReceivedAt int64
}

// LimitsApplyResult has one result per window the report carried; the others
// are nil.
type LimitsApplyResult struct {
	FiveHour *LimitResult
	SevenDay *LimitResult
}

// limitsStore holds the last Claude limits and persists them in the state dir.
type limitsStore struct {
	path string

	mu    sync.Mutex
	state LimitsReport
}

// openLimitsStore loads the state file; a missing or corrupt file means empty.
func openLimitsStore(stateDir string) *limitsStore {
	s := &limitsStore{path: filepath.Join(stateDir, limitsFile)}
	if data, err := os.ReadFile(s.path); err == nil {
		if json.Unmarshal(data, &s.state) != nil {
			s.state = LimitsReport{}
		}
	}
	return s
}

// read returns a copy of the stored windows.
func (s *limitsStore) read() LimitsReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyLimits(s.state)
}

func copyLimits(r LimitsReport) LimitsReport {
	var out LimitsReport
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
func (s *limitsStore) apply(report LimitsReport, now int64) (LimitsApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := copyLimits(s.state)
	var res LimitsApplyResult
	touched := false
	for _, c := range []struct {
		in   *LimitWindow
		cur  **LimitWindow
		into **LimitResult
	}{
		{report.FiveHour, &next.FiveHour, &res.FiveHour},
		{report.SevenDay, &next.SevenDay, &res.SevenDay},
	} {
		if c.in == nil {
			continue
		}
		r := mergeLimitWindow(c.cur, *c.in, now)
		*c.into = &r
		touched = touched || r.Outcome != LimitDropped
	}
	if !touched {
		return res, nil
	}
	data, err := json.Marshal(next)
	if err != nil {
		return res, err
	}
	if err := writeFileAtomic(s.path, data); err != nil {
		return res, err
	}
	s.state = next
	return res, nil
}

// mergeLimitWindow applies the order-safe rule to one window.
func mergeLimitWindow(cur **LimitWindow, in LimitWindow, now int64) LimitResult {
	stored := *cur
	in.ReceivedAt = now
	replace := func(outcome LimitOutcome, prev int64) LimitResult {
		w := in
		*cur = &w
		return LimitResult{Outcome: outcome, PrevReceivedAt: prev}
	}
	if stored == nil {
		return replace(LimitNew, 0)
	}
	prev := stored.ReceivedAt
	switch {
	case in.ResetsAt == stored.ResetsAt:
		if in.UsedPercentage < stored.UsedPercentage {
			return LimitResult{Outcome: LimitDropped, PrevReceivedAt: prev}
		}
		outcome := LimitConfirmed
		if math.Round(in.UsedPercentage) != math.Round(stored.UsedPercentage) {
			outcome = LimitChanged
		}
		return replace(outcome, prev)
	case in.ResetsAt > stored.ResetsAt || stored.ResetsAt <= now:
		return replace(LimitChanged, prev)
	}
	return LimitResult{Outcome: LimitDropped, PrevReceivedAt: prev}
}
