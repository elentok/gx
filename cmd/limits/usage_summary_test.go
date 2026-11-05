package limits

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestUsageSummary_JSONVerb(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	d := Deps{Env: cmdbase.Env{NewClient: func() (*apiclient.Client, error) { return h.Client, nil }}}
	c := NewUsage(d)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"summary", "--json"})
	c.SetContext(context.Background())
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var u wire.UsageSummary
	if err := json.Unmarshal(out.Bytes(), &u); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if u.Mode != wire.UsageModeSpending {
		t.Errorf("mode = %q, want spending", u.Mode)
	}
}

func TestUsageSummary_JSONVerbCarriesForecastAndCodex(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	resets := time.Now().Add(72 * time.Hour).Unix()
	var samples []wire.LimitSample
	for d := 5; d >= 1; d-- {
		day := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()-d, 1, 0, 0, 0, time.Local)
		samples = append(samples, wire.LimitSample{LimitID: wire.LimitIDCodexWeekly, UsedPercentage: float64(10 * (6 - d)), ResetsAt: resets, ReceivedAt: day.Unix()})
	}
	data, err := json.Marshal(map[string]any{
		"samples": map[string]any{wire.LimitIDCodexWeekly: samples},
		"latest":  map[string]any{wire.LimitIDCodexWeekly: wire.LimitSample{LimitID: wire.LimitIDCodexWeekly, UsedPercentage: 50, ResetsAt: resets, ReceivedAt: time.Now().Unix()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.StateDir, "limit-history.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	d := Deps{Env: cmdbase.Env{NewClient: func() (*apiclient.Client, error) { return h.Client, nil }}}
	c := NewUsage(d)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"summary", "--json"})
	c.SetContext(context.Background())
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Codex struct {
			SevenDay struct {
				Chip     map[string]any `json:"chip"`
				Forecast map[string]any `json:"forecast"`
			} `json:"seven_day"`
		} `json:"codex"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	w := raw.Codex.SevenDay
	if w.Chip["direction"] != "down" || w.Forecast["status"] != "projected" || w.Forecast["at_reset"] != float64(80) {
		t.Errorf("codex weekly = %+v, want chip down and forecast projected at 80", w)
	}
}
