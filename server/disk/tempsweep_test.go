package disk

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepStaleTemp(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-3 * time.Hour)
	mk := func(name string, mtime time.Time) {
		t.Helper()
		sub := filepath.Join(dir, name, "epic")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(sub, "run-log.jsonl")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{f, sub, filepath.Join(dir, name)} {
			if err := os.Chtimes(p, mtime, mtime); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("gx-epic-1", old)
	mk("go-build2", old)
	mk("gxs3", old)
	mk("gxf4", old)
	// Go's module cache is read-only.
	mod := filepath.Join(dir, "gxf4", "epic", "mod")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{mod, filepath.Dir(mod), filepath.Join(dir, "gxf4")} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(mod, 0o555); err != nil {
		t.Fatal(err)
	}
	mk("gx-epic-live", old)  // named by a running process
	mk("gx-epic-fresh", old) // old dir, but a file inside changed recently
	mk("gxf-dbg", old)       // not a numbered test root
	if err := os.Chtimes(filepath.Join(dir, "gx-epic-fresh", "epic", "run-log.jsonl"), now, now); err != nil {
		t.Fatal(err)
	}
	mk("other-dir", old) // not a swept prefix

	removed := sweepStaleTemp(dir, time.Hour, func() string { return "/x/gx-epic-live/epic" }, now)

	if len(removed) != 4 {
		t.Fatalf("removed %v, want gx-epic-1, go-build2, gxs3 and gxf4", removed)
	}
	for name, want := range map[string]bool{"gx-epic-1": false, "go-build2": false, "gxs3": false, "gxf4": false, "gxf-dbg": true, "gx-epic-live": true, "gx-epic-fresh": true, "other-dir": true} {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", name, err == nil, want)
		}
	}
}
