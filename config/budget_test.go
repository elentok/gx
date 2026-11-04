package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadBudgetConfigDefaultsWhenAbsent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Budget.SoftLimit != 300 || cfg.Budget.HardLimit != 350 {
		t.Fatalf("Budget limits = %+v, want soft=300 hard=350", cfg.Budget)
	}
	if want := []float64{50, 100, 150, 200, 250}; !floatSlicesEqual(cfg.Budget.NotificationThresholds, want) {
		t.Fatalf("Budget.NotificationThresholds = %v, want %v", cfg.Budget.NotificationThresholds, want)
	}
}

func loadLongTicketTurns(t *testing.T, body string) int {
	t.Helper()
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })
	if body != "" {
		writeBudgetConfig(t, tmp, body)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg.Budget.LongTicketTurns
}

func TestLongTicketThreshold(t *testing.T) {
	cases := []struct {
		name string
		json string
		want int
	}{
		{"default", "", 100},
		{"other budget keys keep the default", `{"budget":{"soft-limit":10}}`, 100},
		{"file overrides", `{"budget":{"long-ticket-turns":40}}`, 40},
		{"zero disables", `{"budget":{"long-ticket-turns":0}}`, 0},
		{"negative clamps to off", `{"budget":{"long-ticket-turns":-5}}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadLongTicketTurns(t, tc.json); got != tc.want {
				t.Fatalf("Budget.LongTicketTurns = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestLongTicketThresholdSchema(t *testing.T) {
	data, err := os.ReadFile("../docs/config-schema.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var schema struct {
		Properties struct {
			Budget struct {
				Properties map[string]struct {
					Type    string `json:"type"`
					Default any    `json:"default"`
					Minimum *int   `json:"minimum"`
				} `json:"properties"`
			} `json:"budget"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	prop, ok := schema.Properties.Budget.Properties["long-ticket-turns"]
	if !ok {
		t.Fatal("schema budget block lacks long-ticket-turns")
	}
	if prop.Type != "integer" || prop.Default != float64(DefaultBudgetConfig().LongTicketTurns) || prop.Minimum == nil || *prop.Minimum != 0 {
		t.Fatalf("schema property = %+v, want integer, default %d, minimum 0", prop, DefaultBudgetConfig().LongTicketTurns)
	}
}

func TestLoadBudgetConfigZeroDisablesLimitsIndependently(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	writeBudgetConfig(t, tmp, `{"budget":{"soft-limit":0,"hard-limit":30}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Budget.SoftLimit != 0 {
		t.Fatalf("Budget.SoftLimit = %v, want 0 (disabled)", cfg.Budget.SoftLimit)
	}
	if cfg.Budget.HardLimit != 30 {
		t.Fatalf("Budget.HardLimit = %v, want 30 (unaffected)", cfg.Budget.HardLimit)
	}

	tmp2 := t.TempDir()
	userConfigDirFn = func() (string, error) { return tmp2, nil }
	writeBudgetConfig(t, tmp2, `{"budget":{"soft-limit":20,"hard-limit":0}}`)

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Budget.HardLimit != 0 {
		t.Fatalf("Budget.HardLimit = %v, want 0 (disabled)", cfg2.Budget.HardLimit)
	}
	if cfg2.Budget.SoftLimit != 20 {
		t.Fatalf("Budget.SoftLimit = %v, want 20 (unaffected)", cfg2.Budget.SoftLimit)
	}
}

func TestLoadBudgetConfigSortsAndDedupesThresholds(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	writeBudgetConfig(t, tmp, `{"budget":{"notification-thresholds":[10,5,10,15,5]}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []float64{5, 10, 15}; !floatSlicesEqual(cfg.Budget.NotificationThresholds, want) {
		t.Fatalf("Budget.NotificationThresholds = %v, want %v", cfg.Budget.NotificationThresholds, want)
	}
}

func TestLoadBudgetConfigBumpsHardLimitAtOrBelowSoftLimit(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	writeBudgetConfig(t, tmp, `{"budget":{"soft-limit":20,"hard-limit":20}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Budget.HardLimit != 20 {
		t.Fatalf("Budget.HardLimit = %v, want 20 (bumped to soft limit)", cfg.Budget.HardLimit)
	}

	tmp2 := t.TempDir()
	userConfigDirFn = func() (string, error) { return tmp2, nil }
	writeBudgetConfig(t, tmp2, `{"budget":{"soft-limit":20,"hard-limit":10}}`)

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Budget.HardLimit != 20 {
		t.Fatalf("Budget.HardLimit = %v, want 20 (bumped to soft limit)", cfg2.Budget.HardLimit)
	}
}

func floatSlicesEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
