package ralphloop

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

func writeSubagentTranscript(t *testing.T, home, cwd, session, name, usage string) {
	t.Helper()
	dir := filepath.Join(filepath.Dir(transcript.PathIn(home, cwd, session)), session, "subagents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","timestamp":"2026-01-01T00:00:00Z","message":{"model":"claude-sonnet-4-5","usage":` + usage + `}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadSessionStats_TotalTokensIncludeSubagents(t *testing.T) {
	const cwd, session = "/work/tree", "sess-1"
	home := t.TempDir()
	setHomeEnv(t, home)
	writeFakeTranscript(t, home, cwd, session, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), [3]any{"claude-sonnet-4-5", 1000, 200})
	writeSubagentTranscript(t, home, cwd, session, "agent-a.jsonl",
		`{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":30,"cache_creation_input_tokens":40}`)
	writeSubagentTranscript(t, home, cwd, session, "agent-b.jsonl",
		`{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}`)

	got, err := readSessionStats(cwd, session)
	if err != nil {
		t.Fatal(err)
	}
	if want := (1000 + 10 + 200) + (100 + 20 + 30 + 40) + (1 + 2 + 3 + 4); got.totalTokens != want {
		t.Errorf("totalTokens = %d, want %d", got.totalTokens, want)
	}
	wantCost := (1000.0/1e6*3 + 10.0/1e6*15 + 200.0/1e6*0.3) +
		(100.0/1e6*3 + 20.0/1e6*15 + 30.0/1e6*0.3 + 40.0/1e6*3.75) +
		(1.0/1e6*3 + 2.0/1e6*15 + 3.0/1e6*0.3 + 4.0/1e6*3.75)
	if math.Abs(got.cost-wantCost) > 1e-9 {
		t.Errorf("cost = %v, want %v", got.cost, wantCost)
	}
}

func TestSessionsTotals(t *testing.T) {
	const cwd = "/work/tree"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	home := t.TempDir()
	setHomeEnv(t, home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	writeFakeTranscript(t, home, cwd, "sess-a", start, [3]any{"claude-sonnet-4-5", 1000, 0})
	writeFakeTranscript(t, home, cwd, "sess-b", start, [3]any{"claude-sonnet-4-5", 2000, 0})
	claudeCost := 3000.0/1e6*3 + 20.0/1e6*15

	t.Run("two readable sessions add up", func(t *testing.T) {
		tokens, cost, err := SessionsTotals([]SessionRef{
			{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-a"},
			{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-b"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if tokens != 3020 || math.Abs(cost-claudeCost) > 1e-9 {
			t.Errorf("got %d tokens, %v cost; want 3020, %v", tokens, cost, claudeCost)
		}
	})

	t.Run("one missing transcript fails the whole sum", func(t *testing.T) {
		tokens, cost, err := SessionsTotals([]SessionRef{
			{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-a"},
			{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-missing"},
		})
		if err == nil {
			t.Fatal("want error")
		}
		if tokens != 0 || cost != 0 {
			t.Errorf("partial result %d, %v", tokens, cost)
		}
	})

	t.Run("codex session adds its total and no cost", func(t *testing.T) {
		rollout := filepath.Join(home, ".codex", "sessions", "2026", "08", "04", "rollout-2026-08-04T12-00-00-codex-1.jsonl")
		if err := writeCodexSessionMeta(rollout, 1, "codex-1", cwd); err != nil {
			t.Fatal(err)
		}
		if err := writeCodexUsageLine(rollout, 2, 500, 7777); err != nil {
			t.Fatal(err)
		}
		tokens, cost, err := SessionsTotals([]SessionRef{
			{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-a"},
			{Agent: AgentCodex, Cwd: cwd, SessionID: "codex-1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		wantCost := 1000.0/1e6*3 + 10.0/1e6*15
		if tokens != 1010+7777 || math.Abs(cost-wantCost) > 1e-9 {
			t.Errorf("got %d tokens, %v cost; want %d, %v", tokens, cost, 1010+7777, wantCost)
		}
	})
}
