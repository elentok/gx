package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
)

var localMidnight = limitseed.LocalMidnight

type seededHistory struct {
	Samples map[string][]wire.LimitSample `json:"samples"`
	Latest  map[string]wire.LimitSample   `json:"latest"`
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
func dayGain(samples []wire.LimitSample, now time.Time, daysAgo int) float64 {
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

func daysCovered(samples []wire.LimitSample, now time.Time) int {
	if len(samples) == 0 {
		return 0
	}
	first := localMidnight(time.Unix(samples[0].ReceivedAt, 0))
	return int(localMidnight(now).Sub(first).Round(24*time.Hour)/(24*time.Hour)) + 1
}

func TestLimitBurnFixture_ShapesSeedTheHistory(t *testing.T) {
	for _, tc := range []struct {
		shape string
		check func(t *testing.T, now time.Time, l wire.UsageLimits, h seededHistory)
	}{
		{"", func(t *testing.T, now time.Time, l wire.UsageLimits, h seededHistory) {
			if l.SevenDay == nil || l.SevenDay.UsedPercentage != 81 {
				t.Errorf("default shape is not menu: %+v", l)
			}
		}},
		{"menu", func(t *testing.T, now time.Time, l wire.UsageLimits, h seededHistory) {
			if l.FiveHour == nil || l.SevenDay == nil || l.SevenDay.UsedPercentage != 81 {
				t.Fatalf("want Claude 5h and weekly at 81%%: %+v", l)
			}
			claude := h.Samples[wire.LimitIDClaudeWeekly]
			if got := daysCovered(claude, now); got < 7 {
				t.Errorf("Claude weekly history covers %d days, want 7", got)
			}
			if today, yesterday := dayGain(claude, now, 0), dayGain(claude, now, 1); today != 7 || yesterday != 5 {
				t.Errorf("Claude weekly gained %v today and %v yesterday, want 7 and 5", today, yesterday)
			}
			if codex := h.Latest[wire.LimitIDCodexWeekly]; codex.UsedPercentage != 40 || codex.ResetsAt <= now.Unix() {
				t.Errorf("Codex weekly latest = %+v, want 40%% with the reset ahead", codex)
			}
			for _, id := range []string{wire.LimitIDClaude5h, wire.LimitIDCodex5h} {
				if len(h.Samples[id]) == 0 {
					t.Errorf("no history for %s", id)
				}
			}
		}},
		{"few-days", func(t *testing.T, now time.Time, l wire.UsageLimits, h seededHistory) {
			if l.SevenDay == nil || l.SevenDay.UsedPercentage != 22 {
				t.Fatalf("want Claude weekly at 22%%: %+v", l)
			}
			claude := h.Samples[wire.LimitIDClaudeWeekly]
			if first := time.Unix(claude[0].ReceivedAt, 0); first.Before(localMidnight(now)) || !first.Before(now) {
				t.Errorf("first sample at %v, want earlier today", first)
			}
			if _, ok := h.Latest[wire.LimitIDCodexWeekly]; ok || len(h.Samples[wire.LimitIDCodexWeekly]) > 0 {
				t.Errorf("Codex data seeded: %+v", h.Samples)
			}
		}},
		{"stale", func(t *testing.T, now time.Time, l wire.UsageLimits, h seededHistory) {
			for _, id := range []string{wire.LimitIDClaudeWeekly, wire.LimitIDCodexWeekly} {
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
		{"past-reset", func(t *testing.T, now time.Time, l wire.UsageLimits, h seededHistory) {
			if l.SevenDay == nil || l.SevenDay.ResetsAt >= now.Unix() {
				t.Errorf("weekly reset is not in the past: %+v", l.SevenDay)
			} else if l.SevenDay.Chip == nil {
				t.Error("past-reset weekly row has no chip")
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
			var sum wire.UsageSummary
			if err := json.Unmarshal([]byte(r.gx(t, "usage", "summary", "--json")), &sum); err != nil {
				t.Fatal(err)
			}
			tc.check(t, time.Now(), sum.Limits, readSeededHistory(t, r.StateDir))
		})
	}
}

func TestLimitBurnFixture_StatsShape(t *testing.T) {
	r := startWith(t, "limit-burn", nil, "--shape", "stats")
	defer r.stop(t)
	st, err := apiclient.New(r.Socket).Reads.Stats(context.Background(), 30, "")
	if err != nil {
		t.Fatal(err)
	}

	for name, b := range map[string]wire.StatsLimitBurn{
		"claude_5h": st.LimitBurn.Claude5h, "claude_weekly": st.LimitBurn.ClaudeWeekly,
		"codex_5h": st.LimitBurn.Codex5h, "codex_weekly": st.LimitBurn.CodexWeekly,
	} {
		if len(b.Days) != 30 {
			t.Fatalf("%s has %d days, want 30", name, len(b.Days))
		}
		head, resets := 0, 0
		for i, d := range b.Days {
			if d.NoData != (i < 7) {
				t.Errorf("%s day %d (%s) no_data = %v", name, i, d.Date, d.NoData)
			}
			if d.NoData {
				head++
			}
			if d.ResetDay {
				resets++
			}
		}
		if head != 7 || resets == 0 {
			t.Errorf("%s: %d no-data days and %d reset days, want 7 and some", name, head, resets)
		}
		if b.StartDay != b.Days[7].Date {
			t.Errorf("%s start_day = %q, want %q", name, b.StartDay, b.Days[7].Date)
		}
	}

	// Days 0-8 are stamp-only (day 4 has no ticket), 9-18 rebuilt, 19-29 live
	// (day 24 is the quiet one).
	for i, src := range st.Cost.Sources {
		want := "live"
		switch {
		case i < 9:
			want = "stamp"
		case i < 19:
			want = "rebuilt"
		}
		if src != want {
			t.Errorf("source of day %d = %q, want %q", i, src, want)
		}
	}
	groups := map[string]wire.StatsCostGroup{}
	for _, g := range st.Cost.Groups {
		groups[g.Group] = g
	}
	harnesses := map[string]bool{}
	for _, name := range []string{"Queue", "Design", "One-off", "Other"} {
		g := groups[name]
		total := 0.0
		for i, v := range g.Tokens {
			total += v.Value
			if i >= 9 {
				for h := range g.Harness[i] {
					harnesses[h] = true
				}
			}
		}
		if total == 0 {
			t.Errorf("group %s has no tokens", name)
		}
	}
	if len(harnesses) != 2 {
		t.Errorf("harnesses in the ledger days = %v, want claude and codex", harnesses)
	}
	if q := groups["Queue"].Tokens; q[0].Value == 0 || q[0].NoData || !q[4].NoData {
		t.Errorf("stamp days: day 0 = %v (want stamped), day 4 = %v (want no data)", q[0], q[4])
	}
	for _, g := range st.Cost.Groups {
		var sum float64
		for _, v := range g.Tokens[24:25] {
			sum += v.Value
		}
		if sum != 0 {
			t.Errorf("group %s has tokens on the quiet live day", g.Group)
		}
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
	before := len(readSeededHistory(t, r.StateDir).Samples[wire.LimitIDCodexWeekly])

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
		if len(readSeededHistory(t, r.StateDir).Samples[wire.LimitIDCodexWeekly]) > before {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no new Codex weekly sample after the appended line (had %d)", before)
}

// appendUsage adds one assistant line with the given input tokens to a transcript.
func appendUsage(t *testing.T, path string, tokens int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":0}}}`+"\n",
		time.Now().Format(time.RFC3339), tokens)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

// todaysTokens is each cost group's tokens for today in /v1/stats.
func todaysTokens(t *testing.T, socket string) map[string]float64 {
	t.Helper()
	st, err := apiclient.New(socket).Reads.Stats(context.Background(), 7, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, g := range st.Cost.Groups {
		out[g.Group] = g.Tokens[len(g.Tokens)-1].Value
	}
	return out
}

func TestLimitBurnFixtureLive(t *testing.T) {
	r := startWith(t, "limit-burn", nil, "--shape", "live")
	defer r.stop(t)
	groups := map[string]string{"queue": "Queue", "design": "Design", "one_off": "One-off", "other": "Other"}
	for key := range groups {
		if r.Transcripts[key] == "" {
			t.Fatalf("no %s transcript in the output: %v", key, r.Transcripts)
		}
	}

	// The first poll that finds a transcript only stores its total, so a baseline
	// line goes in first and the poll gets time to see it.
	for _, p := range r.Transcripts {
		appendUsage(t, p, 1000)
	}
	time.Sleep(3 * time.Second)
	before := todaysTokens(t, r.Socket)

	// waitFor polls until every group has gained want tokens since the baseline.
	waitFor := func(want float64, step string) {
		t.Helper()
		var got map[string]float64
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			got = todaysTokens(t, r.Socket)
			ok := true
			for _, name := range groups {
				ok = ok && got[name]-before[name] == want
			}
			if ok {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("%s: tokens today = %v from %v, want +%v in every group", step, got, before, want)
	}

	for _, p := range r.Transcripts {
		appendUsage(t, p, 250)
	}
	waitFor(250, "after the first append")
	for _, p := range r.Transcripts {
		appendUsage(t, p, 100)
	}
	waitFor(350, "after the second append")
}
