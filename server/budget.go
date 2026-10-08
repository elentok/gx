package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/elentok/gx/events"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/config"
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
	// Projects splits each day's total by project: day -> project -> cost.
	Projects map[string]map[string]float64 `json:"projects,omitempty"`
	Seen map[string]seenCost `json:"seen"`
	// Latch holds the limit state of one budget day. It is persisted so a restart
	// does not forget a reached limit, and it is dropped when the day changes.
	Latch budgetLatch `json:"latch"`
}

// budgetLatch is sticky: once a limit is reached it stays reached until an
// override or midnight, even if the limit is later raised.
type budgetLatch struct {
	Day  string `json:"day"`
	Soft bool   `json:"soft,omitempty"`
	Hard bool   `json:"hard,omitempty"`
	// Override is the day's total at the last override. Limits count spend
	// beyond it, so an override buys a fresh allowance rather than silencing
	// the limit for the rest of the day.
	Override float64 `json:"override,omitempty"`
	// Notified is the highest level (1 soft, 2 hard) already announced.
	Notified int `json:"notified,omitempty"`
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
	if l.Projects == nil {
		l.Projects = map[string]map[string]float64{}
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
func (l *budgetLedger) record(project, key string, cost float64, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev, seen := l.Seen[key]
	l.Seen[key] = seenCost{Cost: cost, At: now}
	delta := cost - prev.Cost
	if delta <= 0 {
		return
	}
	if !seen || !prev.At.Before(now) {
		l.add(now.Format(budgetDayLayout), project, delta)
		return
	}
	total := now.Sub(prev.At)
	for from := prev.At; from.Before(now); {
		to := nextMidnight(from)
		if to.After(now) {
			to = now
		}
		l.add(from.Format(budgetDayLayout), project, delta*float64(to.Sub(from))/float64(total))
		from = to
	}
}

// add counts spend toward the day's one machine-wide total and, when the
// project is known, toward its share of it. Callers hold l.mu.
func (l *budgetLedger) add(day, project string, amount float64) {
	l.Days[day] += amount
	if project == "" {
		return
	}
	if l.Projects[day] == nil {
		l.Projects[day] = map[string]float64{}
	}
	l.Projects[day][project] += amount
}

// byProject is the budget day's total split per project. Spend recorded before
// projects were tracked is in the total only.
func (l *budgetLedger) byProject(key string) map[string]float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.Projects[key])
}

// latches rolls the latch to now's day, sets any limit that spend since the
// override has reached, and returns the result. Limits of zero are off.
func (l *budgetLedger) latches(now time.Time, soft, hard float64) budgetLatch {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := now.Format(budgetDayLayout)
	if l.Latch.Day != day {
		l.Latch = budgetLatch{Day: day}
	}
	spent := l.Days[day] - l.Latch.Override
	if hard > 0 && spent >= hard {
		l.Latch.Hard = true
	}
	if soft > 0 && spent >= soft || l.Latch.Hard {
		l.Latch.Soft = true
	}
	return l.Latch
}

// override clears the latches and moves the override point to today's total.
func (l *budgetLedger) override(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := now.Format(budgetDayLayout)
	l.Latch = budgetLatch{Day: day, Override: l.Days[day], Notified: l.Latch.Notified}
}

// BudgetStatus is today's spend against the configured limits, in dollars. A
// zero limit means that limit is off. Shared by /v1/budget and the snapshot.
// BudgetPaused is the budget's hold on new starts; it is independent of the
// operator's queue pause.
type BudgetStatus struct {
	Day          string  `json:"day"`
	Total        float64 `json:"total"`
	SoftLimit    float64 `json:"soft_limit"`
	HardLimit    float64 `json:"hard_limit"`
	BudgetPaused bool    `json:"budget_paused"`
	HardLatched  bool    `json:"hard_latched"`
	// Projects is each project's share of Total.
	Projects map[string]float64 `json:"projects,omitempty"`
}

func (s *Server) budgetStatus(now time.Time) BudgetStatus {
	latch := s.budgetLatches(now)
	day := now.Format(budgetDayLayout)
	return BudgetStatus{
		Day:          day,
		Total:        s.ledger.today(now),
		Projects:     s.ledger.byProject(day),
		SoftLimit:    s.cfg.BudgetSoftLimit,
		HardLimit:    s.cfg.BudgetHardLimit,
		BudgetPaused: latch.Soft,
		HardLatched:  latch.Hard,
	}
}

func (s *Server) budgetLatches(now time.Time) budgetLatch {
	return s.ledger.latches(now, s.cfg.BudgetSoftLimit, s.cfg.BudgetHardLimit)
}

// BudgetResult is the outcome of a budget write: the status after it, or a
// refusal.
type BudgetResult struct {
	Budget  *BudgetStatus `json:"budget,omitempty"`
	Refused bool          `json:"refused,omitempty"`
	Reason  string        `json:"reason,omitempty"`
	Message string        `json:"message,omitempty"`
}

// ReasonNothingLatched refuses an override when no limit is holding anything.
const ReasonNothingLatched = "nothing-latched"

// budgetOverride lifts the latches so work may resume. The ledger is saved
// before it returns, so a restart cannot bring the latch back.
func (s *Server) budgetOverride(now time.Time) (BudgetResult, error) {
	if s.cfg.Orchestrator != config.OrchestratorServer {
		return BudgetResult{Refused: true, Reason: ReasonSchedulerNotSelected, Message: `orchestrator is not "server"`}, nil
	}
	if !s.budgetLatches(now).Soft {
		return BudgetResult{Refused: true, Reason: ReasonNothingLatched, Message: "no budget limit is reached"}, nil
	}
	s.ledger.override(now)
	if err := s.ledger.save(now); err != nil {
		return BudgetResult{}, err
	}
	s.events.publish(EventQueueChanged, "")
	s.kickRunner()
	status := s.budgetStatus(now)
	return BudgetResult{Budget: &status}, nil
}

func (s *Server) budgetOverrideWrite(w http.ResponseWriter, _ *http.Request) {
	res, err := s.budgetOverride(time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
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
			var project string
			if addr, err := tickets.ParseAddress(it.Address, tickets.AddressContext{}); err == nil {
				project = addr.Project
			}
			s.ledger.record(project, it.Address+"@"+it.Session, cost, now)
		}
	}
	hard := s.budgetLatches(now).Hard
	if err := s.ledger.save(now); err != nil {
		s.log.Error("save budget ledger", "err", err)
	}
	s.notifyBudget(now)
	if hard {
		s.killForBudget(now)
	}
}

// budgetSoftReached holds new starts back; the hard limit implies it.
func (s *Server) budgetSoftReached(now time.Time) bool { return s.budgetLatches(now).Soft }

func (s *Server) budgetHardReached(now time.Time) bool { return s.budgetLatches(now).Hard }

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
	parkErr := s.parkTicket(dir, addr, t.TicketPath, events.BudgetKilled, "daily budget hard limit reached")
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
