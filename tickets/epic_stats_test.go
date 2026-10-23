package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

func TestSumEpicStats_FrictionIncludesCancelledButTokensStayDoneOnly(t *testing.T) {
	doneTokens, cancelledTokens := int64(100), int64(900)
	ts := []Ticket{
		{Status: "done", TotalTokens: &doneTokens, ActualCost: 1, Friction: &schema.FrictionSummary{TotalPausedSecs: 10}},
		{Status: "cancelled", TotalTokens: &cancelledTokens, ActualCost: 5, HadCodex: true, Friction: &schema.FrictionSummary{TotalPausedSecs: 7}},
	}

	got := SumEpicStats(ts)

	if got.Friction.TotalPausedSecs != 17 {
		t.Errorf("Friction.TotalPausedSecs = %d, want 17", got.Friction.TotalPausedSecs)
	}
	if got.TotalTokens == nil || *got.TotalTokens != 100 {
		t.Errorf("TotalTokens = %v, want 100", got.TotalTokens)
	}
	if got.ActualCost != 1 {
		t.Errorf("ActualCost = %v, want 1", got.ActualCost)
	}
	if got.HadCodex {
		t.Error("HadCodex = true, want false: cancelled tickets are not counted")
	}
}

func TestBackfillFriction_EpicSumIncludesCancelledTicket(t *testing.T) {
	root := t.TempDir()
	epicDir := filepath.Join(root, "e")
	if err := os.MkdirAll(filepath.Join(epicDir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"ticket.md":         "---\nkind: loop\nstatus: open\nmerged_at: 2026-01-01T00:00:00Z\n---\n\n# e\n",
		"issues/01-done.md": "---\nid: \"01\"\nstatus: done\ntype: implement\nfriction:\n  total_paused_secs: 10\n---\nBody.\n",
		"issues/02-gone.md": "---\nid: \"02\"\nstatus: cancelled\ntype: implement\n---\nBody.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(epicDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := BackfillFriction(root, func(epicName string, tk Ticket) (schema.FrictionSummary, error) {
		return schema.FrictionSummary{TotalPausedSecs: 7}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	got := LoadEpicTicketMD(epicDir)
	if got.Friction == nil || got.Friction.TotalPausedSecs != 17 {
		t.Errorf("epic Friction = %+v, want TotalPausedSecs 17", got.Friction)
	}
}
