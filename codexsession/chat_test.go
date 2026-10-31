package codexsession

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRolloutLookupPrefersLiveOverArchive(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	live := filepath.Join(home, "sessions", "2026", "01", "01", "rollout-x-abc.jsonl")
	plainArchive := filepath.Join(home, "archived_sessions", "rollout-x-abc.jsonl")
	zst := filepath.Join(home, "archived_sessions", "rollout-x-abc.jsonl.zst")
	touch(t, zst)
	if _, ok, err := RolloutPathIn(home, "abc"); err != nil || ok {
		t.Errorf("zst only: RolloutPathIn ok = %v, err = %v, want false, nil", ok, err)
	}
	if got, ok, err := ArchivedRolloutIn(home, "abc"); err != nil || !ok || got != zst {
		t.Errorf("ArchivedRolloutIn = %q, %v, %v, want %q", got, ok, err, zst)
	}
	touch(t, plainArchive)
	if got, ok, _ := RolloutPathIn(home, "abc"); !ok || got != plainArchive {
		t.Errorf("plain archive: got %q, %v, want %q", got, ok, plainArchive)
	}
	touch(t, live)
	if got, ok, _ := RolloutPathIn(home, "abc"); !ok || got != live {
		t.Errorf("live: got %q, %v, want %q", got, ok, live)
	}
	if _, ok, err := ArchivedRolloutIn(t.TempDir(), "abc"); err != nil || ok {
		t.Errorf("empty home: ok = %v, err = %v, want false, nil", ok, err)
	}
}

func TestRolloutPathIn(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	want := filepath.Join(home, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-abc.jsonl")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := RolloutPathIn(home, "abc"); err != nil || !ok || got != want {
		t.Errorf("RolloutPathIn = %q, %v, %v, want %q", got, ok, err, want)
	}
	if _, ok, err := RolloutPathIn(home, "other"); err != nil || ok {
		t.Errorf("unknown id: ok = %v, err = %v, want false, nil", ok, err)
	}
	if _, ok, err := RolloutPathIn(t.TempDir(), "abc"); err != nil || ok {
		t.Errorf("no sessions dir: ok = %v, err = %v, want false, nil", ok, err)
	}
}
