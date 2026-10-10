package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEpicEvolveFields_LoadAndSurviveTimingWrite(t *testing.T) {
	t.Parallel()
	scratch := t.TempDir()
	epicDir := filepath.Join(scratch, "epic")
	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fm := "---\nstatus: open\nevolved_at: 2026-01-02T03:04:05Z\nevolve_skipped: true\nevolve_ticket: p:epic/03\nprovisional_destination: true\n---\n\n# epic\n"
	if err := os.WriteFile(filepath.Join(epicDir, "ticket.md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StampEpicStarted(scratch, "epic", time.Now()); err != nil {
		t.Fatal(err)
	}

	epic := Epic{Path: epicDir}
	loadEpicTicketMD(&epic)
	if len(epic.FrontmatterErrs) != 0 {
		t.Fatalf("FrontmatterErrs = %v", epic.FrontmatterErrs)
	}
	if want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC); !epic.EvolvedAt.Equal(want) {
		t.Errorf("EvolvedAt = %v, want %v", epic.EvolvedAt, want)
	}
	if !epic.EvolveSkipped || epic.EvolveTicket != "p:epic/03" || !epic.ProvisionalDestination {
		t.Errorf("epic = %+v, want evolve fields kept", epic)
	}
}
