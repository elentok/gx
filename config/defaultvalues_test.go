package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/nativerunner"
)

func sizeParser(s string) (int64, error) {
	n, err := ParseByteSize(s)
	return int64(n), err
}

func TestDefaultValue_EveryGlobalKeyHasAValueTheRegistryAccepts(t *testing.T) {
	for _, e := range registry.Default().All() {
		if e.ProjectOnly || e.WriteOnly() {
			continue
		}
		v, ok := DefaultValue(e.Key)
		if !ok {
			t.Errorf("%s: no default", e.Key)
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s: marshal: %v", e.Key, err)
		}
		if err := registry.CheckValue(e, raw, sizeParser); err != nil {
			t.Errorf("%s: default %s rejected: %v", e.Key, raw, err)
		}
	}
}

func TestDefaultValue_SecretsHaveNone(t *testing.T) {
	for _, e := range registry.Default().All() {
		if !e.WriteOnly() {
			continue
		}
		if v, ok := DefaultValue(e.Key); ok {
			t.Errorf("%s: secret default %v", e.Key, v)
		}
	}
}

func TestDefaultValue_HiddenAndFlatKeysMatchTheLoader(t *testing.T) {
	loaded, err := Parse([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path registry.KeyPath
		want any
	}{
		{registry.Key("runner-wait"), durationText(loaded.RunnerWait)},
		{registry.Key("recovery", "notify-hold"), durationText(loaded.Recovery.NotifyHold)},
		{registry.Key("server", "log-retention"), durationText(nativerunner.DefaultLogRetention)}, // the loader's zero means this
		{registry.Key("cpu-watchdog", "sustain"), durationText(loaded.CPUWatchdog.Sustain)},
		{registry.Key("cpu-watchdog", "cooldown"), durationText(loaded.CPUWatchdog.Cooldown)},
		{registry.Key("execution-queue", "spin-window"), durationText(loaded.ExecutionQueue.SpinWindow)},
		{registry.Key("execution-queue", "waiting-slot-grace"), durationText(loaded.ExecutionQueue.WaitingSlotGrace)},
		{registry.Key("execution-queue", "stalled-nudge-delay"), durationText(loaded.ExecutionQueue.StalledNudgeDelay)},
		{registry.Key("evolve.deep-read-cap"), float64(loaded.EvolveDeepReadCap)},
	}
	for _, c := range cases {
		got, ok := DefaultValue(c.path)
		if !ok || got != c.want {
			t.Errorf("%s = %v (ok=%v), want %v", c.path, got, ok, c.want)
		}
	}
}

func TestDefaultValue_DurationRoundTripsInShortestForm(t *testing.T) {
	cases := map[time.Duration]string{
		5 * time.Minute:            "5m",
		10 * time.Hour:             "10h",
		90 * time.Second:           "1m30s",
		time.Hour + 10*time.Minute: "1h10m",
		6 * time.Hour:              "6h",
		30 * time.Second:           "30s",
	}
	for d, want := range cases {
		got := durationText(d)
		if got != want {
			t.Errorf("durationText(%v) = %q, want %q", d, got, want)
		}
		back, err := time.ParseDuration(got)
		if err != nil || back != d {
			t.Errorf("%q parses to %v (%v), want %v", got, back, err, d)
		}
	}
}
