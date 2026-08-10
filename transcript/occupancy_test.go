package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	preCompactAssistantLine  = `{"type":"assistant","timestamp":"2026-08-10T10:00:00.000000000Z","message":{"model":"claude-opus-5","usage":{"input_tokens":3,"cache_read_input_tokens":132600,"cache_creation_input_tokens":0,"output_tokens":50}}}`
	postCompactAssistantLine = `{"type":"assistant","timestamp":"2026-08-10T10:00:02.000000000Z","message":{"model":"claude-opus-5","usage":{"input_tokens":1,"cache_read_input_tokens":40000,"cache_creation_input_tokens":0,"output_tokens":10}}}`
	compactBoundaryLine      = `{"type":"system","subtype":"compact_boundary","timestamp":"2026-08-10T10:00:01.000000000Z","compactMetadata":{"trigger":"manual"}}`
)

func TestReadOccupancy_BoundaryNewerThanAssistantTurn_ReportsOccupancyAndStale(t *testing.T) {
	path := writeTranscript(t, preCompactAssistantLine, compactBoundaryLine)

	reading, err := ReadOccupancy(path)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if !reading.Found {
		t.Fatal("ReadOccupancy() Found = false, want true — the occupancy is still reported, only flagged")
	}
	if got := reading.Usage.Occupancy(); got != 132603 {
		t.Errorf("Occupancy() = %d, want 132603", got)
	}
	if !reading.Stale {
		t.Error("Stale = false, want true with a compaction boundary after the last assistant turn")
	}
}

func TestReadOccupancy_AssistantTurnAfterBoundary_FlagClear(t *testing.T) {
	path := writeTranscript(t, preCompactAssistantLine, compactBoundaryLine, postCompactAssistantLine)

	reading, err := ReadOccupancy(path)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if !reading.Found {
		t.Fatal("ReadOccupancy() Found = false, want true")
	}
	if got := reading.Usage.Occupancy(); got != 40001 {
		t.Errorf("Occupancy() = %d, want 40001 (the post-compaction turn)", got)
	}
	if reading.Stale {
		t.Error("Stale = true, want false once a post-compaction assistant turn has landed")
	}
}

func TestReadOccupancy_IdenticalTimestamps_OrderedByFilePosition(t *testing.T) {
	const ts = "2026-08-10T10:00:01.000000000Z"
	boundary := strings.Replace(compactBoundaryLine, "2026-08-10T10:00:01.000000000Z", ts, 1)
	assistant := strings.Replace(postCompactAssistantLine, "2026-08-10T10:00:02.000000000Z", ts, 1)

	stalePath := writeTranscript(t, assistant, boundary)
	stale, err := ReadOccupancy(stalePath)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if !stale.Stale {
		t.Error("boundary last in file: Stale = false, want true — same timestamp must not read as a tie")
	}

	freshPath := writeTranscript(t, boundary, assistant)
	fresh, err := ReadOccupancy(freshPath)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if fresh.Stale {
		t.Error("assistant last in file: Stale = true, want false — same timestamp must not read as a tie")
	}
}

func TestReadOccupancy_BoundaryOlderThanLastAssistantTurn_FlagClear(t *testing.T) {
	path := writeTranscript(t,
		preCompactAssistantLine,
		compactBoundaryLine,
		postCompactAssistantLine,
		`{"type":"user","timestamp":"2026-08-10T10:00:03.000000000Z","message":{}}`,
	)

	reading, err := ReadOccupancy(path)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if reading.Stale {
		t.Error("Stale = true, want false — a boundary already answered by a later assistant turn must not suppress forever")
	}
}

func TestReadOccupancy_NoBoundaryAtAll_ReportsFreshOccupancy(t *testing.T) {
	path := writeTranscript(t, `{"type":"user","message":{}}`, preCompactAssistantLine)

	reading, err := ReadOccupancy(path)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if !reading.Found || reading.Stale {
		t.Errorf("reading = %+v, want Found=true Stale=false", reading)
	}
}

func TestReadOccupancy_NoAssistantLineAtAll_NotFoundButBoundaryStillFlagged(t *testing.T) {
	path := writeTranscript(t, `{"type":"user","message":{}}`, compactBoundaryLine)

	reading, err := ReadOccupancy(path)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if reading.Found {
		t.Error("Found = true, want false with no assistant line anywhere")
	}
	if !reading.Stale {
		t.Error("Stale = false, want true — a boundary with no assistant turn after it is exactly the stale case")
	}
}

func TestReadOccupancy_EmptyAndMissingFile_NotFoundNotStale(t *testing.T) {
	missing, err := ReadOccupancy(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatalf("ReadOccupancy(missing) error = %v", err)
	}
	if missing.Found || missing.Stale {
		t.Errorf("missing file: reading = %+v, want the zero reading", missing)
	}

	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	reading, err := ReadOccupancy(empty)
	if err != nil {
		t.Fatalf("ReadOccupancy(empty) error = %v", err)
	}
	if reading.Found || reading.Stale {
		t.Errorf("empty file: reading = %+v, want the zero reading", reading)
	}
}

func TestReadOccupancy_BoundaryAndTurnFarBeforeInitialTailWindow_StillFound(t *testing.T) {
	lines := []string{preCompactAssistantLine, compactBoundaryLine}
	// Pad past initialTailBytes so the first tail read can't reach either
	// line, forcing the doubling passes to re-derive the same verdict.
	padding := strings.Repeat("x", initialTailBytes*3)
	for range 500 {
		lines = append(lines, `{"type":"user","message":{}}`+"// "+padding)
	}
	path := writeTranscript(t, lines...)

	reading, err := ReadOccupancy(path)
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if !reading.Found || reading.Usage.Occupancy() != 132603 {
		t.Errorf("reading = %+v, want the assistant line found across doubling passes", reading)
	}
	if !reading.Stale {
		t.Error("Stale = false, want true — the boundary is still newer than the assistant turn")
	}
}

func TestReadSessionOccupancy_ReadsThroughPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cwd := "/repo/worktree"
	slugDir := filepath.Join(dir, ".claude", "projects", Slugify(cwd))
	if err := os.MkdirAll(slugDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sessionPath := filepath.Join(slugDir, "sess-1.jsonl")
	body := strings.Join([]string{preCompactAssistantLine, compactBoundaryLine}, "\n") + "\n"
	if err := os.WriteFile(sessionPath, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reading, err := ReadSessionOccupancy(cwd, "sess-1")
	if err != nil {
		t.Fatalf("ReadSessionOccupancy() error = %v", err)
	}
	if !reading.Found || reading.Usage.Occupancy() != 132603 || !reading.Stale {
		t.Errorf("reading = %+v, want Found=true Occupancy=132603 Stale=true", reading)
	}
}
