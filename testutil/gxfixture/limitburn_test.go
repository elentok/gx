package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

type seededHistory struct {
	Samples map[string][]server.LimitSample `json:"samples"`
	Latest  map[string]server.LimitSample   `json:"latest"`
}

func readSeededHistory(t *testing.T, stateDir string) seededHistory {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, "limit-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	var h seededHistory
	if err := json.Unmarshal(data, &h); err != nil {
		t.Fatal(err)
	}
	return h
}

// dayGain is how much a limit rose within one local day (daysAgo 0 is today):
// the last sample of the day minus the last one before it.
func dayGain(samples []server.LimitSample, now time.Time, daysAgo int) float64 {
	m := localMidnight(now).AddDate(0, 0, -daysAgo)
	var before, last float64
	for _, s := range samples {
		at := time.Unix(s.ReceivedAt, 0)
		switch {
		case at.Before(m):
			before = s.UsedPercentage
		case at.Before(m.AddDate(0, 0, 1)):
			last = s.UsedPercentage
		}
	}
	return last - before
}

func daysCovered(samples []server.LimitSample, now time.Time) int {
	if len(samples) == 0 {
		return 0
	}
	first := localMidnight(time.Unix(samples[0].ReceivedAt, 0))
	return int(localMidnight(now).Sub(first).Round(24*time.Hour)/(24*time.Hour)) + 1
}

func TestLimitBurnFixture_ShapesSeedTheHistory(t *testing.T) {
	for _, tc := range []struct {
		shape string
		check func(t *testing.T, now time.Time, l server.LimitsReport, h seededHistory)
	}{
		{"", func(t *testing.T, now time.Time, l server.LimitsReport, h seededHistory) {
			if l.SevenDay == nil || l.SevenDay.UsedPercentage != 81 {
				t.Errorf("default shape is not menu: %+v", l)
			}
		}},
		{"menu", func(t *testing.T, now time.Time, l server.LimitsReport, h seededHistory) {
			if l.FiveHour == nil || l.SevenDay == nil || l.SevenDay.UsedPercentage != 81 {
				t.Fatalf("want Claude 5h and weekly at 81%%: %+v", l)
			}
			claude := h.Samples[server.LimitIDClaudeWeekly]
			if got := daysCovered(claude, now); got < 7 {
				t.Errorf("Claude weekly history covers %d days, want 7", got)
			}
			if today, yesterday := dayGain(claude, now, 0), dayGain(claude, now, 1); today != 7 || yesterday != 5 {
				t.Errorf("Claude weekly gained %v today and %v yesterday, want 7 and 5", today, yesterday)
			}
			if codex := h.Latest[server.LimitIDCodexWeekly]; codex.UsedPercentage != 40 || codex.ResetsAt <= now.Unix() {
				t.Errorf("Codex weekly latest = %+v, want 40%% with the reset ahead", codex)
			}
			for _, id := range []string{server.LimitIDClaude5h, server.LimitIDCodex5h} {
				if len(h.Samples[id]) == 0 {
					t.Errorf("no history for %s", id)
				}
			}
		}},
		{"few-days", func(t *testing.T, now time.Time, l server.LimitsReport, h seededHistory) {
			if l.SevenDay == nil || l.SevenDay.UsedPercentage != 22 {
				t.Fatalf("want Claude weekly at 22%%: %+v", l)
			}
			claude := h.Samples[server.LimitIDClaudeWeekly]
			if first := time.Unix(claude[0].ReceivedAt, 0); first.Before(localMidnight(now)) || !first.Before(now) {
				t.Errorf("first sample at %v, want earlier today", first)
			}
			if _, ok := h.Latest[server.LimitIDCodexWeekly]; ok || len(h.Samples[server.LimitIDCodexWeekly]) > 0 {
				t.Errorf("Codex data seeded: %+v", h.Samples)
			}
		}},
		{"stale", func(t *testing.T, now time.Time, l server.LimitsReport, h seededHistory) {
			for _, id := range []string{server.LimitIDClaudeWeekly, server.LimitIDCodexWeekly} {
				samples := h.Samples[id]
				if got := daysCovered(samples, now); got < 7 {
					t.Errorf("%s history covers %d days, want 7", id, got)
				}
				latest := h.Latest[id]
				if age := now.Sub(time.Unix(latest.ReceivedAt, 0)); age < 10*time.Minute {
					t.Errorf("%s newest reading %v old, want over 10m", id, age)
				}
				if latest.ResetsAt <= now.Unix() {
					t.Errorf("%s reset is not in the future", id)
				}
			}
		}},
		{"past-reset", func(t *testing.T, now time.Time, l server.LimitsReport, h seededHistory) {
			if l.SevenDay == nil || l.SevenDay.ResetsAt >= now.Unix() {
				t.Errorf("weekly reset is not in the past: %+v", l.SevenDay)
			}
		}},
	} {
		t.Run(tc.shape, func(t *testing.T) {
			var flags []string
			if tc.shape != "" {
				flags = []string{"--shape", tc.shape}
			}
			r := startWith(t, "limit-burn", nil, flags...)
			defer r.stop(t)
			var sum server.UsageSummary
			if err := json.Unmarshal([]byte(r.gx(t, "usage", "summary", "--json")), &sum); err != nil {
				t.Fatal(err)
			}
			tc.check(t, time.Now(), sum.Limits, readSeededHistory(t, r.StateDir))
		})
	}
}

func TestLimitBurnFixture_UnknownShapeNamesTheValidOnes(t *testing.T) {
	out, err := exec.Command(fixtureBin, "--gx", gxBin, "--fakeclaude", fakeBin, "--shape", "bogus", "limit-burn").CombinedOutput()
	if err == nil {
		t.Fatal("unknown shape started")
	}
	for _, name := range []string{"menu", "few-days", "stale", "past-reset"} {
		if !strings.Contains(string(out), name) {
			t.Errorf("error does not list %q:\n%s", name, out)
		}
	}
}

// A line appended to the held Codex worker's rollout makes the usage scan save
// a new Codex weekly sample.
func TestLimitBurnFixture_AppendedTokenCountSavesACodexSample(t *testing.T) {
	r := startWith(t, "limit-burn", nil)
	defer r.stop(t)
	before := len(readSeededHistory(t, r.StateDir).Samples[server.LimitIDCodexWeekly])

	var rollout string
	deadline := time.Now().Add(30 * time.Second)
	for rollout == "" && time.Now().Before(deadline) {
		_ = filepath.WalkDir(r.CodexSessionsDir, func(path string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasPrefix(e.Name(), "rollout-") {
				rollout = path
			}
			return nil
		})
		time.Sleep(100 * time.Millisecond)
	}
	if rollout == "" {
		t.Fatalf("no rollout under %s", r.CodexSessionsDir)
	}
	line := fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":55,"window_minutes":10080,"resets_at":%d}}}}`+"\n",
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), time.Now().Add(72*time.Hour).Unix())
	f, err := os.OpenFile(rollout, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	f.Close()

	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if len(readSeededHistory(t, r.StateDir).Samples[server.LimitIDCodexWeekly]) > before {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no new Codex weekly sample after the appended line (had %d)", before)
}
