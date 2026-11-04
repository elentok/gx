package budget

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/server/limits"
)

const (
	ledgerFileName      = "budget-ledger.json"
	ledgerSeenRetention = 48 * time.Hour
	dayLayout           = limits.DayLayout
)

// seenCost is an iteration's cumulative cost at its last poll. It is persisted so
// a restart adds only what accrued since, not the iteration's whole cost again.
type seenCost struct {
	Cost float64   `json:"cost"`
	At   time.Time `json:"at"`
}

// ledger is the persisted per-day total of iteration cost deltas. A budget
// day runs from local midnight to local midnight.
type ledger struct {
	mu   sync.Mutex
	path string
	Days map[string]float64 `json:"days"`
	// Projects splits each day's total by project: day -> project -> cost.
	Projects map[string]map[string]float64 `json:"projects,omitempty"`
	// Roots splits each day's total by root (an epic or one-off, "project:epic").
	Roots map[string]map[string]float64 `json:"roots,omitempty"`
	Seen  map[string]seenCost           `json:"seen"`
	// Latch holds the limit state of one budget day. It is persisted so a restart
	// does not forget a reached limit, and it is dropped when the day changes.
	Latch latch `json:"latch"`
	// soft and hard are the live daily limits: config.Budget's at startup,
	// raised by an increase. Not persisted; a restart reads config again.
	soft, hard float64
	// hardHeld keeps a reload-lowered hard limit from latching while spend
	// still reaches it.
	hardHeld bool
}

// latch is sticky: once a limit is reached it stays reached until an
// override or midnight, even if the limit is later raised.
type latch struct {
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

// daysCopy returns a copy of the per-day totals, safe to read without the lock.
func (l *ledger) daysCopy() map[string]float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.Days)
}

func openLedger(stateDir string, soft, hard float64) (*ledger, error) {
	l := &ledger{path: filepath.Join(stateDir, ledgerFileName), soft: soft, hard: hard}
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
	if l.Roots == nil {
		l.Roots = map[string]map[string]float64{}
	}
	if l.Seen == nil {
		l.Seen = map[string]seenCost{}
	}
	return l, nil
}

// today is the total for the budget day containing now.
func (l *ledger) today(now time.Time) float64 { return l.day(now.Format(dayLayout)) }

// day is the total for one budget day, keyed as dayLayout.
func (l *ledger) day(key string) float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Days[key]
}

// record folds one iteration's cumulative cost at now into the ledger. The delta
// since its last poll is spread over the budget days that poll interval touched,
// in proportion to the time spent in each, so an iteration crossing midnight
// splits across both days. Cumulative cost only grows: a lower reading (a
// restarted session) adds nothing.
func (l *ledger) record(project, key string, cost float64, now time.Time) {
	l.recordRoot(project, "", key, cost, now)
}

// recordRoot is record that also counts the spend toward its root.
func (l *ledger) recordRoot(project, root, key string, cost float64, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev, seen := l.Seen[key]
	l.Seen[key] = seenCost{Cost: cost, At: now}
	delta := cost - prev.Cost
	if delta <= 0 {
		return
	}
	if !seen || !prev.At.Before(now) {
		l.add(now.Format(dayLayout), project, root, delta)
		return
	}
	total := now.Sub(prev.At)
	for from := prev.At; from.Before(now); {
		to := nextMidnight(from)
		if to.After(now) {
			to = now
		}
		l.add(from.Format(dayLayout), project, root, delta*float64(to.Sub(from))/float64(total))
		from = to
	}
}

// add counts spend toward the day's one machine-wide total and, when the
// project and root are known, toward their shares of it. Callers hold l.mu.
func (l *ledger) add(day, project, root string, amount float64) {
	l.Days[day] += amount
	addShare(l.Projects, day, project, amount)
	addShare(l.Roots, day, root, amount)
}

func addShare(shares map[string]map[string]float64, day, name string, amount float64) {
	if name == "" {
		return
	}
	if shares[day] == nil {
		shares[day] = map[string]float64{}
	}
	shares[day][name] += amount
}

// byRoot is the budget day's total split per root.
func (l *ledger) byRoot(key string) map[string]float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.Roots[key])
}

// byProject is the budget day's total split per project. Spend recorded before
// projects were tracked is in the total only.
func (l *ledger) byProject(key string) map[string]float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.Projects[key])
}

// limits returns the live soft and hard limits.
func (l *ledger) limits() (soft, hard float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.soft, l.hard
}

// latches rolls the latch to now's day, sets any limit that spend since the
// override has reached, and returns the result. Limits of zero are off.
func (l *ledger) latches(now time.Time) latch {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := l.rollLocked(now)
	spent := l.Days[day] - l.Latch.Override
	if l.hard > 0 && spent >= l.hard && !l.hardHeld {
		l.Latch.Hard = true
	}
	if l.soft > 0 && spent >= l.soft || l.Latch.Hard {
		l.Latch.Soft = true
	}
	latch := l.Latch
	// A held hard limit pauses starts without the sticky soft latch, so
	// lifting the hold resumes them.
	latch.Soft = latch.Soft || l.hardHeld
	return latch
}

// held reports whether a reload-lowered hard limit is being held.
func (l *ledger) held() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.hardHeld
}

// releaseHold lets the hard limit latch again at the next check.
func (l *ledger) releaseHold() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hardHeld = false
}

// setLimits sets the live limits, then drops each latch that spend since the
// override no longer reaches. The override point stays where it is.
func (l *ledger) setLimits(now time.Time, soft, hard float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.setLimitsLocked(now, soft, hard)
}

// setLimitsHoldingHard is setLimits for a reload: a hard limit that spend
// already reaches is held instead of latched, so lowering it in config never
// stops agents at once. It reports whether the hold is on.
func (l *ledger) setLimitsHoldingHard(now time.Time, soft, hard float64) (held bool, spent float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	spent = l.setLimitsLocked(now, soft, hard)
	l.hardHeld = l.hardHeld || hard > 0 && spent >= hard && !l.Latch.Hard
	return l.hardHeld, spent
}

// setLimitsLocked returns today's spend since the override. Callers hold l.mu.
func (l *ledger) setLimitsLocked(now time.Time, soft, hard float64) float64 {
	l.soft, l.hard = soft, hard
	day := l.rollLocked(now)
	spent := l.Days[day] - l.Latch.Override
	l.Latch.Hard = l.Latch.Hard && hard > 0 && spent >= hard
	l.Latch.Soft = l.Latch.Hard || l.Latch.Soft && soft > 0 && spent >= soft
	l.hardHeld = l.hardHeld && hard > 0 && spent >= hard
	return spent
}

// rollLocked starts a fresh latch when now is a new budget day, and returns
// the day. Callers hold l.mu.
func (l *ledger) rollLocked(now time.Time) string {
	day := now.Format(dayLayout)
	if l.Latch.Day != day {
		l.Latch = latch{Day: day}
		l.hardHeld = false
	}
	return day
}

// override clears the latches and moves the override point to today's total.
func (l *ledger) override(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := now.Format(dayLayout)
	l.Latch = latch{Day: day, Override: l.Days[day], Notified: l.Latch.Notified}
	l.hardHeld = false
}

func nextMidnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
}

// save drops iterations not polled for a while, then writes through a rename so
// a crash never leaves a torn file.
func (l *ledger) save(now time.Time) error {
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
