package codexsession

import (
	"os"
	"path/filepath"
	"testing"
)

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
