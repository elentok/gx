// Package tokenledger owns token-ledger.json: per-day tokens and dollars by
// project, agent group and harness (ADR 0046). It is its own package so the
// migration step can write it without importing server.
//
// Lock order everywhere: the project migration lock first, then the ledger lock.
package tokenledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elentok/gx/fsutil"
)

const (
	FileName = "token-ledger.json"
	lockName = FileName + ".lock"

	// DayLayout is the local-day format used for every day field.
	DayLayout = "2006-01-02"
)

// Agent groups (glossary).
const (
	GroupQueue  = "Queue"
	GroupDesign = "Design"
	GroupOneOff = "One-off"
	GroupOther  = "Other"
)

// Key identifies one bucket. Project may be empty ("no project").
type Key struct {
	Day     string `json:"day"`
	Project string `json:"project"`
	Group   string `json:"group"`
	Harness string `json:"harness"`
}

// Row is one bucket as stored and as passed to ApplyRebuilt.
type Row struct {
	Key
	Tokens  int64   `json:"tokens"`
	Dollars float64 `json:"dollars"`
	Rebuilt bool    `json:"rebuilt,omitempty"`
}

// Delta is a live increment for one bucket.
type Delta struct {
	Key
	Tokens  int64
	Dollars float64
}

// Seen is a session's last recorded cumulative total.
type Seen struct {
	Tokens  int64   `json:"tokens"`
	Dollars float64 `json:"dollars"`
}

type fileData struct {
	Rows            []Row           `json:"rows"`
	Seen            map[string]Seen `json:"seen"`
	FirstRebuiltDay string          `json:"firstRebuiltDay,omitempty"`
	FirstLiveDay    string          `json:"firstLiveDay,omitempty"`
}

type sig struct {
	size  int64
	mtime time.Time
	ok    bool
}

// Ledger is a handle on the state dir's token ledger. Several handles, in one
// or several processes, may write the same dir.
type Ledger struct {
	dir  string
	path string
	now  func() time.Time

	mu      sync.Mutex
	data    fileData
	buckets map[Key]Row
	loaded  sig
}

// Open loads the ledger in stateDir. A missing file is an empty ledger.
func Open(stateDir string) (*Ledger, error) {
	return OpenWithClock(stateDir, time.Now)
}

// OpenWithClock is Open with a clock, which decides the first live day and the
// rebuilt cutoff. Fixtures use it to seed a ledger whose live range started in
// the past.
func OpenWithClock(stateDir string, now func() time.Time) (*Ledger, error) {
	l := &Ledger{dir: stateDir, path: filepath.Join(stateDir, FileName), now: now}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.reload(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Ledger) today() string { return l.now().Format(DayLayout) }

func (l *Ledger) stat() sig {
	fi, err := os.Stat(l.path)
	if err != nil {
		return sig{}
	}
	return sig{size: fi.Size(), mtime: fi.ModTime(), ok: true}
}

// reload reads the data file. Callers hold l.mu.
func (l *Ledger) reload() error {
	s := l.stat()
	var d fileData
	if s.ok {
		b, err := os.ReadFile(l.path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &d); err != nil {
			return err
		}
	}
	if d.Seen == nil {
		d.Seen = map[string]Seen{}
	}
	l.data = d
	l.buckets = make(map[Key]Row, len(d.Rows))
	for _, r := range d.Rows {
		l.buckets[r.Key] = r
	}
	l.loaded = s
	return nil
}

// refresh reloads when another writer changed the file. One stat per call.
func (l *Ledger) refresh() error {
	if l.stat() == l.loaded {
		return nil
	}
	return l.reload()
}

// mutate re-reads the file under the exclusive lock, applies fn and writes
// when fn reports a change.
func (l *Ledger) mutate(fn func() bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return withLock(l.dir, func() error {
		if err := l.reload(); err != nil {
			return err
		}
		if !fn() {
			return nil
		}
		return l.save()
	})
}

func (l *Ledger) save() error {
	l.data.Rows = l.data.Rows[:0]
	for _, r := range l.buckets {
		l.data.Rows = append(l.data.Rows, r)
	}
	slices.SortFunc(l.data.Rows, func(a, b Row) int {
		return strings.Compare(rowSortKey(a.Key), rowSortKey(b.Key))
	})
	b, err := json.MarshalIndent(l.data, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(l.path, b); err != nil {
		return err
	}
	l.loaded = l.stat()
	return nil
}

func rowSortKey(k Key) string {
	return k.Day + "\x00" + k.Project + "\x00" + k.Group + "\x00" + k.Harness
}

// withLock runs fn holding an exclusive flock on a lock file separate from the
// data file, because the data file is replaced by rename.
func withLock(dir string, fn func() error) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}

// stampLive records the first live day once. It reports whether it changed.
func (l *Ledger) stampLive() bool {
	if l.data.FirstLiveDay != "" {
		return false
	}
	l.data.FirstLiveDay = l.today()
	return true
}

// Add adds a live delta. A zero delta writes nothing.
func (l *Ledger) Add(d Delta) error {
	if d.Tokens == 0 && d.Dollars == 0 {
		return nil
	}
	return l.mutate(func() bool {
		r := l.buckets[d.Key]
		r.Key = d.Key
		r.Tokens += d.Tokens
		r.Dollars += d.Dollars
		l.buckets[d.Key] = r
		l.stampLive()
		return true
	})
}

// Seen returns a session's last recorded total.
func (l *Ledger) Seen(session string) (Seen, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.refresh(); err != nil {
		return Seen{}, false, err
	}
	s, ok := l.data.Seen[session]
	return s, ok, nil
}

// SetSeen records a session's cumulative total. An unchanged total writes
// nothing.
func (l *Ledger) SetSeen(session string, s Seen) error {
	return l.mutate(func() bool {
		if old, ok := l.data.Seen[session]; ok && old == s {
			return false
		}
		l.data.Seen[session] = s
		l.stampLive()
		return true
	})
}

// Record adds a live delta and sets the session's seen total in one locked
// write, so a failure can't store one without the other. A zero delta adds
// nothing; an unchanged seen total with a zero delta writes nothing.
func (l *Ledger) Record(d Delta, session string, s Seen) error {
	return l.mutate(func() bool {
		changed := false
		if d.Tokens != 0 || d.Dollars != 0 {
			r := l.buckets[d.Key]
			r.Key = d.Key
			r.Tokens += d.Tokens
			r.Dollars += d.Dollars
			l.buckets[d.Key] = r
			changed = true
		}
		if old, ok := l.data.Seen[session]; !ok || old != s {
			l.data.Seen[session] = s
			changed = true
		}
		if changed {
			l.stampLive()
		}
		return changed
	})
}

// Day returns the buckets of one local day, sorted by project, group, harness.
func (l *Ledger) Day(day string) ([]Row, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.refresh(); err != nil {
		return nil, err
	}
	var out []Row
	for k, r := range l.buckets {
		if k.Day == day {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Row) int {
		return strings.Compare(rowSortKey(a.Key), rowSortKey(b.Key))
	})
	return out, nil
}

// Range returns the buckets of every day from..to inclusive, sorted by day,
// project, group, harness, plus the first rebuilt and first live days, all from
// one locked read.
func (l *Ledger) Range(from, to string) (rows []Row, firstRebuilt, firstLive string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.refresh(); err != nil {
		return nil, "", "", err
	}
	for k, r := range l.buckets {
		if k.Day >= from && k.Day <= to {
			rows = append(rows, r)
		}
	}
	slices.SortFunc(rows, func(a, b Row) int {
		return strings.Compare(rowSortKey(a.Key), rowSortKey(b.Key))
	})
	return rows, l.data.FirstRebuiltDay, l.data.FirstLiveDay, nil
}

// FirstLiveDay is the day of the first live write, or "".
func (l *Ledger) FirstLiveDay() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.refresh(); err != nil {
		return "", err
	}
	return l.data.FirstLiveDay, nil
}

// FirstRebuiltDay is the earliest rebuilt day, or "".
func (l *Ledger) FirstRebuiltDay() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.refresh(); err != nil {
		return "", err
	}
	return l.data.FirstRebuiltDay, nil
}

// ApplyRebuilt writes rebuilt rows. It skips days on or after the first live
// day (or, with no live row yet, on or after today), never lowers a bucket and
// writes nothing when no bucket changes. It returns the rows written and the
// distinct days skipped.
func (l *Ledger) ApplyRebuilt(rows []Row) (written, skippedDays int, err error) {
	err = l.mutate(func() bool {
		cutoff := l.data.FirstLiveDay
		if cutoff == "" {
			cutoff = l.today()
		}
		skipped := map[string]bool{}
		for _, in := range rows {
			if in.Day >= cutoff {
				skipped[in.Day] = true
				continue
			}
			old, exists := l.buckets[in.Key]
			next := Row{Key: in.Key, Tokens: in.Tokens, Dollars: in.Dollars, Rebuilt: true}
			if exists {
				next.Tokens = max(old.Tokens, next.Tokens)
				next.Dollars = max(old.Dollars, next.Dollars)
				if old == next {
					continue
				}
			}
			l.buckets[in.Key] = next
			written++
			if l.data.FirstRebuiltDay == "" || in.Day < l.data.FirstRebuiltDay {
				l.data.FirstRebuiltDay = in.Day
			}
		}
		skippedDays = len(skipped)
		return written > 0
	})
	return written, skippedDays, err
}
