package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
)

const (
	ledgerFileName      = "budget-ledger.json"
	defaultBudgetPoll   = 30 * time.Second
	ledgerSeenRetention = 48 * time.Hour
	budgetDayLayout     = "2006-01-02"
)

// seenCost is an iteration's cumulative cost at its last poll. It is persisted so
// a restart adds only what accrued since, not the iteration's whole cost again.
type seenCost struct {
	Cost float64   `json:"cost"`
	At   time.Time `json:"at"`
}

// budgetLedger is the persisted per-day total of iteration cost deltas. A budget
// day runs from local midnight to local midnight.
type budgetLedger struct {
	mu   sync.Mutex
	path string
	Days map[string]float64  `json:"days"`
	Seen map[string]seenCost `json:"seen"`
}

func openLedger(stateDir string) (*budgetLedger, error) {
	l := &budgetLedger{path: filepath.Join(stateDir, ledgerFileName)}
	data, err := os.ReadFile(l.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, l); err != nil {
			return nil, err
		}
	}
	if l.Days == nil {
		l.Days = map[string]float64{}
	}
	if l.Seen == nil {
		l.Seen = map[string]seenCost{}
	}
	return l, nil
}

// today is the total for the budget day containing now.
func (l *budgetLedger) today(now time.Time) float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Days[now.Format(budgetDayLayout)]
}

// record folds one iteration's cumulative cost at now into the ledger. The delta
// since its last poll is spread over the budget days that poll interval touched,
// in proportion to the time spent in each, so an iteration crossing midnight
// splits across both days. Cumulative cost only grows: a lower reading (a
// restarted session) adds nothing.
func (l *budgetLedger) record(key string, cost float64, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev, seen := l.Seen[key]
	l.Seen[key] = seenCost{Cost: cost, At: now}
	delta := cost - prev.Cost
	if delta <= 0 {
		return
	}
	if !seen || !prev.At.Before(now) {
		l.Days[now.Format(budgetDayLayout)] += delta
		return
	}
	total := now.Sub(prev.At)
	for from := prev.At; from.Before(now); {
		to := nextMidnight(from)
		if to.After(now) {
			to = now
		}
		l.Days[from.Format(budgetDayLayout)] += delta * float64(to.Sub(from)) / float64(total)
		from = to
	}
}

func nextMidnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
}

// save drops iterations not polled for a while, then writes through a rename so
// a crash never leaves a torn file.
func (l *budgetLedger) save(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, s := range l.Seen {
		if now.Sub(s.At) > ledgerSeenRetention {
			delete(l.Seen, k)
		}
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// iterationCost is the cumulative cost of a live iteration; ok is false when it
// has none (no session yet, or an agent without a transcript cost).
func iterationCost(it IterationInfo) (float64, bool) {
	if it.Session == "" {
		return 0, false
	}
	cost, ok, err := ralphloop.SessionCost(it.Worktree, it.Session)
	return cost, ok && err == nil
}

// pollBudget adds every live iteration's cost delta to the ledger. All ticket
// kinds count.
func (s *Server) pollBudget(now time.Time) {
	for _, t := range s.idx.snapshot().Tickets {
		if t.Status != "claimed" {
			continue
		}
		it, ok := s.liveIteration(t.Address)
		if !ok {
			continue
		}
		if cost, ok := s.costOf(it); ok {
			s.ledger.record(it.Address+"@"+it.Session, cost, now)
		}
	}
	if err := s.ledger.save(now); err != nil {
		s.log.Error("save budget ledger", "err", err)
	}
}

func (s *Server) keepBudgetPolled(ctx context.Context) {
	interval := s.cfg.BudgetPollInterval
	if interval <= 0 {
		interval = defaultBudgetPoll
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.pollBudget(time.Now())
		}
	}
}
