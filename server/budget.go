package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const (
	defaultBudgetKillGrace = 15 * time.Second
	ledgerFileName         = "budget-ledger.json"
	defaultBudgetPoll      = 30 * time.Second
	ledgerSeenRetention    = 48 * time.Hour
	budgetDayLayout        = "2006-01-02"
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
func (l *budgetLedger) today(now time.Time) float64 { return l.day(now.Format(budgetDayLayout)) }

// day is the total for one budget day, keyed as budgetDayLayout.
func (l *budgetLedger) day(key string) float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Days[key]
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

// BudgetStatus is today's spend against the configured limits, in dollars. A
// zero limit means that limit is off. Shared by /v1/budget and the snapshot.
type BudgetStatus struct {
	Day       string  `json:"day"`
	Total     float64 `json:"total"`
	SoftLimit float64 `json:"soft_limit"`
	HardLimit float64 `json:"hard_limit"`
}

func (s *Server) budgetStatus(now time.Time) BudgetStatus {
	return BudgetStatus{
		Day:       now.Format(budgetDayLayout),
		Total:     s.ledger.today(now),
		SoftLimit: s.cfg.BudgetSoftLimit,
		HardLimit: s.cfg.BudgetHardLimit,
	}
}

func (s *Server) budget(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.budgetStatus(time.Now()))
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
	s.notifyBudget(now)
	if s.budgetHardReached(now) {
		s.killForBudget(now)
	}
}

// budgetSoftReached holds new starts back; the hard limit implies it.
func (s *Server) budgetSoftReached(now time.Time) bool {
	total := s.ledger.today(now)
	return s.cfg.BudgetSoftLimit > 0 && total >= s.cfg.BudgetSoftLimit || s.budgetHardReached(now)
}

func (s *Server) budgetHardReached(now time.Time) bool {
	return s.cfg.BudgetHardLimit > 0 && s.ledger.today(now) >= s.cfg.BudgetHardLimit
}

// killForBudget stops every live pane: ctrl+c, a grace period, then it closes
// and parks the panes whose cost still rose. A pane that went quiet is left to
// its own finish.
func (s *Server) killForBudget(now time.Time) {
	grace := s.cfg.BudgetKillGrace
	if grace <= 0 {
		grace = defaultBudgetKillGrace
	}
	runs := s.registry.list()
	before := map[string]float64{}
	for _, t := range runs {
		before[t.Address] = s.liveCost(t.Address)
		if err := herdr.AgentSendKeys(t.Pane, "ctrl+c"); err != nil {
			s.log.Warn("budget stop signal", "ticket", t.Address, "err", err)
		}
	}
	time.Sleep(grace)
	for _, t := range runs {
		if s.liveCost(t.Address) <= before[t.Address] {
			continue
		}
		if err := s.parkBudgetKilled(t); err != nil {
			s.log.Error("budget kill", "ticket", t.Address, "err", err)
		}
	}
}

func (s *Server) liveCost(address string) float64 {
	it, ok := s.liveIteration(address)
	if !ok {
		return 0
	}
	cost, _ := s.costOf(it)
	return cost
}

// parkBudgetKilled closes the pane, then parks the ticket needs-repair.
func (s *Server) parkBudgetKilled(t trackedRun) error {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return err
	}
	closeErr := herdr.TabClose(t.Tab)
	parkErr := ralphloop.ParkBudgetKilled(dir, addr.Epic, addr.ID, t.TicketPath, "daily budget hard limit reached")
	if parkErr == nil {
		s.events.publish(EventTicketParked, t.Address)
	}
	return errors.Join(closeErr, parkErr)
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
