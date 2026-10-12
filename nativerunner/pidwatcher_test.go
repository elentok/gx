package nativerunner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPidWatcher_IgnoresFileFromBeforeSpawn(t *testing.T) {
	dir := t.TempDir()
	spawned := time.Now()
	w := newPidWatcher(dir, 4242, spawned)
	write := func(startedAt time.Time) {
		data := fmt.Sprintf(`{"pid":4242,"startedAt":%d,"status":"idle"}`, startedAt.UnixMilli())
		if err := os.WriteFile(filepath.Join(dir, "4242.json"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(spawned.Add(-time.Hour))
	w.poll()
	if s, _ := w.state(); s != nil {
		t.Fatalf("stale file from a reused pid read as %+v", *s)
	}

	// Claude's startedAt is a little after the spawn; slack covers rounding.
	write(spawned.Add(-time.Second))
	w.poll()
	if s, turn := w.state(); s == nil || s.Status != ClaudeIdle || turn != 1 {
		t.Fatalf("fresh file: got %v at turn %d", s, turn)
	}
}

func TestPidWatcher_IgnoresFileWithoutStartedAt(t *testing.T) {
	dir := t.TempDir()
	w := newPidWatcher(dir, 4242, time.Now())
	if err := os.WriteFile(filepath.Join(dir, "4242.json"), []byte(`{"pid":4242,"status":"idle"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	w.poll()
	if s, _ := w.state(); s != nil {
		t.Fatalf("file without startedAt read as %+v", *s)
	}
}
