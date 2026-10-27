package tickets

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

func writeEpicTicketMD(t *testing.T, content string) string {
	t.Helper()
	epic := t.TempDir()
	if err := os.WriteFile(filepath.Join(epic, "ticket.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return epic
}

func TestStampEpicDesignStarted_RoundTripsAndKeepsTheFirstStamp(t *testing.T) {
	epic := writeEpicTicketMD(t, "---\nkind: map\n---\n\n# epic\n")
	first := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := StampEpicDesignStarted(epic, first); err != nil {
		t.Fatal(err)
	}
	if err := StampEpicDesignStarted(epic, first.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := LoadEpicTicketMD(epic).DesignStartedAt; !got.Equal(first) {
		t.Errorf("DesignStartedAt = %v, want %v", got, first)
	}
}

func TestLoadEpicTicketMD_ReadsTheStatsFields(t *testing.T) {
	epic := writeEpicTicketMD(t, `---
status: open
lines_added: 120
lines_removed: 30
total_tokens: 4500
actual_cost: 1.25
had_codex: true
friction:
  pauses_by_reason:
    ask: 2
  total_paused_secs: 90
  person_pause_secs: [30, 60]
---

# epic
`)

	got := LoadEpicTicketMD(epic)

	if got.LinesAdded == nil || *got.LinesAdded != 120 {
		t.Errorf("LinesAdded = %v, want 120", got.LinesAdded)
	}
	if got.LinesRemoved == nil || *got.LinesRemoved != 30 {
		t.Errorf("LinesRemoved = %v, want 30", got.LinesRemoved)
	}
	if got.TotalTokens == nil || *got.TotalTokens != 4500 {
		t.Errorf("TotalTokens = %v, want 4500", got.TotalTokens)
	}
	if got.ActualCost != 1.25 {
		t.Errorf("ActualCost = %v, want 1.25", got.ActualCost)
	}
	if !got.HadCodex {
		t.Error("HadCodex = false, want true")
	}
	want := &schema.FrictionSummary{
		PausesByReason:  map[string]int{"ask": 2},
		TotalPausedSecs: 90,
		PersonPauseSecs: []int{30, 60},
	}
	if !reflect.DeepEqual(got.Friction, want) {
		t.Errorf("Friction = %+v, want %+v", got.Friction, want)
	}
}

func TestLoadEpicTicketMD_LeavesTheStatsFieldsUnsetWhenAbsent(t *testing.T) {
	got := LoadEpicTicketMD(writeEpicTicketMD(t, "---\nstatus: open\n---\n\n# epic\n"))

	if got.LinesAdded != nil || got.LinesRemoved != nil || got.TotalTokens != nil || got.Friction != nil {
		t.Errorf("want the pointer fields unset, got %+v", got)
	}
	if got.ActualCost != 0 || got.HadCodex {
		t.Errorf("ActualCost = %v, HadCodex = %v, want zero values", got.ActualCost, got.HadCodex)
	}
}

func TestStampEpicFields_WritesTheStatsKeysAndNeverOverwritesThem(t *testing.T) {
	epic := writeEpicTicketMD(t, "---\nstatus: open\ntotal_tokens: 10\n---\n\n# epic\n")

	err := StampEpicFields(epic, map[string]any{
		"lines_added":   7,
		"lines_removed": 3,
		"total_tokens":  99,
		"actual_cost":   0.5,
		"had_codex":     true,
		"friction":      schema.FrictionSummary{TotalPausedSecs: 5},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := LoadEpicTicketMD(epic)
	if got.LinesAdded == nil || *got.LinesAdded != 7 || got.LinesRemoved == nil || *got.LinesRemoved != 3 {
		t.Errorf("lines = %v/%v, want 7/3", got.LinesAdded, got.LinesRemoved)
	}
	if got.TotalTokens == nil || *got.TotalTokens != 10 {
		t.Errorf("TotalTokens = %v, want the existing 10 kept", got.TotalTokens)
	}
	if got.ActualCost != 0.5 || !got.HadCodex {
		t.Errorf("ActualCost = %v, HadCodex = %v, want 0.5/true", got.ActualCost, got.HadCodex)
	}
	if got.Friction == nil || got.Friction.TotalPausedSecs != 5 {
		t.Errorf("Friction = %+v, want TotalPausedSecs 5", got.Friction)
	}
	raw, _ := os.ReadFile(filepath.Join(epic, "ticket.md"))
	if strings.Contains(string(raw), "total_tokens: 99") {
		t.Errorf("existing total_tokens was overwritten:\n%s", raw)
	}
}
