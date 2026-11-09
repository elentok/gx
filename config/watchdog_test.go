package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseWatchdog(t *testing.T) {
	def := DefaultWatchdogConfig()
	if def != (WatchdogConfig{MaxTreeProcesses: 200, Interval: time.Second, UlimitHeadroom: 1000}) {
		t.Fatalf("default = %+v", def)
	}
	for _, tc := range []struct {
		name, json string
		want       WatchdogConfig
	}{
		{"absent", `{}`, def},
		{"set", `{"watchdog":{"max-tree-processes":50,"interval":"250ms","ulimit-headroom":10}}`,
			WatchdogConfig{MaxTreeProcesses: 50, Interval: 250 * time.Millisecond, UlimitHeadroom: 10}},
		{"non-positive", `{"watchdog":{"max-tree-processes":0,"interval":"-1s","ulimit-headroom":-5}}`, def},
		{"bad duration", `{"watchdog":{"interval":"soon"}}`, def},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.json))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if cfg.Watchdog != tc.want {
				t.Errorf("Watchdog = %+v, want %+v", cfg.Watchdog, tc.want)
			}
		})
	}
}

func TestWatchdogSchema(t *testing.T) {
	data, err := os.ReadFile("../docs/config-schema.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var schema struct {
		Properties struct {
			Watchdog struct {
				Properties map[string]struct {
					Type        string `json:"type"`
					Default     any    `json:"default"`
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"watchdog"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	props := schema.Properties.Watchdog.Properties
	for key, want := range map[string]struct {
		typ string
		def any
	}{
		"max-tree-processes": {"integer", float64(200)},
		"ulimit-headroom":    {"integer", float64(1000)},
		"interval":           {"string", nil},
	} {
		p, ok := props[key]
		if !ok {
			t.Errorf("schema watchdog block lacks %s", key)
			continue
		}
		if p.Type != want.typ || p.Default != want.def {
			t.Errorf("%s = type %s default %v, want %s %v", key, p.Type, p.Default, want.typ, want.def)
		}
	}
	if !strings.Contains(props["interval"].Description, "1s") {
		t.Errorf("interval description %q lacks the 1s default", props["interval"].Description)
	}
	if !strings.Contains(props["ulimit-headroom"].Description, "launches") {
		t.Errorf("ulimit-headroom description %q lacks the launch-time note", props["ulimit-headroom"].Description)
	}
}
