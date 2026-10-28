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
		writeFakeTranscript(t, home, cwd, session, start, [3]any{"claude-sonnet-4-5", 1000, 0})
		return home
	}

	t.Run("sums subagent cost, keeps peak occupancy of the main transcript", func(t *testing.T) {
		home := setup(t)
		main, err := readSessionStats(home, cwd, session)
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

		got, err := readSessionStats(home, cwd, session)
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
		home := setup(t)
		got, err := readSessionStats(home, cwd, session)
		if err != nil {
			t.Fatal(err)
		}
		want := 1000.0/1e6*3 + 10.0/1e6*15
		if math.Abs(got.cost-want) > 1e-9 {
			t.Errorf("cost = %v, want %v", got.cost, want)
		}
	})
}

func TestReadSessionStats_UndatedBoundaryDoesNotBoundSession(t *testing.T) {
	const cwd, session = "/work/tree", "sess-1"
	home := t.TempDir()
	path := transcript.PathIn(home, cwd, session)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"system","subtype":"compact_boundary","compactMetadata":{"trigger":"manual"}}` + "\n" +
		`{"type":"user","timestamp":"2026-01-01T00:00:01Z","message":{}}` + "\n" +
		`{"type":"user","timestamp":"2026-01-01T00:00:05Z","message":{}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := readSessionStats(home, cwd, session)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC); !got.start.Equal(want) {
		t.Errorf("start = %v, want %v", got.start, want)
	}
	if want := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC); !got.end.Equal(want) {
		t.Errorf("end = %v, want %v", got.end, want)
	}
}
