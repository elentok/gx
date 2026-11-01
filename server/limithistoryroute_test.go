package server_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
)

func TestLimitHistory_ReportSurvivesRestart(t *testing.T) {
	h := limitsHarness(t)
	dir := h.StateDir
	sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(10, 4_000_000_000), SevenDay: limitWin(3, 4_000_000_001)})
	sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(10, 4_000_000_000)}) // unchanged: no sample
	sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(25.5, 4_000_000_000)})
	h.Restart(t)

	data, err := os.ReadFile(filepath.Join(dir, "limit-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Samples map[string][]server.LimitSample `json:"samples"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	five := f.Samples[server.LimitIDClaude5h]
	if len(five) != 2 || five[0].UsedPercentage != 10 || five[1].UsedPercentage != 25.5 || five[1].ResetsAt != 4_000_000_000 || five[1].ReceivedAt <= 0 {
		t.Errorf("5h samples = %+v, want 10 then 25.5", five)
	}
	if n := len(f.Samples[server.LimitIDClaudeWeekly]); n != 1 {
		t.Errorf("weekly samples = %d, want 1", n)
	}

	// A report after the restart appends to what was loaded.
	sendLimits(t, h, server.LimitsReport{FiveHour: limitWin(40, 4_000_000_000)})
	data, _ = os.ReadFile(filepath.Join(dir, "limit-history.json"))
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if n := len(f.Samples[server.LimitIDClaude5h]); n != 3 {
		t.Errorf("5h samples after restart = %d, want 3", n)
	}
}
