package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func TestLimitsFixture_ShapesShowInTheSummary(t *testing.T) {
	for _, tc := range []struct {
		shape string
		check func(t *testing.T, now int64, l server.LimitsReport)
	}{
		{"", func(t *testing.T, now int64, l server.LimitsReport) {
			if l.FiveHour == nil || l.SevenDay == nil || l.SevenDay.UsedPercentage != 81 {
				t.Errorf("default shape is not fresh: %+v", l)
			}
		}},
		{"fresh", func(t *testing.T, now int64, l server.LimitsReport) {
			if l.FiveHour == nil || l.SevenDay == nil {
				t.Fatalf("want two windows: %+v", l)
			}
			if l.SevenDay.UsedPercentage != 81 {
				t.Errorf("weekly at %v, want 81", l.SevenDay.UsedPercentage)
			}
			if age := now - l.FiveHour.ReceivedAt; age > 120 {
				t.Errorf("fresh window received %ds ago", age)
			}
		}},
		{"stale", func(t *testing.T, now int64, l server.LimitsReport) {
			if l.FiveHour == nil || l.SevenDay == nil {
				t.Fatalf("want two windows: %+v", l)
			}
			for _, w := range []*server.LimitWindow{l.FiveHour, l.SevenDay} {
				if age := time.Duration(now-w.ReceivedAt) * time.Second; age < 170*time.Minute || age > 190*time.Minute {
					t.Errorf("received %v ago, want about 3h", age)
				}
			}
		}},
		{"past-reset", func(t *testing.T, now int64, l server.LimitsReport) {
			if l.FiveHour == nil || l.FiveHour.ResetsAt >= now {
				t.Errorf("5h reset is not in the past: %+v", l.FiveHour)
			}
			if l.SevenDay == nil || l.SevenDay.ResetsAt <= now {
				t.Errorf("weekly reset is not in the future: %+v", l.SevenDay)
			}
		}},
		{"one-window", func(t *testing.T, now int64, l server.LimitsReport) {
			if l.FiveHour != nil || l.SevenDay == nil || l.SevenDay.UsedPercentage != 12 {
				t.Errorf("want only a 12%% weekly window: %+v", l)
			}
		}},
	} {
		t.Run(tc.shape, func(t *testing.T) {
			var flags []string
			if tc.shape != "" {
				flags = []string{"--shape", tc.shape}
			}
			r := startWith(t, "limits", nil, flags...)
			defer r.stop(t)
			var sum server.UsageSummary
			if err := json.Unmarshal([]byte(r.gx(t, "usage", "summary", "--json")), &sum); err != nil {
				t.Fatal(err)
			}
			if sum.Mode != server.UsageModeLimits {
				t.Errorf("mode %q, want limits", sum.Mode)
			}
			tc.check(t, time.Now().Unix(), sum.Limits)
		})
	}
}

func TestLimitsFixture_UnknownShapeNamesTheValidOnes(t *testing.T) {
	out, err := exec.Command(fixtureBin, "--gx", gxBin, "--fakeclaude", fakeBin, "--shape", "bogus", "limits").CombinedOutput()
	if err == nil {
		t.Fatal("unknown shape started")
	}
	for _, name := range []string{"fresh", "stale", "past-reset", "one-window"} {
		if !strings.Contains(string(out), name) {
			t.Errorf("error does not list %q:\n%s", name, out)
		}
	}
}
