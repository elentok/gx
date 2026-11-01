package codexsession

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tokenCountLine(ts string, total int) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":%d}}}}`+"\n", ts, total)
}

func TestSessionDaysCodex(t *testing.T) {
	const cwd, id = "/work/tree", "abc-123"
	loc := time.FixedZone("plus2", 2*60*60)
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "01", "01")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// 21:30Z is 23:30 local on Jan 1; 22:30Z is 00:30 local on Jan 2.
	body := fmt.Sprintf(`{"timestamp":"2026-01-01T21:00:00Z","type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n", id, cwd) +
		tokenCountLine("2026-01-01T21:30:00Z", 100) +
		tokenCountLine("2026-01-01T22:30:00Z", 250) +
		tokenCountLine("2026-01-01T22:40:00Z", 400) +
		`{"timestamp":"2026-01-01T22:50:00Z","type":"event_msg","payload":{"type":"token_`
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-01-01T21-00-00-"+id+".jsonl"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	days, err := SessionDaysIn(home, loc)
	if err != nil {
		t.Fatal(err)
	}
	want := []DayUsage{
		{SessionID: id, Cwd: cwd, Day: "2026-01-01", Tokens: 100},
		{SessionID: id, Cwd: cwd, Day: "2026-01-02", Tokens: 300},
	}
	if len(days) != len(want) || days[0] != want[0] || days[1] != want[1] {
		t.Fatalf("days = %+v, want %+v", days, want)
	}

	stats, ok, err := ReadStatsIn(home, cwd, id)
	if err != nil || !ok {
		t.Fatalf("ReadStatsIn ok=%v err=%v", ok, err)
	}
	if sum := days[0].Tokens + days[1].Tokens; sum != stats.TotalTokens {
		t.Errorf("day sum = %d, want total %d", sum, stats.TotalTokens)
	}
}
