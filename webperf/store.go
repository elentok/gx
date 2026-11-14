// Package webperf keeps web performance samples on disk: one JSON-lines file
// per server-local day, with dedupe, caps and 90-day retention. It has no HTTP
// and no stats code; it trusts the samples it gets (the endpoint validates).
package webperf

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DirName is the subdirectory of the state dir that holds the day files.
	DirName = "web-performance"
	// DayLayout is the local-day format used in file names and Day.Day.
	DayLayout = "2006-01-02"

	fileExt = ".jsonl"

	MaxPerSessionPerDay = 500
	MaxPerDay           = 20000
	RetentionDays       = 90

	sweepEvery = 24 * time.Hour
)

// Sample is one stored line. Optional numbers are nil when absent.
type Sample struct {
	ID             string  `json:"id"`
	Session        string  `json:"session"`
	Metric         string  `json:"metric"`
	Value          float64 `json:"value"`
	Route          string  `json:"route,omitempty"`
	BuildID        string  `json:"build_id,omitempty"`
	WebVersion     string  `json:"web_version,omitempty"`
	Env            string  `json:"env,omitempty"`
	NavigationType string  `json:"navigation_type,omitempty"`
	// Tab is "hard" for a hard load and "route" for a route change.
	Tab string `json:"tab,omitempty"`
	// Timestamp is the browser's time, informational only: the day comes from
	// the receive time.
	Timestamp int64 `json:"ts,omitempty"`

	TTFB       *float64 `json:"ttfb,omitempty"`
	APILatency *float64 `json:"api_latency,omitempty"`
	JSBytes    *float64 `json:"js_bytes,omitempty"`
	CSSBytes   *float64 `json:"css_bytes,omitempty"`

	InputDelay        *float64 `json:"input_delay,omitempty"`
	Processing        *float64 `json:"processing,omitempty"`
	PresentationDelay *float64 `json:"presentation_delay,omitempty"`
	Action            string   `json:"action,omitempty"`
	WorstChunk        string   `json:"worst_chunk,omitempty"`
	InteractionType   string   `json:"interaction_type,omitempty"`
	WorstFrame        *float64 `json:"worst_frame,omitempty"`

	LCPLoadDelay    *float64 `json:"lcp_load_delay,omitempty"`
	LCPLoadDuration *float64 `json:"lcp_load_duration,omitempty"`
	LCPRenderDelay  *float64 `json:"lcp_render_delay,omitempty"`

	// Outcome is ready, error or empty; Trigger is how a route span began;
	// DataSource is cache or network. Set on route and app readiness samples.
	Outcome    string `json:"outcome,omitempty"`
	Trigger    string `json:"trigger,omitempty"`
	DataSource string `json:"data_source,omitempty"`
}

// Store is a handle on the day files. Safe for concurrent use.
type Store struct {
	dir string
	now func() time.Time

	mu     sync.Mutex
	states map[string]*dayState // dedupe and cap state, today and yesterday
	cache  map[string]*cached

	stop chan struct{}
	done chan struct{}
}

type dayState struct {
	seen       map[string]struct{}
	perSession map[string]int
	total      int
}

type sig struct {
	size  int64
	mtime time.Time
	ok    bool
}

type cached struct {
	day   *Day
	sig   sig
	final bool // loaded after the day ended: never re-read
}

// Open creates the store under stateDir, loads today's and yesterday's keys,
// sweeps old files and starts the daily sweep. Close stops it.
func Open(stateDir string, now func() time.Time) (*Store, error) {
	dir := filepath.Join(stateDir, DirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		dir:    dir,
		now:    now,
		states: map[string]*dayState{},
		cache:  map[string]*cached{},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	t := now()
	if err := s.Sweep(t); err != nil {
		return nil, err
	}
	s.mu.Lock()
	for _, d := range []string{dayOf(t), dayOf(t.AddDate(0, 0, -1))} {
		if _, err := s.stateLocked(d); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	s.mu.Unlock()
	go s.sweepLoop()
	return s, nil
}

// Close stops the daily sweep.
func (s *Store) Close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}

func (s *Store) sweepLoop() {
	defer close(s.done)
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			_ = s.Sweep(s.now())
		}
	}
}

func dayOf(t time.Time) string { return t.Format(DayLayout) }

func (s *Store) path(day string) string { return filepath.Join(s.dir, day+fileExt) }

func (s *Store) stat(day string) sig {
	fi, err := os.Stat(s.path(day))
	if err != nil {
		return sig{}
	}
	return sig{size: fi.Size(), mtime: fi.ModTime(), ok: true}
}

// readSamples parses a day file, skipping lines that do not parse (a torn last
// line from a crash).
func readSamples(path string) ([]Sample, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Sample
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sm Sample
		if json.Unmarshal(line, &sm) != nil {
			continue
		}
		out = append(out, sm)
	}
	return out, nil
}

func dedupeKey(sm *Sample) string { return sm.Session + "\x00" + sm.ID }

// stateLocked returns the day's dedupe and cap state, loading it from the file
// on first use.
func (s *Store) stateLocked(day string) (*dayState, error) {
	if st, ok := s.states[day]; ok {
		return st, nil
	}
	samples, err := readSamples(s.path(day))
	if err != nil {
		return nil, err
	}
	st := &dayState{seen: make(map[string]struct{}, len(samples)), perSession: map[string]int{}}
	for i := range samples {
		st.seen[dedupeKey(&samples[i])] = struct{}{}
		st.perSession[samples[i].Session]++
		st.total++
	}
	s.states[day] = st
	return st, nil
}

// Append stores a batch received at the given time in that local day's file.
// It returns how many samples were stored and how many were dropped (duplicate
// or over a cap).
func (s *Store) Append(received time.Time, samples []Sample) (stored, dropped int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := dayOf(received)
	st, err := s.stateLocked(day)
	if err != nil {
		return 0, 0, err
	}
	var buf bytes.Buffer
	var keys []string
	var sessions []string
	for i := range samples {
		sm := &samples[i]
		k := dedupeKey(sm)
		if _, dup := st.seen[k]; dup {
			dropped++
			continue
		}
		if st.total+stored >= MaxPerDay || st.perSession[sm.Session]+countOf(sessions, sm.Session) >= MaxPerSessionPerDay {
			dropped++
			continue
		}
		line, err := json.Marshal(sm)
		if err != nil {
			return 0, 0, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
		// Mark seen now so a repeat inside the batch drops too.
		st.seen[k] = struct{}{}
		keys = append(keys, k)
		sessions = append(sessions, sm.Session)
		stored++
	}
	if stored == 0 {
		return 0, dropped, nil
	}
	if err := s.writeLocked(day, buf.Bytes()); err != nil {
		for _, k := range keys {
			delete(st.seen, k)
		}
		return 0, 0, err
	}
	for _, sess := range sessions {
		st.perSession[sess]++
	}
	st.total += stored
	return stored, dropped, nil
}

func countOf(list []string, v string) int {
	n := 0
	for _, x := range list {
		if x == v {
			n++
		}
	}
	return n
}

// writeLocked appends data in one write, first starting a new line when the
// file ends in a torn one.
func (s *Store) writeLocked(day string, data []byte) error {
	f, err := os.OpenFile(s.path(day), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil {
		return err
	} else if fi.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, fi.Size()-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			data = append([]byte{'\n'}, data...)
		}
	}
	_, err = f.Write(data)
	return err
}

// Sweep deletes day files older than the retention window, measured from now.
func (s *Store) Sweep(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := dayOf(now.AddDate(0, 0, -RetentionDays))
	days, err := s.daysLocked()
	if err != nil {
		return err
	}
	for _, d := range days {
		if d < cutoff {
			if err := os.Remove(s.path(d)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			delete(s.cache, d)
			delete(s.states, d)
		}
	}
	keep := map[string]bool{dayOf(now): true, dayOf(now.AddDate(0, 0, -1)): true}
	for d := range s.states {
		if !keep[d] {
			delete(s.states, d)
		}
	}
	return nil
}

// daysLocked lists stored days, oldest first.
func (s *Store) daysLocked() ([]string, error) {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var days []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, fileExt) {
			continue
		}
		d := strings.TrimSuffix(name, fileExt)
		if _, err := time.Parse(DayLayout, d); err != nil {
			continue
		}
		days = append(days, d)
	}
	sort.Strings(days)
	return days, nil
}

// Days lists the stored days, oldest first.
func (s *Store) Days() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.daysLocked()
}

// OldestDay is the oldest stored day, or false when nothing is stored.
func (s *Store) OldestDay() (string, bool) {
	days, err := s.Days()
	if err != nil || len(days) == 0 {
		return "", false
	}
	return days[0], true
}

// Day returns the columns for one local day (YYYY-MM-DD). A day with no file
// has zero rows. Past days are parsed once; today's file is re-read when its
// size or mtime changed.
func (s *Store) Day(day string) (*Day, error) {
	s.mu.Lock()
	past := day < dayOf(s.now())
	cur := s.stat(day)
	if c, ok := s.cache[day]; ok {
		if c.final {
			s.mu.Unlock()
			return c.day, nil
		}
		if c.sig == cur {
			c.final = past
			s.mu.Unlock()
			return c.day, nil
		}
	}
	s.mu.Unlock()
	// Parse outside the lock so several days load at once. Two callers may
	// parse the same day; the last write wins and both results are equal.
	samples, err := readSamples(s.path(day))
	if err != nil {
		return nil, err
	}
	d := buildDay(day, samples)
	s.mu.Lock()
	s.cache[day] = &cached{day: d, sig: cur, final: past}
	s.mu.Unlock()
	return d, nil
}

// Percentile is the nearest-rank percentile (p in 0..100) of the values,
// ignoring NaN. It reports false when there are no values.
func Percentile(vals []float64, p float64) (float64, bool) {
	sorted := make([]float64, 0, len(vals))
	for _, v := range vals {
		if !math.IsNaN(v) {
			sorted = append(sorted, v)
		}
	}
	if len(sorted) == 0 {
		return 0, false
	}
	sort.Float64s(sorted)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	rank = min(max(rank, 1), len(sorted))
	return sorted[rank-1], true
}
