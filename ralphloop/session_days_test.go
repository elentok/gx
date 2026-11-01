package ralphloop

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

func assistantLine(ts string, in, out int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":%d}}}`+"\n", ts, in, out)
}

func TestSessionDaysClaude(t *testing.T) {
	const cwd, session = "/work/tree", "sess-1"
	loc := time.FixedZone("plus2", 2*60*60)
	home := t.TempDir()

	path := transcript.PathIn(home, cwd, session)
	subDir := filepath.Join(filepath.Dir(path), session, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 21:30Z is 23:30 local on Jan 1; 22:30Z is 00:30 local on Jan 2.
	main := assistantLine("2026-01-01T21:30:00Z", 1000, 10) +
		assistantLine("2026-01-01T22:30:00Z", 2000, 20) +
		`{"type":"assistant","timestamp":"2026-01-01T22:`
	if err := os.WriteFile(path, []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	sub := assistantLine("2026-01-01T22:40:00Z", 500, 5)
	if err := os.WriteFile(filepath.Join(subDir, "agent-a.jsonl"), []byte(sub), 0644); err != nil {
		t.Fatal(err)
	}

	days, err := ClaudeSessionDays(home, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Day != "2026-01-01" || days[1].Day != "2026-01-02" {
		t.Fatalf("days = %+v, want Jan 1 and Jan 2", days)
	}
	if days[0].SessionID != session {
		t.Errorf("SessionID = %q, want %q", days[0].SessionID, session)
	}
	if days[0].Tokens != 1010 || days[1].Tokens != 2020+505 {
		t.Errorf("tokens = %d, %d; want 1010 and 2525", days[0].Tokens, days[1].Tokens)
	}

	total, err := readTranscriptStats(path)
	if err != nil {
		t.Fatal(err)
	}
	var tokens int
	var cost float64
	for _, d := range days {
		tokens += d.Tokens
		cost += d.Cost
	}
	if tokens != total.totalTokens || math.Abs(cost-total.cost) > 1e-9 {
		t.Errorf("day sums = %d / %v, want totals %d / %v", tokens, cost, total.totalTokens, total.cost)
	}
}
