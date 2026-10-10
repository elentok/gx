package ralphloop

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

func TestReadSessionStats_SubagentTranscripts(t *testing.T) {
	const cwd, session = "/work/tree", "sess-1"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	setup := func(t *testing.T) (home string) {
		home = t.TempDir()
		setHomeEnv(t, home)
		writeFakeTranscript(t, home, cwd, session, start, [3]any{"claude-sonnet-4-5", 1000, 0})
		return home
	}

	t.Run("sums subagent cost, keeps peak occupancy of the main transcript", func(t *testing.T) {
		home := setup(t)
		main, err := readSessionStats(cwd, session)
		if err != nil {
			t.Fatal(err)
		}

		subDir := filepath.Join(filepath.Dir(transcript.PathIn(home, cwd, session)), session, "subagents")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		subPath := filepath.Join(subDir, "agent-a.jsonl")
		sub := `{"type":"assistant","timestamp":"2026-01-01T00:00:00Z","message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":900000,"output_tokens":10}}}` + "\n"
		if err := os.WriteFile(subPath, []byte(sub), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := readSessionStats(cwd, session)
		if err != nil {
			t.Fatal(err)
		}
		if got.peakOccupancy != main.peakOccupancy {
			t.Errorf("peakOccupancy = %d, want %d", got.peakOccupancy, main.peakOccupancy)
		}
		want := main.cost + 900000.0/1e6*3 + 10.0/1e6*15
		if math.Abs(got.cost-want) > 1e-9 {
			t.Errorf("cost = %v, want %v", got.cost, want)
		}
	})

	t.Run("no subagents dir leaves cost unchanged", func(t *testing.T) {
		setup(t)
		got, err := readSessionStats(cwd, session)
		if err != nil {
			t.Fatal(err)
		}
		want := 1000.0/1e6*3 + 10.0/1e6*15
		if math.Abs(got.cost-want) > 1e-9 {
			t.Errorf("cost = %v, want %v", got.cost, want)
		}
	})
}
