package chatdrafts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// startRecordTTL is how long a Start record keeps an old draft URL redirecting.
const startRecordTTL = 7 * 24 * time.Hour

const startsDir = "starts"

type startRecord struct {
	Session   string    `json:"session"`
	StartedAt time.Time `json:"startedAt"`
}

func (s *Store) startPath(id string) string {
	return filepath.Join(s.root, startsDir, id+".json")
}

// RecordStart remembers that draft id became chat session, so a printed link
// to the draft keeps working.
func (s *Store) RecordStart(id, session string) error {
	if !idPattern.MatchString(id) {
		return nil
	}
	b, err := json.Marshal(startRecord{Session: session, StartedAt: s.now().UTC()})
	if err != nil {
		return err
	}
	if err := writeAtomic(s.startPath(id), b); err != nil {
		return err
	}
	s.pruneStarts()
	return nil
}

// Started is the session id draft id became, "" when no live record exists.
func (s *Store) Started(id string) string {
	if !idPattern.MatchString(id) {
		return ""
	}
	s.pruneStarts()
	b, err := os.ReadFile(s.startPath(id))
	if err != nil {
		return ""
	}
	var rec startRecord
	if json.Unmarshal(b, &rec) != nil {
		return ""
	}
	return rec.Session
}

// pruneStarts deletes records older than startRecordTTL. Failures are ignored:
// a stale record only keeps a redirect a little longer.
func (s *Store) pruneStarts() {
	dir := filepath.Join(s.root, startsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := s.now().Add(-startRecordTTL)
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var rec startRecord
		if json.Unmarshal(b, &rec) != nil || rec.StartedAt.Before(cutoff) {
			_ = os.Remove(p)
		}
	}
}
