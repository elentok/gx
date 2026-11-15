package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLastAssistantReading_CarriesTheNewestAssistantTime(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	body := `{"type":"assistant","timestamp":"2026-11-01T12:00:00Z","message":{"model":"a","usage":{"input_tokens":5}}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-11-01T12:05:00Z","message":{"model":"<synthetic>","usage":{"input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	u, _, err := LastAssistantReading(path)
	if want := time.Date(2026, 11, 1, 12, 5, 0, 0, time.UTC); err != nil || !u.At.Equal(want) {
		t.Errorf("At = %v err=%v, want %v", u.At, err, want)
	}
}
