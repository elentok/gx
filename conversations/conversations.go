// Package conversations keeps gx's record of every conversation a linked
// interactive session reported. It lives outside the session's state dir, so
// it outlives removing the session, and it needs no server to read.
package conversations

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/config"
)

const startedFile = "started"

// Record is one conversation, counted toward the latest Session ticket its
// session had while it ran.
type Record struct {
	ID      string `json:"id"`
	Dir     string `json:"dir"`
	Harness string `json:"harness"`
	Ticket  string `json:"ticket"`
	// RecordedAt is when the conversation was first recorded.
	RecordedAt time.Time `json:"recorded_at"`
}

// DefaultDir is where the server keeps the records: <data>/conversations.
func DefaultDir() (string, error) {
	data, err := config.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "conversations"), nil
}

// MarkStarted notes when recording began, once: later calls keep the first
// time.
func MarkStarted(root string, now time.Time) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	path := filepath.Join(root, startedFile)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(now.UTC().Format(time.RFC3339Nano))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Started is when recording began; false when it never did.
func Started(root string) (time.Time, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, startedFile))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse %s: %w", startedFile, err)
	}
	return t, true, nil
}

// Save writes r, keeping the RecordedAt of an earlier save of the same
// conversation, so a re-link moves it to the new ticket.
func Save(root string, r Record) error {
	if r.ID == "" || !filepath.IsLocal(r.ID) || strings.ContainsRune(r.ID, filepath.Separator) {
		return fmt.Errorf("bad conversation id %q", r.ID)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	path := filepath.Join(root, r.ID+".json")
	if prev, err := read(path); err == nil {
		r.RecordedAt = prev.RecordedAt
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// List returns every record, oldest first. A missing root has none.
func List(root string) ([]Record, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, err := read(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b Record) int { return a.RecordedAt.Compare(b.RecordedAt) })
	return out, nil
}

func read(path string) (Record, error) {
	var r Record
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("parse %s: %w", path, err)
	}
	return r, nil
}
