package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// RecordsDirName is the subdirectory of the state dir that holds the day files.
	RecordsDirName = "rest-records"
	// RecordsRetentionDays is how long records are kept.
	RecordsRetentionDays = 90

	dayLayout = "2006-01-02"
	fileExt   = ".jsonl"
)

// Records is the append-only store of Break starts, dropped Breaks and
// unlocks: one JSON-lines file per local day, as webperf keeps its samples.
type Records struct {
	dir string
	now func() time.Time
	mu  sync.Mutex
}

// OpenRecords creates the store under stateDir.
func OpenRecords(stateDir string, now func() time.Time) (*Records, error) {
	dir := filepath.Join(stateDir, RecordsDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Records{dir: dir, now: now}, nil
}

func (s *Records) path(day string) string { return filepath.Join(s.dir, day+fileExt) }

// Append adds the records to the day file of each record's time, then prunes
// the files older than the retention.
func (s *Records) Append(recs ...Record) error {
	if len(recs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range recs {
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if err := appendLine(s.path(r.Time.Format(dayLayout)), append(line, '\n')); err != nil {
			return err
		}
	}
	return s.pruneLocked(s.now())
}

func appendLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (s *Records) pruneLocked(now time.Time) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	cutoff := now.AddDate(0, 0, -RecordsRetentionDays).Format(dayLayout)
	for _, e := range entries {
		day, ok := strings.CutSuffix(e.Name(), fileExt)
		if ok && day < cutoff {
			if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// Range returns the records of every local day from the day of from to the
// day of to, both included, oldest first. A line that does not parse (a torn
// last line from a crash) is skipped.
func (s *Records) Range(from, to time.Time) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Record
	last := to.Format(dayLayout)
	for d := from; d.Format(dayLayout) <= last; d = d.AddDate(0, 0, 1) {
		b, err := os.ReadFile(s.path(d.Format(dayLayout)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(b, []byte("\n")) {
			var r Record
			if len(bytes.TrimSpace(line)) == 0 || json.Unmarshal(line, &r) != nil {
				continue
			}
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}
