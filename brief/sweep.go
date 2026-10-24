package brief

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Retention is how long logs and index lines are kept.
const Retention = 14 * 24 * time.Hour

const indexName = "index.jsonl"

// lockIndex takes an exclusive lock that appenders and the sweep share, so a
// rewrite never drops a line appended while it runs.
func lockIndex(logDir string) (unlock func(), err error) {
	f, err := os.OpenFile(filepath.Join(logDir, indexName+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}

// sweep removes logs and index lines older than Retention. Errors are
// ignored: a failed sweep must never fail a run.
func sweep(logDir string, now time.Time) {
	cutoff := now.Add(-Retention)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(logDir, e.Name()))
		}
	}
	sweepIndex(logDir, cutoff)
}

func sweepIndex(logDir string, cutoff time.Time) {
	unlock, err := lockIndex(logDir)
	if err != nil {
		return
	}
	defer unlock()

	path := filepath.Join(logDir, indexName)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var kept bytes.Buffer
	dropped := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var e indexEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			if t, err := time.Parse(time.RFC3339, e.Time); err == nil && t.Before(cutoff) {
				dropped = true
				continue
			}
		}
		kept.Write(sc.Bytes())
		kept.WriteByte('\n')
	}
	if !dropped {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, kept.Bytes(), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}
