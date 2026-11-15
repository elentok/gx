package codexsession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadTurnModel_CarriesTheTurnTime(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	body := `{"type":"turn_context","timestamp":"2026-11-01T12:00:00.5Z","payload":{"model":"gpt-6.1-terra","effort":"high"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _, err := ReadTurnModel(path)
	if want := time.Date(2026, 11, 1, 12, 0, 0, 500_000_000, time.UTC); err != nil || !m.At.Equal(want) {
		t.Errorf("At = %v err=%v, want %v", m.At, err, want)
	}
}
