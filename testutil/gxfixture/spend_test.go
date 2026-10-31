package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func TestSpendFixture_ShapesShowInTheSummary(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	for _, tc := range []struct {
		shape string
		check func(t *testing.T, s server.UsageSpend)
	}{
		{"", func(t *testing.T, s server.UsageSpend) {
			if s.Today != 4.82 || s.SinceSunday < s.Today || s.SinceFirst < s.SinceSunday {
				t.Errorf("default shape is not normal: %+v", s)
			}
		}},
		{"normal", func(t *testing.T, s server.UsageSpend) {
			if s.Today != 4.82 || s.SinceSunday < s.Today || s.SinceFirst < s.SinceSunday {
				t.Errorf("windows do not nest: %+v", s)
			}
			if s.SinceSundayShort || s.SinceFirstShort {
				t.Errorf("a window is short: %+v", s)
			}
		}},
		{"short", func(t *testing.T, s server.UsageSpend) {
			if s.FirstDay != today || s.Today != 4.82 {
				t.Errorf("want first day %s and 4.82 today: %+v", today, s)
			}
		}},
		{"empty", func(t *testing.T, s server.UsageSpend) {
			if s.Today != 0 || s.SinceSunday != 0 || s.SinceFirst != 0 || s.FirstDay != "" {
				t.Errorf("want an empty ledger: %+v", s)
			}
		}},
	} {
		t.Run(tc.shape, func(t *testing.T) {
			var flags []string
			if tc.shape != "" {
				flags = []string{"--shape", tc.shape}
			}
			r := startWith(t, "spend", nil, flags...)
			defer r.stop(t)
			var sum server.UsageSummary
			if err := json.Unmarshal([]byte(r.gx(t, "usage", "summary", "--json")), &sum); err != nil {
				t.Fatal(err)
			}
			if sum.Mode != server.UsageModeSpending {
				t.Errorf("mode %q, want spending", sum.Mode)
			}
			tc.check(t, sum.Spend)
		})
	}
}

func TestSpendFixture_AppendedUsageRaisesTodaysTotal(t *testing.T) {
	r := startWith(t, "spend", nil, "--shape", "short")
	defer r.stop(t)
	if r.AgentTranscript == "" {
		t.Fatal("no agent_transcript in the output")
	}
	total := func() float64 {
		var b server.BudgetStatus
		if err := json.Unmarshal([]byte(r.gx(t, "budget", "status", "--json")), &b); err != nil {
			t.Fatal(err)
		}
		return b.Total
	}
	before := total()
	if err := os.MkdirAll(filepath.Dir(r.AgentTranscript), 0o755); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":1000000,"output_tokens":0}}}`+"\n",
		time.Now().Format(time.RFC3339))
	f, err := os.OpenFile(r.AgentTranscript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		got := total()
		if d := got - before; d > 2.99 && d < 3.01 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("total %v after the usage line, want %v + 3.00", got, before)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
